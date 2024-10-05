package driver

import (
	log "github.com/sirupsen/logrus"
	"github.com/wdvxdr1123/ZeroBot"
	"sync/atomic"
)

type Hooks struct {
	connectionEstablished []func() error
	botConnected          []func(ctx *zero.Ctx) error
	disConnected          []func(ctx *zero.Ctx) error
	diconnectLock         atomic.Bool
}

func NewHooks() *Hooks {
	return &Hooks{
		connectionEstablished: make([]func() error, 0),
		botConnected:          make([]func(ctx *zero.Ctx) error, 0),
		disConnected:          make([]func(ctx *zero.Ctx) error, 0),
		diconnectLock:         atomic.Bool{},
	}
}
func (hooks *Hooks) AddBotConnectHook(onBotConnect ...func(ctx *zero.Ctx) error) {
	hooks.botConnected = append(hooks.botConnected, onBotConnect...)
}

func (hooks *Hooks) AddBotDisconnectHook(onBotDisconnect ...func(ctx *zero.Ctx) error) {
	hooks.disConnected = append(hooks.disConnected, onBotDisconnect...)
}

// NewHookCtx 创建钩子函数中使用的Ctx
func NewHookCtx(selfId int64) *zero.Ctx {
	caller, ok := zero.APICallers.Load(selfId)
	if !ok {
		return nil
	}
	return &zero.Ctx{
		Event:  &zero.Event{SelfID: selfId},
		Caller: caller,
	}
}

func (hooks *Hooks) onConnectionEstablished() {
	defer func() {
		if err := recover(); err != nil {
			log.Fatalf("[bot] 执行连接建立完成钩子函数出现错误：%v", err)
		}
	}()

	for _, fun := range hooks.connectionEstablished {
		err := fun()
		if err != nil {
			log.Fatalf("[bot] 执行连接建立完成钩子函数出现错误：%v", err)
		}
	}
}
func (hooks *Hooks) onBotConnect(selfID int64) {
	defer func() {
		if err := recover(); err != nil {
			log.Fatalf("[bot] 执行单个bot连接钩子函数出现错误：%v", err)
		}
	}()

	onConnected := hooks.botConnected
	ctx := NewHookCtx(selfID)
	for _, fun := range onConnected {
		err := fun(ctx)
		if err != nil {
			log.Fatalf("[bot] 执行bot %d 的连接钩子函数出现错误：%v", selfID, err)
		}
	}
}

func (hooks *Hooks) onDisconnect(selfID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[bot] 执行断开连接钩子函数出现错误：%v", r)
		}
	}()

	ctx := NewHookCtx(selfID)
	for _, fun := range hooks.disConnected {
		err := fun(ctx)
		if err != nil {
			log.Errorf("[bot] 执行断开连接钩子函数出现错误：%v", err)
		}
	}
}
