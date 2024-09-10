package driver

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RomiChan/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/utils/helper"
)

var (
	nullResponse = zero.APIResponse{}
)

// WSClient ...
type WSClient struct {
	seq         uint64
	conn        *websocket.Conn
	mu          sync.Mutex // 写锁
	seqMap      seqSyncMap
	Url         string // ws连接地址
	AccessToken string
	SelfIDs     []int64
	Hooks       *Hooks
}

// NewWebSocketClient 默认Driver，使用正向WS通信
func NewWebSocketClient(url, accessToken string, selfIDs []int64) *WSClient {
	if selfIDs == nil {
		selfIDs = []int64{}
	}
	return &WSClient{
		Url:         url,
		AccessToken: accessToken,
		SelfIDs:     selfIDs,
		Hooks:       NewHooks(),
	}
}

// Connect 连接ws服务端
func (ws *WSClient) Connect() {
	log.Infof("[ws] 开始尝试连接到Websocket服务器: %v", ws.Url)
	header := http.Header{
		"X-Client-Role": []string{"Universal"},
		"User-Agent":    []string{"ZeroBot/1.7.4"},
	}
	if ws.AccessToken != "" {
		header["Authorization"] = []string{"Bearer " + ws.AccessToken}
	}

	network, address := resolveURI(ws.Url)
	dialer := websocket.Dialer{
		NetDial: func(_, addr string) (net.Conn, error) {
			if network == "unix" {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					host = addr
				}
				filepath, err := base64.RawURLEncoding.DecodeString(host)
				if err == nil {
					addr = helper.BytesToString(filepath)
				}
			}
			return net.Dial(network, addr) // support unix socket transport
		},
	}

	for {
		conn, res, err := dialer.Dial(address, header)
		if err != nil {
			log.Warnf("[ws] 连接到Websocket服务器 %v 时出现错误: %v", ws.Url, err)
			time.Sleep(2 * time.Second) // 等待两秒后重新连接
			continue
		}
		ws.conn = conn
		_ = res.Body.Close()
		var rsp struct {
			SelfID int64 `json:"self_id"`
		}

		connected := atomic.Int32{}
		botCnt := int32(len(ws.SelfIDs))
		if botCnt == 0 {
			connected.Store(1)
		} else {
			connected.Store(botCnt)
		}

		timeout := 5 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		for {
			select {
			case <-ctx.Done():
				if len(ws.SelfIDs) > 1 {
					log.Warnf("[ws] 与Websocket服务器连接超时，%d个bot连接失败", connected.Load())
				}
				cancel()
				goto connected
			default:
				err = ws.conn.ReadJSON(&rsp)
				if err != nil {
					log.Warnf("[ws] 与Websocket服务器 %v 握手时出现错误: %v", ws.Url, err)
					time.Sleep(2 * time.Second) // 等待两秒后重新连接
					continue
				}

				selfId := rsp.SelfID
				zero.APICallers.Store(selfId, ws) // 添加Caller到 APICaller list...
				log.Infof("[ws] 连接Websocket服务器: %s 成功, 账号: %d", ws.Url, selfId)

				connected.Add(-1)
				if connected.Load() == 0 {
					cancel()
					goto connected
				}
			}
		}
	}
connected:
	ws.Hooks.onConnectionEstablished()
	ws.Hooks.shouldExecuteDisconnect.Store(true)
}

// Listen 开始监听事件
func (ws *WSClient) Listen(handler func([]byte, zero.APICaller)) {
	go func() {
		for {
			t, payload, err := ws.conn.ReadMessage()
			if err != nil { // reconnect
				for _, selfID := range ws.SelfIDs {
					zero.APICallers.Delete(selfID) // 断开从apicaller中删除
				}
				log.Warn("[ws] Websocket服务器连接断开...")
				if ws.Hooks.shouldExecuteDisconnect.Load() {
					ws.Hooks.onDisconnect(ws.SelfIDs)
				}
				ws.Hooks.shouldExecuteDisconnect.Store(false)
				time.Sleep(time.Millisecond * time.Duration(3))
				ws.Connect()
				continue
			}
			if t != websocket.TextMessage {
				continue
			}
			rsp := gjson.Parse(helper.BytesToString(payload))
			if rsp.Get("echo").Exists() { // 存在echo字段，是api调用的返回
				log.Debug("[ws] 接收到API调用返回: ", strings.TrimSpace(helper.BytesToString(payload)))
				if c, ok := ws.seqMap.LoadAndDelete(rsp.Get("echo").Uint()); ok {
					c <- zero.APIResponse{ // 发送api调用响应
						Status:  rsp.Get("status").String(),
						Data:    rsp.Get("data"),
						Msg:     rsp.Get("msg").Str,
						Wording: rsp.Get("wording").Str,
						RetCode: rsp.Get("retcode").Int(),
						Echo:    rsp.Get("echo").Uint(),
					}
					close(c) // channel only use once
				}
				continue
			}
			if rsp.Get("meta_event_type").Str == "heartbeat" { // 忽略心跳事件
				continue
			}
			if rsp.Get("sub_type").Str == "connect" { //处理连接事件
				selfId := rsp.Get("self_id").Int()
				zero.APICallers.Store(selfId, ws) // 添加Caller到 APICaller list...
				log.Infof("[ws] 连接Websocket服务器: %s 成功, 账号: %d", ws.Url, selfId)
				ws.Hooks.onSingleBotConnect(selfId)
				continue
			}
			log.Debug("[ws] 接收到事件: ", helper.BytesToString(payload))
			handler(payload, ws)
		}
	}()
	for _, selfID := range ws.SelfIDs {
		ws.Hooks.onSingleBotConnect(selfID)
	}
	ws.Hooks.onAllBotConnected(ws.SelfIDs)

	loadPlugin()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	for {
		select {
		case <-time.After(1 * time.Second):
		case <-interrupt:
			log.Info("[ws] 监听到退出信号, 尝试关闭连接...")
			if ws.Hooks.shouldExecuteDisconnect.Load() {
				ws.Hooks.onDisconnect(ws.SelfIDs)
				ws.Hooks.shouldExecuteDisconnect.Store(false)
			}
			err := ws.conn.Close()
			if err != nil {
				log.Error("[ws] 关闭连接失败: ", err.Error())
			} else {
				log.Info("[ws] 已关闭连接")
				os.Exit(0)
			}
		}
	}
}

func (ws *WSClient) nextSeq() uint64 {
	return atomic.AddUint64(&ws.seq, 1)
}

// CallApi 发送ws请求
func (ws *WSClient) CallApi(req zero.APIRequest) (zero.APIResponse, error) {
	ch := make(chan zero.APIResponse, 1)
	req.Echo = ws.nextSeq()
	ws.seqMap.Store(req.Echo, ch)

	// send message
	ws.mu.Lock() // websocket write is not goroutine safe
	err := ws.conn.WriteJSON(&req)
	ws.mu.Unlock()
	if err != nil {
		log.Warn("[ws] 向WebsocketServer发送API请求失败: ", err.Error())
		return nullResponse, err
	}
	log.Debug("[ws] 向服务器发送请求: ", &req)
	select { // 等待数据返回
	case rsp, ok := <-ch:
		if !ok {
			return nullResponse, io.ErrClosedPipe
		}
		return rsp, nil
	case <-time.After(time.Minute):
		return nullResponse, os.ErrDeadlineExceeded
	}
}
