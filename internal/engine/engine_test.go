package engine

import (
	"sync/atomic"
	"testing"
	"time"
)

func newTestLoop() GameLoop {
	return NewGameLoop(GameLoopConfig{TickRate: 100})
}

func TestEngine_StartStop(t *testing.T) {
	loop := newTestLoop()
	e := NewEngine(EngineConfig{TickRate: 100}, loop)

	if err := e.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !e.IsRunning() {
		t.Error("expected engine to be running")
	}

	if err := e.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if e.IsRunning() {
		t.Error("expected engine to be stopped")
	}
}

func TestEngine_DoubleStart(t *testing.T) {
	loop := newTestLoop()
	e := NewEngine(EngineConfig{}, loop)
	e.Start()
	defer e.Stop()

	err := e.Start()
	if err == nil {
		t.Error("expected error on double Start")
	}
}

func TestEngine_StopNotRunning(t *testing.T) {
	loop := newTestLoop()
	e := NewEngine(EngineConfig{}, loop)

	err := e.Stop()
	if err == nil {
		t.Error("expected error when stopping non-running engine")
	}
}

func TestEngine_RegisterHandlers_Called(t *testing.T) {
	loop := newTestLoop()
	e := NewEngine(EngineConfig{TickRate: 100}, loop)

	var inputCalled, updateCalled, syncCalled, cleanupCalled atomic.Int32

	e.RegisterInputHandler(func(tick uint64, dt time.Duration) { inputCalled.Add(1) })
	e.RegisterUpdateHandler(func(tick uint64, dt time.Duration) { updateCalled.Add(1) })
	e.RegisterSyncHandler(func(tick uint64, dt time.Duration) { syncCalled.Add(1) })
	e.RegisterCleanupHandler(func(tick uint64, dt time.Duration) { cleanupCalled.Add(1) })

	e.Start()
	time.Sleep(50 * time.Millisecond)
	e.Stop()

	if inputCalled.Load() == 0 {
		t.Error("expected input handler to be called")
	}
	if updateCalled.Load() == 0 {
		t.Error("expected update handler to be called")
	}
	if syncCalled.Load() == 0 {
		t.Error("expected sync handler to be called")
	}
	if cleanupCalled.Load() == 0 {
		t.Error("expected cleanup handler to be called")
	}
}

func TestEngine_NilLoop(t *testing.T) {
	e := NewEngine(EngineConfig{}, nil)

	// Should not panic with nil loop.
	if err := e.Start(); err != nil {
		t.Fatalf("Start with nil loop failed: %v", err)
	}
	if err := e.Stop(); err != nil {
		t.Fatalf("Stop with nil loop failed: %v", err)
	}
}

func TestEngine_DefaultTickRate(t *testing.T) {
	loop := NewGameLoop(GameLoopConfig{})
	e := NewEngine(EngineConfig{TickRate: 0}, loop)
	_ = e
	// TickRate defaults to 20 — just verify no panic.
}
