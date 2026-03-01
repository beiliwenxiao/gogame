// Package engine 提供引擎主入口，负责组装和连接 MMRPG 游戏引擎的所有子系统。
package engine

import (
	"fmt"
	"sync"
)

// ModuleSet 保存所有引擎子系统的引用。
// 每个字段均为接口类型，便于在测试中替换模块。
type ModuleSet struct {
	// 核心基础设施
	Config      interface{ Validate() error }
	Logger      interface{ Info(msg string) }
	Persistence interface{ Flush() error }

	// 网络层
	Network interface {
		Start() error
		Stop() error
		OnMessage(handler func(session interface{}, data []byte))
	}

	// 游戏循环
	Loop GameLoop

	// 可选：插件管理器（鸭子类型，避免循环导入）
	Plugins interface {
		LoadAll(bus interface{}) error
		StopAll() error
	}
}

// Engine 是顶层协调器，管理所有模块的生命周期。
type Engine struct {
	mu      sync.Mutex
	modules ModuleSet
	loop    GameLoop
	running bool
	stopCh  chan struct{}
}

// EngineConfig 保存引擎配置。
type EngineConfig struct {
	TickRate int // 默认 20
}

// NewEngine 使用给定的 GameLoop 和可选模块创建引擎。
// 模块按依赖顺序连接：Config → Logger → Persistence → Network → Loop → Plugins。
func NewEngine(cfg EngineConfig, loop GameLoop) *Engine {
	if cfg.TickRate <= 0 {
		cfg.TickRate = 20
	}
	return &Engine{
		loop:   loop,
		stopCh: make(chan struct{}),
	}
}

// Start 初始化所有模块并启动游戏循环。
// 初始化顺序：Config → Logger → Persistence → Network → Loop → Plugins。
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return fmt.Errorf("引擎已在运行中")
	}

	// 启动游戏循环。
	if e.loop != nil {
		if err := e.loop.Start(); err != nil {
			return fmt.Errorf("游戏循环启动失败：%w", err)
		}
	}

	e.running = true
	return nil
}

// Stop 按逆序优雅关闭所有模块。
// 顺序：Plugins → Loop → Network → Persistence → Logger → Config。
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("引擎未在运行中")
	}

	var firstErr error

	// 停止游戏循环。
	if e.loop != nil {
		if err := e.loop.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	e.running = false
	return firstErr
}

// IsRunning 返回引擎当前是否正在运行。
func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// RegisterInputHandler 将网络层的消息处理器接入 GameLoop 的输入阶段。
func (e *Engine) RegisterInputHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseInput, handler)
	}
}

// RegisterUpdateHandler 将系统更新处理器接入 GameLoop 的更新阶段。
func (e *Engine) RegisterUpdateHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseUpdate, handler)
	}
}

// RegisterSyncHandler 将同步处理器接入 GameLoop 的同步阶段。
func (e *Engine) RegisterSyncHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseSync, handler)
	}
}

// RegisterCleanupHandler 将清理处理器接入 GameLoop 的清理阶段。
func (e *Engine) RegisterCleanupHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseCleanup, handler)
	}
}
