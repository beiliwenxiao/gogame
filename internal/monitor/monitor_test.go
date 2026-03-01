package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Logger tests
// ---------------------------------------------------------------------------

func TestNewLogger(t *testing.T) {
	l := NewLogger("engine")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if l.module != "engine" {
		t.Errorf("expected module 'engine', got %q", l.module)
	}
}

func TestLogger_Levels(t *testing.T) {
	// Smoke test: calling each level should not panic.
	l := NewLogger("test")
	ctx := context.Background()
	l.Debug(ctx, "debug %d", 1)
	l.Info(ctx, "info %s", "hello")
	l.Warn(ctx, "warn")
	l.Error(ctx, "error %v", "oops")
}

func TestLogger_SetLevel(t *testing.T) {
	l := NewLogger("test")
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		l.SetLevel(level) // should not panic
	}
}

func TestGoroutineID(t *testing.T) {
	id := goroutineID()
	if id == 0 {
		t.Error("expected non-zero goroutine ID")
	}
}

func TestLogger_Prefix(t *testing.T) {
	l := NewLogger("combat")
	p := l.prefix()
	if len(p) == 0 {
		t.Fatal("expected non-empty prefix")
	}
	// Should contain module name.
	if !containsStr(p, "combat") {
		t.Errorf("prefix %q should contain 'combat'", p)
	}
	// Should contain gid.
	if !containsStr(p, "gid:") {
		t.Errorf("prefix %q should contain 'gid:'", p)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Monitor / Metrics tests
// ---------------------------------------------------------------------------

func TestNewMonitor(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	if m == nil {
		t.Fatal("expected non-nil monitor")
	}
}

func TestRecordTickDuration(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.RecordTickDuration(10 * time.Millisecond)
	m.RecordTickDuration(20 * time.Millisecond)

	metrics := m.GetMetrics()
	// Average of 10ms and 20ms = 15ms.
	if metrics.AvgTickDuration != 15*time.Millisecond {
		t.Errorf("expected 15ms avg, got %v", metrics.AvgTickDuration)
	}
}

func TestRecordTickDuration_RollingWindow(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	// Fill the entire 100-slot window with 50ms.
	for i := 0; i < 100; i++ {
		m.RecordTickDuration(50 * time.Millisecond)
	}
	// Now add one more — should wrap around.
	m.RecordTickDuration(150 * time.Millisecond)
	metrics := m.GetMetrics()
	// 99 * 50ms + 150ms = 5100ms / 100 = 51ms
	expected := 51 * time.Millisecond
	if metrics.AvgTickDuration != expected {
		t.Errorf("expected %v avg, got %v", expected, metrics.AvgTickDuration)
	}
}

func TestRecordActiveEntities(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.RecordActiveEntities(500)
	metrics := m.GetMetrics()
	if metrics.ActiveEntities != 500 {
		t.Errorf("expected 500, got %d", metrics.ActiveEntities)
	}
}

func TestRecordNetworkThroughput(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.RecordNetworkThroughput(1000, 2000)
	m.RecordNetworkThroughput(500, 300)
	metrics := m.GetMetrics()
	if metrics.NetworkBytesIn != 1500 {
		t.Errorf("expected 1500 bytes in, got %d", metrics.NetworkBytesIn)
	}
	if metrics.NetworkBytesOut != 2300 {
		t.Errorf("expected 2300 bytes out, got %d", metrics.NetworkBytesOut)
	}
}

func TestSetOnlineClients(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.SetOnlineClients(42)
	metrics := m.GetMetrics()
	if metrics.OnlineClients != 42 {
		t.Errorf("expected 42, got %d", metrics.OnlineClients)
	}
}

func TestSetRoomCount(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.SetRoomCount(7)
	metrics := m.GetMetrics()
	if metrics.RoomCount != 7 {
		t.Errorf("expected 7, got %d", metrics.RoomCount)
	}
}

func TestGetMetrics_MemoryUsage(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	metrics := m.GetMetrics()
	if metrics.MemoryUsage == 0 {
		t.Error("expected non-zero memory usage")
	}
}

func TestGetMetrics_ZeroTicks(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	metrics := m.GetMetrics()
	if metrics.AvgTickDuration != 0 {
		t.Errorf("expected 0 avg tick duration with no data, got %v", metrics.AvgTickDuration)
	}
}

// ---------------------------------------------------------------------------
// Performance alert tests
// ---------------------------------------------------------------------------

func TestCheckPerformanceAlert_NoAlert(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TickRate = 20 // 50ms frame interval
	m := NewMonitor(cfg)

	var alertFired bool
	m.OnAlert(func(msg string) { alertFired = true })

	// Record a fast tick (well under 90% of 50ms = 45ms).
	m.RecordTickDuration(10 * time.Millisecond)
	m.CheckPerformanceAlert(1)

	if alertFired {
		t.Error("alert should not fire for fast ticks")
	}
}

func TestCheckPerformanceAlert_Fires(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TickRate = 20
	cfg.AlertThresholdPct = 0.9
	cfg.AlertConsecutiveFrames = 3
	m := NewMonitor(cfg)

	var alertMsg string
	m.OnAlert(func(msg string) { alertMsg = msg })

	// Record 3 slow ticks (> 45ms each).
	for i := 0; i < 3; i++ {
		m.RecordTickDuration(48 * time.Millisecond)
		m.CheckPerformanceAlert(uint64(i + 1))
	}

	if alertMsg == "" {
		t.Error("expected alert to fire after 3 consecutive slow frames")
	}
}

func TestCheckPerformanceAlert_ResetOnFastTick(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TickRate = 20
	cfg.AlertThresholdPct = 0.9
	cfg.AlertConsecutiveFrames = 3
	m := NewMonitor(cfg)

	var alertFired bool
	m.OnAlert(func(msg string) { alertFired = true })

	// 2 slow ticks, then 1 fast tick — should reset counter.
	m.RecordTickDuration(48 * time.Millisecond)
	m.CheckPerformanceAlert(1)
	m.RecordTickDuration(48 * time.Millisecond)
	m.CheckPerformanceAlert(2)
	m.RecordTickDuration(5 * time.Millisecond)
	m.CheckPerformanceAlert(3)

	// Then 2 more slow — still not enough.
	m.RecordTickDuration(48 * time.Millisecond)
	m.CheckPerformanceAlert(4)
	m.RecordTickDuration(48 * time.Millisecond)
	m.CheckPerformanceAlert(5)

	if alertFired {
		t.Error("alert should not fire — counter was reset")
	}
}

func TestCheckPerformanceAlert_ZeroTickRate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TickRate = 0
	m := NewMonitor(cfg)
	// Should not panic.
	m.RecordTickDuration(100 * time.Millisecond)
	m.CheckPerformanceAlert(1)
}

// ---------------------------------------------------------------------------
// Concurrency test
// ---------------------------------------------------------------------------

func TestMonitor_ConcurrentAccess(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.RecordTickDuration(time.Duration(n) * time.Millisecond)
			m.RecordActiveEntities(n)
			m.RecordNetworkThroughput(int64(n), int64(n*2))
			m.SetOnlineClients(n)
			m.SetRoomCount(n)
			_ = m.GetMetrics()
			m.CheckPerformanceAlert(uint64(n))
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// HTTP metrics endpoint tests
// ---------------------------------------------------------------------------

func TestHandleMetrics(t *testing.T) {
	m := NewMonitor(DefaultConfig())
	m.SetOnlineClients(10)
	m.SetRoomCount(3)
	m.RecordActiveEntities(200)
	m.RecordTickDuration(25 * time.Millisecond)
	m.RecordNetworkThroughput(1024, 2048)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	m.handleMetrics(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	var metrics Metrics
	if err := json.Unmarshal(body, &metrics); err != nil {
		t.Fatalf("failed to decode metrics JSON: %v", err)
	}
	if metrics.OnlineClients != 10 {
		t.Errorf("expected 10 online clients, got %d", metrics.OnlineClients)
	}
	if metrics.RoomCount != 3 {
		t.Errorf("expected 3 rooms, got %d", metrics.RoomCount)
	}
	if metrics.ActiveEntities != 200 {
		t.Errorf("expected 200 entities, got %d", metrics.ActiveEntities)
	}
	if metrics.NetworkBytesIn != 1024 {
		t.Errorf("expected 1024 bytes in, got %d", metrics.NetworkBytesIn)
	}
	if metrics.NetworkBytesOut != 2048 {
		t.Errorf("expected 2048 bytes out, got %d", metrics.NetworkBytesOut)
	}
}

// ---------------------------------------------------------------------------
// Webhook alert tests
// ---------------------------------------------------------------------------

func TestSendWebhookAlert_NoURL(t *testing.T) {
	m := NewMonitor(DefaultConfig()) // no WebhookURL
	m.SendWebhookAlert("engine", "test error")
	if m.WebhookSentCount() != 0 {
		t.Error("should not send webhook when URL is empty")
	}
}

func TestSendWebhookAlert_Success(t *testing.T) {
	var received WebhookPayload
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.WebhookURL = ts.URL
	m := NewMonitor(cfg)

	m.SendWebhookAlert("combat", "critical error in combat system")

	if m.WebhookSentCount() != 1 {
		t.Errorf("expected 1 webhook sent, got %d", m.WebhookSentCount())
	}
	if received.Level != "ERROR" {
		t.Errorf("expected ERROR level, got %q", received.Level)
	}
	if received.Module != "combat" {
		t.Errorf("expected module 'combat', got %q", received.Module)
	}
	if received.Message != "critical error in combat system" {
		t.Errorf("unexpected message: %q", received.Message)
	}
	if received.Time == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestSendWebhookAlert_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.WebhookURL = ts.URL
	m := NewMonitor(cfg)

	// Should not panic even on server error.
	m.SendWebhookAlert("network", "connection pool exhausted")
	// The webhook was still sent (we count the attempt).
	if m.WebhookSentCount() != 1 {
		t.Errorf("expected 1 webhook sent, got %d", m.WebhookSentCount())
	}
}

// ---------------------------------------------------------------------------
// DefaultConfig test
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.TickRate != 20 {
		t.Errorf("expected tick rate 20, got %d", cfg.TickRate)
	}
	if cfg.AlertThresholdPct != 0.9 {
		t.Errorf("expected 0.9 threshold, got %f", cfg.AlertThresholdPct)
	}
	if cfg.AlertConsecutiveFrames != 10 {
		t.Errorf("expected 10 consecutive frames, got %d", cfg.AlertConsecutiveFrames)
	}
	if cfg.HTTPAddr != ":9100" {
		t.Errorf("expected :9100, got %q", cfg.HTTPAddr)
	}
}
