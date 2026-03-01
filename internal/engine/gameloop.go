package engine

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// TickHandler is a callback invoked during a specific tick phase.
type TickHandler func(tick uint64, dt time.Duration)

// GameLoop drives the game logic at a fixed tick rate.
type GameLoop interface {
	Start() error
	Stop() error
	CurrentTick() uint64
	TickRate() int
	RegisterPhase(phase TickPhase, handler TickHandler)
}

// GameLoopConfig holds configuration for the game loop.
type GameLoopConfig struct {
	TickRate int // Ticks per second, default 20
	OnStop   func() // Optional callback invoked on safe stop after the last tick
}

// gameLoop is the concrete implementation of GameLoop.
type gameLoop struct {
	config   GameLoopConfig
	tick     atomic.Uint64
	handlers [4][]TickHandler // indexed by TickPhase (0-3)
	mu       sync.RWMutex     // protects handlers

	stopCh  chan struct{}
	stopped chan struct{}
	running atomic.Bool
}

// NewGameLoop creates a new GameLoop with the given configuration.
func NewGameLoop(cfg GameLoopConfig) GameLoop {
	if cfg.TickRate <= 0 {
		cfg.TickRate = 20
	}
	return &gameLoop{
		config:  cfg,
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

// RegisterPhase registers a handler for the given tick phase.
// Multiple handlers per phase are executed in registration order.
func (gl *gameLoop) RegisterPhase(phase TickPhase, handler TickHandler) {
	if phase < PhaseInput || phase > PhaseCleanup {
		return
	}
	gl.mu.Lock()
	defer gl.mu.Unlock()
	gl.handlers[phase] = append(gl.handlers[phase], handler)
}

// CurrentTick returns the current monotonically increasing tick number.
func (gl *gameLoop) CurrentTick() uint64 {
	return gl.tick.Load()
}

// TickRate returns the configured ticks per second.
func (gl *gameLoop) TickRate() int {
	return gl.config.TickRate
}

// Start begins the game loop. Returns an error if already running.
func (gl *gameLoop) Start() error {
	if !gl.running.CompareAndSwap(false, true) {
		return fmt.Errorf("game loop already running")
	}
	go gl.run()
	return nil
}

// Stop signals the game loop to finish the current tick and stop.
// It blocks until the loop has fully stopped.
func (gl *gameLoop) Stop() error {
	if !gl.running.Load() {
		return fmt.Errorf("game loop not running")
	}
	close(gl.stopCh)
	<-gl.stopped
	return nil
}

// run is the main loop goroutine.
func (gl *gameLoop) run() {
	defer close(gl.stopped)
	defer gl.running.Store(false)

	interval := time.Duration(float64(time.Second) / float64(gl.config.TickRate))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-gl.stopCh:
			// Safe stop: invoke OnStop callback for state persistence
			if gl.config.OnStop != nil {
				gl.config.OnStop()
			}
			return
		case <-ticker.C:
			gl.executeTick(interval)
		}
	}
}

// executeTick runs one complete tick through all four phases.
func (gl *gameLoop) executeTick(interval time.Duration) {
	currentTick := gl.tick.Add(1)
	start := time.Now()

	gl.mu.RLock()
	// Execute phases in order: Input → Update → Sync → Cleanup
	for phase := PhaseInput; phase <= PhaseCleanup; phase++ {
		for _, handler := range gl.handlers[phase] {
			handler(currentTick, interval)
		}
	}
	gl.mu.RUnlock()

	elapsed := time.Since(start)

	// Tick timeout detection: if processing exceeded the frame interval, log a warning.
	if elapsed > interval {
		log.Printf("[WARN] GameLoop: tick %d took %v (exceeds frame interval %v), skipping frames to catch up", currentTick, elapsed, interval)
		// Calculate how many extra frames were consumed and advance the tick counter
		// to catch up, so the logical frame number stays monotonically increasing.
		skipped := int(elapsed/interval) - 1
		if skipped > 0 {
			gl.tick.Add(uint64(skipped))
			log.Printf("[WARN] GameLoop: skipped %d frame(s) to catch up", skipped)
		}
	}
}
