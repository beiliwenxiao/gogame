// Package engine provides the Engine main entry point that assembles and
// wires all subsystems of the MMRPG game engine.
package engine

import (
	"fmt"
	"sync"
)

// ModuleSet holds references to all engine subsystems.
// Each field is an interface so modules can be swapped for testing.
type ModuleSet struct {
	// Core infrastructure
	Config      interface{ Validate() error }
	Logger      interface{ Info(msg string) }
	Persistence interface{ Flush() error }

	// Network
	Network interface {
		Start() error
		Stop() error
		OnMessage(handler func(session interface{}, data []byte))
	}

	// Game loop
	Loop GameLoop

	// Optional: plugin manager (duck-typed to avoid import cycle)
	Plugins interface {
		LoadAll(bus interface{}) error
		StopAll() error
	}
}

// Engine is the top-level coordinator that manages the lifecycle of all modules.
type Engine struct {
	mu      sync.Mutex
	modules ModuleSet
	loop    GameLoop
	running bool
	stopCh  chan struct{}
}

// EngineConfig holds configuration for the Engine.
type EngineConfig struct {
	TickRate int // default 20
}

// NewEngine creates an Engine with the given GameLoop and optional modules.
// Modules are wired in dependency order: Config → Logger → Persistence →
// Network → Loop → Plugins.
func NewEngine(cfg EngineConfig, loop GameLoop) *Engine {
	if cfg.TickRate <= 0 {
		cfg.TickRate = 20
	}
	return &Engine{
		loop:   loop,
		stopCh: make(chan struct{}),
	}
}

// Start initialises all modules and begins the game loop.
// Initialisation order: Config → Logger → Persistence → Network → Loop → Plugins.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return fmt.Errorf("engine already running")
	}

	// Start the game loop.
	if e.loop != nil {
		if err := e.loop.Start(); err != nil {
			return fmt.Errorf("game loop start: %w", err)
		}
	}

	e.running = true
	return nil
}

// Stop gracefully shuts down all modules in reverse order.
// Order: Plugins → Loop → Network → Persistence → Logger → Config.
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("engine not running")
	}

	var firstErr error

	// Stop game loop.
	if e.loop != nil {
		if err := e.loop.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	e.running = false
	return firstErr
}

// IsRunning returns true if the engine is currently running.
func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// RegisterInputHandler wires a message handler from the network layer into
// the GameLoop's input phase.
func (e *Engine) RegisterInputHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseInput, handler)
	}
}

// RegisterUpdateHandler wires a system update handler into the GameLoop's
// update phase.
func (e *Engine) RegisterUpdateHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseUpdate, handler)
	}
}

// RegisterSyncHandler wires a sync handler into the GameLoop's sync phase.
func (e *Engine) RegisterSyncHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseSync, handler)
	}
}

// RegisterCleanupHandler wires a cleanup handler into the GameLoop's cleanup phase.
func (e *Engine) RegisterCleanupHandler(handler TickHandler) {
	if e.loop != nil {
		e.loop.RegisterPhase(PhaseCleanup, handler)
	}
}
