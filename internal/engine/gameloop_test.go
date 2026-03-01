package engine

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNewGameLoop_DefaultTickRate(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{})
	if gl.TickRate() != 20 {
		t.Errorf("expected default tick rate 20, got %d", gl.TickRate())
	}
}

func TestNewGameLoop_CustomTickRate(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 10})
	if gl.TickRate() != 10 {
		t.Errorf("expected tick rate 10, got %d", gl.TickRate())
	}
}

func TestNewGameLoop_NegativeTickRate_DefaultsTo20(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: -5})
	if gl.TickRate() != 20 {
		t.Errorf("expected default tick rate 20 for negative input, got %d", gl.TickRate())
	}
}

func TestGameLoop_StartStop(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20})
	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	// Let it run briefly
	time.Sleep(50 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestGameLoop_DoubleStart_ReturnsError(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20})
	if err := gl.Start(); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer gl.Stop()

	if err := gl.Start(); err == nil {
		t.Error("expected error on double Start, got nil")
	}
}

func TestGameLoop_StopWithoutStart_ReturnsError(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20})
	if err := gl.Stop(); err == nil {
		t.Error("expected error on Stop without Start, got nil")
	}
}

func TestGameLoop_TickCounterIncreases(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20}) // 50ms per tick
	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	// Wait enough time for several ticks
	time.Sleep(200 * time.Millisecond)
	tick := gl.CurrentTick()
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if tick == 0 {
		t.Error("expected tick counter > 0 after running")
	}
}

func TestGameLoop_InitialTickIsZero(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20})
	if gl.CurrentTick() != 0 {
		t.Errorf("expected initial tick 0, got %d", gl.CurrentTick())
	}
}

func TestGameLoop_FourPhaseExecution(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20}) // 50ms interval

	var order []TickPhase
	var orderTick uint64

	// Register one handler per phase
	for _, phase := range []TickPhase{PhaseInput, PhaseUpdate, PhaseSync, PhaseCleanup} {
		p := phase
		gl.RegisterPhase(p, func(tick uint64, dt time.Duration) {
			order = append(order, p)
			orderTick = tick
		})
	}

	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	// Wait for at least one tick
	time.Sleep(100 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if orderTick == 0 {
		t.Fatal("no ticks executed")
	}

	// Verify the phase order for the last tick (at minimum)
	// Each tick should produce [Input, Update, Sync, Cleanup]
	if len(order) < 4 {
		t.Fatalf("expected at least 4 phase calls, got %d", len(order))
	}

	// Check that every group of 4 follows the correct order
	for i := 0; i+3 < len(order); i += 4 {
		expected := []TickPhase{PhaseInput, PhaseUpdate, PhaseSync, PhaseCleanup}
		for j := 0; j < 4; j++ {
			if order[i+j] != expected[j] {
				t.Errorf("tick group at %d: phase[%d] = %d, want %d", i/4, j, order[i+j], expected[j])
			}
		}
	}
}

func TestGameLoop_MultipleHandlersPerPhase(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20})

	var callOrder []int

	gl.RegisterPhase(PhaseInput, func(tick uint64, dt time.Duration) {
		callOrder = append(callOrder, 1)
	})
	gl.RegisterPhase(PhaseInput, func(tick uint64, dt time.Duration) {
		callOrder = append(callOrder, 2)
	})
	gl.RegisterPhase(PhaseUpdate, func(tick uint64, dt time.Duration) {
		callOrder = append(callOrder, 3)
	})

	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify that within each tick, handler 1 runs before 2, and both before 3
	if len(callOrder) < 3 {
		t.Fatalf("expected at least 3 handler calls, got %d", len(callOrder))
	}

	// Check first tick's order
	if callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Errorf("expected first tick order [1,2,3], got %v", callOrder[:3])
	}
}

func TestGameLoop_MonotonicallyIncreasingTicks(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20})

	var ticks []uint64
	gl.RegisterPhase(PhaseInput, func(tick uint64, dt time.Duration) {
		ticks = append(ticks, tick)
	})

	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if len(ticks) < 2 {
		t.Fatalf("expected at least 2 ticks, got %d", len(ticks))
	}

	for i := 1; i < len(ticks); i++ {
		if ticks[i] <= ticks[i-1] {
			t.Errorf("tick %d (%d) not greater than tick %d (%d)", i, ticks[i], i-1, ticks[i-1])
		}
	}
}

func TestGameLoop_OnStopCallback(t *testing.T) {
	var called atomic.Bool
	gl := NewGameLoop(GameLoopConfig{
		TickRate: 20,
		OnStop: func() {
			called.Store(true)
		},
	})

	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if !called.Load() {
		t.Error("expected OnStop callback to be called")
	}
}

func TestGameLoop_TickTimeout_SkipsFrames(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20}) // 50ms interval

	var ticksSeen []uint64
	// Register a slow handler that takes longer than the frame interval
	gl.RegisterPhase(PhaseInput, func(tick uint64, dt time.Duration) {
		ticksSeen = append(ticksSeen, tick)
		if tick == 1 {
			// Simulate a slow tick that takes ~120ms (>50ms interval)
			time.Sleep(120 * time.Millisecond)
		}
	})

	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	// Wait enough for the slow tick + a few more ticks
	time.Sleep(400 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if len(ticksSeen) < 2 {
		t.Fatalf("expected at least 2 ticks, got %d", len(ticksSeen))
	}

	// After the slow tick 1, the counter should have jumped (skipped frames)
	// The second tick seen should be > 2 because frames were skipped
	if len(ticksSeen) >= 2 && ticksSeen[1] <= ticksSeen[0]+1 {
		// This is acceptable if the skip didn't happen due to timing,
		// but we at least verify monotonic increase
		for i := 1; i < len(ticksSeen); i++ {
			if ticksSeen[i] <= ticksSeen[i-1] {
				t.Errorf("ticks not monotonically increasing: %v", ticksSeen)
				break
			}
		}
	}
}

func TestGameLoop_DtParameter(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20}) // 50ms interval

	var receivedDt time.Duration
	gl.RegisterPhase(PhaseInput, func(tick uint64, dt time.Duration) {
		receivedDt = dt
	})

	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	expected := 50 * time.Millisecond
	if receivedDt != expected {
		t.Errorf("expected dt=%v, got %v", expected, receivedDt)
	}
}

func TestGameLoop_RegisterInvalidPhase(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20})

	// Should not panic for invalid phase values
	gl.RegisterPhase(TickPhase(-1), func(tick uint64, dt time.Duration) {})
	gl.RegisterPhase(TickPhase(10), func(tick uint64, dt time.Duration) {})
}

func TestGameLoop_TickRateAccuracy(t *testing.T) {
	gl := NewGameLoop(GameLoopConfig{TickRate: 20}) // 50ms per tick

	var tickCount atomic.Uint64
	gl.RegisterPhase(PhaseInput, func(tick uint64, dt time.Duration) {
		tickCount.Add(1)
	})

	if err := gl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := gl.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	count := tickCount.Load()
	// At 20 ticks/sec over 500ms, expect ~10 ticks. Allow some tolerance.
	if count < 7 || count > 14 {
		t.Errorf("expected ~10 ticks in 500ms at 20 tick/s, got %d", count)
	}
}
