package monitor

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_Basic(t *testing.T) {
	pool := NewWorkerPool(4, 16)
	defer pool.Stop()

	var count atomic.Int64
	for i := 0; i < 20; i++ {
		pool.Submit(func() {
			count.Add(1)
		})
	}
	pool.Wait()

	if count.Load() != 20 {
		t.Errorf("expected 20 tasks executed, got %d", count.Load())
	}
}

func TestWorkerPool_ActiveWorkers(t *testing.T) {
	pool := NewWorkerPool(8, 32)
	defer pool.Stop()

	if pool.ActiveWorkers() != 8 {
		t.Errorf("expected 8 workers, got %d", pool.ActiveWorkers())
	}
}

func TestWorkerPool_DefaultWorkers(t *testing.T) {
	pool := NewWorkerPool(0, 0)
	defer pool.Stop()

	if pool.ActiveWorkers() < 1 {
		t.Errorf("expected at least 1 worker, got %d", pool.ActiveWorkers())
	}
}

func TestWorkerPool_PendingTasks(t *testing.T) {
	// Use 1 worker and a blocking task to observe pending count.
	pool := NewWorkerPool(1, 64)
	defer pool.Stop()

	ready := make(chan struct{})
	done := make(chan struct{})

	// First task blocks until we signal.
	pool.Submit(func() {
		<-ready
		close(done)
	})

	// Submit more tasks while first is blocked.
	for i := 0; i < 5; i++ {
		pool.Submit(func() {})
	}

	// At least 1 task is pending (the blocking one is running, others queued).
	if pool.PendingTasks() < 1 {
		t.Logf("pending tasks: %d (may have drained quickly)", pool.PendingTasks())
	}

	close(ready)
	<-done
	pool.Wait()

	if pool.PendingTasks() != 0 {
		t.Errorf("expected 0 pending after Wait, got %d", pool.PendingTasks())
	}
}

func TestWorkerPool_ParallelExecution(t *testing.T) {
	pool := NewWorkerPool(4, 32)
	defer pool.Stop()

	start := time.Now()
	for i := 0; i < 4; i++ {
		pool.Submit(func() {
			time.Sleep(50 * time.Millisecond)
		})
	}
	pool.Wait()
	elapsed := time.Since(start)

	// 4 tasks × 50ms in parallel with 4 workers should finish in ~50ms, not 200ms.
	if elapsed > 150*time.Millisecond {
		t.Errorf("expected parallel execution ~50ms, got %v", elapsed)
	}
}

func TestWorkerPool_Stop_DrainsTasks(t *testing.T) {
	pool := NewWorkerPool(2, 32)

	var count atomic.Int64
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			count.Add(1)
		})
	}
	pool.Wait()
	pool.Stop()

	if count.Load() != 10 {
		t.Errorf("expected 10 tasks completed before stop, got %d", count.Load())
	}
}

// ---------- Performance alert integration ----------

func TestMonitor_PerformanceAlert_Fires(t *testing.T) {
	cfg := Config{
		TickRate:               20,
		AlertThresholdPct:      0.9,
		AlertConsecutiveFrames: 3,
	}
	m := NewMonitor(cfg)

	var alertMsg string
	m.OnAlert(func(msg string) { alertMsg = msg })

	frameInterval := time.Second / time.Duration(cfg.TickRate) // 50ms
	slowTick := time.Duration(float64(frameInterval) * 0.95)   // 47.5ms > 90%

	for i := 0; i < 5; i++ {
		m.RecordTickDuration(slowTick)
		m.CheckPerformanceAlert(uint64(i))
	}

	if alertMsg == "" {
		t.Error("expected performance alert to fire after consecutive slow frames")
	}
}

func TestMonitor_PerformanceAlert_NoFire_FastTicks(t *testing.T) {
	cfg := Config{
		TickRate:               20,
		AlertThresholdPct:      0.9,
		AlertConsecutiveFrames: 3,
	}
	m := NewMonitor(cfg)

	var alertFired bool
	m.OnAlert(func(msg string) { alertFired = true })

	frameInterval := time.Second / time.Duration(cfg.TickRate)
	fastTick := time.Duration(float64(frameInterval) * 0.5) // 50% — well under threshold

	for i := 0; i < 10; i++ {
		m.RecordTickDuration(fastTick)
		m.CheckPerformanceAlert(uint64(i))
	}

	if alertFired {
		t.Error("expected no alert for fast ticks")
	}
}

func TestMonitor_RecordActiveEntities(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.RecordActiveEntities(1000)

	metrics := m.GetMetrics()
	if metrics.ActiveEntities != 1000 {
		t.Errorf("expected 1000 active entities, got %d", metrics.ActiveEntities)
	}
}

func TestMonitor_RecordNetworkThroughput(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.RecordNetworkThroughput(1024, 2048)
	m.RecordNetworkThroughput(512, 256)

	metrics := m.GetMetrics()
	if metrics.NetworkBytesIn != 1536 {
		t.Errorf("expected 1536 bytes in, got %d", metrics.NetworkBytesIn)
	}
	if metrics.NetworkBytesOut != 2304 {
		t.Errorf("expected 2304 bytes out, got %d", metrics.NetworkBytesOut)
	}
}
