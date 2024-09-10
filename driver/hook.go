package driver

import (
	log "github.com/sirupsen/logrus"
	"github.com/wdvxdr1123/ZeroBot"
	"sync/atomic"
)

type Hooks struct {
	connectionEstablished   []func() error
	singleBotConnected      map[int64][]func(ctx *zero.Ctx) error
	allBotConnected         []func(ctx []*zero.Ctx) error
	disConnected            []func(ctx []*zero.Ctx) error
	shouldExecuteDisconnect atomic.Bool
}

func NewHooks() *Hooks {
	return &Hooks{
		connectionEstablished:   make([]func() error, 0),
		singleBotConnected:      make(map[int64][]func(ctx *zero.Ctx) error),
		allBotConnected:         make([]func(ctx []*zero.Ctx) error, 0),
		disConnected:            make([]func(ctx []*zero.Ctx) error, 0),
		shouldExecuteDisconnect: atomic.Bool{},
	}
}
func (hooks *Hooks) AddSingleBotConnectHook(selfId int64, onBotConnect ...func(ctx *zero.Ctx) error) {
	hooks.singleBotConnected[selfId] = append(hooks.singleBotConnected[selfId], onBotConnect...)
}

func (hooks *Hooks) AddAllBotConnectHook(onAllBotConnected ...func(ctx []*zero.Ctx) error) {
	hooks.allBotConnected = append(hooks.allBotConnected, onAllBotConnected...)
}

func (hooks *Hooks) AddBotDisconnectHook(onBotDisconnect ...func(ctx []*zero.Ctx) error) {
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
func (hooks *Hooks) onSingleBotConnect(selfID int64) {
	defer func() {
		if err := recover(); err != nil {
			log.Fatalf("[bot] 执行单个bot连接钩子函数出现错误：%v", err)
		}
	}()

	onConnected := hooks.singleBotConnected[selfID]
	ctx := NewHookCtx(selfID)
	for _, fun := range onConnected {
		err := fun(ctx)
		if err != nil {
			log.Fatalf("[bot] 执行bot %d 的连接钩子函数出现错误：%v", selfID, err)
		}
	}
}

func (hooks *Hooks) onAllBotConnected(selfIDs []int64) {
	defer func() {
		if err := recover(); err != nil {
			log.Fatalf("[bot] 执行所有bot连接钩子函数出现错误：%v", err)
		}
	}()

	var ctxs []*zero.Ctx
	for _, selfID := range selfIDs {
		ctx := NewHookCtx(selfID)
		ctxs = append(ctxs, ctx)
	}

	for _, fun := range hooks.allBotConnected {
		err := fun(ctxs)
		if err != nil {
			log.Fatalf("[bot] 执行所有bot连接钩子函数出现错误：%v", err)
		}
	}
}

func (hooks *Hooks) onDisconnect(selfIDs []int64) {
	defer func() {
		if err := recover(); err != nil {
			if err := recover(); err != nil {
				log.Errorf("[bot] 执行断开连接钩子函数出现错误：%v", err)
			}
		}
	}()

	var ctxs []*zero.Ctx
	for _, selfID := range selfIDs {
		ctx := NewHookCtx(selfID)
		ctxs = append(ctxs, ctx)
	}

	for _, fun := range hooks.disConnected {
		err := fun(ctxs)
		if err != nil {
			log.Errorf("[bot] 执行断开连接钩子函数出现错误：%v", err)
		}
	}
}
