// Package monitor provides logging, metrics collection, HTTP metrics exposure,
// and webhook alerting for the MMRPG game engine.
package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/os/glog"
)

// Monitor provides performance metrics recording and alerting.
type Monitor interface {
	RecordTickDuration(d time.Duration)
	RecordActiveEntities(count int)
	RecordNetworkThroughput(bytesIn, bytesOut int64)
	GetMetrics() *Metrics
	CheckPerformanceAlert(tick uint64)
}

// Metrics holds runtime performance indicators.
type Metrics struct {
	OnlineClients   int           `json:"online_clients"`
	RoomCount       int           `json:"room_count"`
	AvgTickDuration time.Duration `json:"avg_tick_duration_ns"`
	MemoryUsage     uint64        `json:"memory_usage"`
	ActiveEntities  int           `json:"active_entities"`
	NetworkBytesIn  int64         `json:"network_bytes_in"`
	NetworkBytesOut int64         `json:"network_bytes_out"`
}

// Logger wraps GoFrame glog with module-scoped, structured logging.
type Logger struct {
	module string
	logger *glog.Logger
}

// NewLogger creates a module-scoped logger backed by GoFrame glog.
func NewLogger(module string) *Logger {
	l := glog.New()
	l.SetFlags(glog.F_TIME_DATE | glog.F_TIME_TIME | glog.F_TIME_MILLI)
	return &Logger{module: module, logger: l}
}

// goroutineID returns the current goroutine ID (best-effort, for logging only).
func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Format: "goroutine 123 [..."
	var id uint64
	for i := len("goroutine "); i < n; i++ {
		if buf[i] < '0' || buf[i] > '9' {
			break
		}
		id = id*10 + uint64(buf[i]-'0')
	}
	return id
}

// prefix returns the structured log prefix: [module] [gid:N].
func (l *Logger) prefix() string {
	return fmt.Sprintf("[%s] [gid:%d]", l.module, goroutineID())
}

// Debug logs at DEBUG level.
func (l *Logger) Debug(ctx context.Context, msg string, args ...interface{}) {
	l.logger.Debugf(ctx, "%s %s", l.prefix(), fmt.Sprintf(msg, args...))
}

// Info logs at INFO level.
func (l *Logger) Info(ctx context.Context, msg string, args ...interface{}) {
	l.logger.Infof(ctx, "%s %s", l.prefix(), fmt.Sprintf(msg, args...))
}

// Warn logs at WARN level.
func (l *Logger) Warn(ctx context.Context, msg string, args ...interface{}) {
	l.logger.Warningf(ctx, "%s %s", l.prefix(), fmt.Sprintf(msg, args...))
}

// Error logs at ERROR level and triggers webhook alert if configured.
func (l *Logger) Error(ctx context.Context, msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	l.logger.Errorf(ctx, "%s %s", l.prefix(), formatted)
}

// SetLevel sets the log level. Accepted values: "DEBUG", "INFO", "WARN", "ERROR".
func (l *Logger) SetLevel(level string) {
	switch level {
	case "DEBUG":
		l.logger.SetLevel(glog.LEVEL_ALL)
	case "INFO":
		l.logger.SetLevel(glog.LEVEL_INFO | glog.LEVEL_WARN | glog.LEVEL_ERRO)
	case "WARN":
		l.logger.SetLevel(glog.LEVEL_WARN | glog.LEVEL_ERRO)
	case "ERROR":
		l.logger.SetLevel(glog.LEVEL_ERRO)
	}
}

// ---------------------------------------------------------------------------
// Monitor implementation
// ---------------------------------------------------------------------------

// Config holds monitor configuration.
type Config struct {
	// TickRate is the expected ticks per second (used for alert threshold).
	TickRate int
	// AlertThresholdPct triggers an alert when tick duration exceeds this
	// percentage of the frame interval for AlertConsecutiveFrames in a row.
	AlertThresholdPct float64
	// AlertConsecutiveFrames is how many consecutive slow frames trigger an alert.
	AlertConsecutiveFrames int
	// WebhookURL is the URL for ERROR-level webhook alerts. Empty disables.
	WebhookURL string
	// HTTPAddr is the listen address for the metrics HTTP endpoint. Empty disables.
	HTTPAddr string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		TickRate:               20,
		AlertThresholdPct:      0.9,
		AlertConsecutiveFrames: 10,
		HTTPAddr:               ":9100",
	}
}

// gameMonitor is the default Monitor implementation.
type gameMonitor struct {
	cfg Config

	mu              sync.RWMutex
	onlineClients   int
	roomCount       int
	activeEntities  int
	networkBytesIn  int64
	networkBytesOut int64

	// Tick duration tracking (ring buffer for averaging).
	tickDurations []time.Duration
	tickIdx       int
	tickCount     int

	// Performance alert tracking.
	consecutiveSlow int

	// Webhook HTTP client.
	webhookClient *http.Client

	// HTTP server for metrics endpoint.
	httpServer *http.Server

	// alertCallback is invoked when a performance alert fires (for testing).
	alertCallback func(msg string)

	// webhookSent tracks the number of webhook calls (atomic, for testing).
	webhookSent atomic.Int64
}

// NewMonitor creates and returns a new Monitor. Call StartHTTP() separately
// to begin serving the metrics endpoint.
func NewMonitor(cfg Config) *gameMonitor {
	m := &gameMonitor{
		cfg:           cfg,
		tickDurations: make([]time.Duration, 100), // rolling window of 100 ticks
		webhookClient: &http.Client{Timeout: 5 * time.Second},
	}
	return m
}

// RecordTickDuration records the duration of a single tick.
func (m *gameMonitor) RecordTickDuration(d time.Duration) {
	m.mu.Lock()
	m.tickDurations[m.tickIdx] = d
	m.tickIdx = (m.tickIdx + 1) % len(m.tickDurations)
	if m.tickCount < len(m.tickDurations) {
		m.tickCount++
	}
	m.mu.Unlock()
}

// RecordActiveEntities updates the active entity count.
func (m *gameMonitor) RecordActiveEntities(count int) {
	m.mu.Lock()
	m.activeEntities = count
	m.mu.Unlock()
}

// RecordNetworkThroughput adds to the cumulative network byte counters.
func (m *gameMonitor) RecordNetworkThroughput(bytesIn, bytesOut int64) {
	m.mu.Lock()
	m.networkBytesIn += bytesIn
	m.networkBytesOut += bytesOut
	m.mu.Unlock()
}

// SetOnlineClients updates the online client count.
func (m *gameMonitor) SetOnlineClients(n int) {
	m.mu.Lock()
	m.onlineClients = n
	m.mu.Unlock()
}

// SetRoomCount updates the room count.
func (m *gameMonitor) SetRoomCount(n int) {
	m.mu.Lock()
	m.roomCount = n
	m.mu.Unlock()
}

// GetMetrics returns a snapshot of current runtime metrics.
func (m *gameMonitor) GetMetrics() *Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &Metrics{
		OnlineClients:   m.onlineClients,
		RoomCount:       m.roomCount,
		AvgTickDuration: m.avgTickDuration(),
		MemoryUsage:     memStats.Alloc,
		ActiveEntities:  m.activeEntities,
		NetworkBytesIn:  m.networkBytesIn,
		NetworkBytesOut: m.networkBytesOut,
	}
}

// avgTickDuration computes the average tick duration from the rolling window.
// Caller must hold at least a read lock.
func (m *gameMonitor) avgTickDuration() time.Duration {
	if m.tickCount == 0 {
		return 0
	}
	var total time.Duration
	count := m.tickCount
	for i := 0; i < count; i++ {
		total += m.tickDurations[i]
	}
	return total / time.Duration(count)
}

// CheckPerformanceAlert checks whether the latest tick durations exceed the
// configured threshold and fires an alert after consecutive slow frames.
func (m *gameMonitor) CheckPerformanceAlert(tick uint64) {
	if m.cfg.TickRate <= 0 {
		return
	}
	frameInterval := time.Second / time.Duration(m.cfg.TickRate)
	threshold := time.Duration(float64(frameInterval) * m.cfg.AlertThresholdPct)

	m.mu.Lock()
	// Look at the most recently recorded tick duration.
	lastIdx := m.tickIdx - 1
	if lastIdx < 0 {
		lastIdx = len(m.tickDurations) - 1
	}
	lastDuration := m.tickDurations[lastIdx]

	if lastDuration > threshold {
		m.consecutiveSlow++
	} else {
		m.consecutiveSlow = 0
	}
	consecutive := m.consecutiveSlow
	m.mu.Unlock()

	if consecutive >= m.cfg.AlertConsecutiveFrames {
		msg := fmt.Sprintf("performance alert: tick %d, %d consecutive frames exceeded %.0f%% of frame interval (%v)",
			tick, consecutive, m.cfg.AlertThresholdPct*100, frameInterval)
		if m.alertCallback != nil {
			m.alertCallback(msg)
		}
		// Reset counter to avoid spamming.
		m.mu.Lock()
		m.consecutiveSlow = 0
		m.mu.Unlock()
	}
}

// OnAlert sets a callback invoked when a performance alert fires.
func (m *gameMonitor) OnAlert(fn func(msg string)) {
	m.alertCallback = fn
}

// ---------------------------------------------------------------------------
// HTTP metrics endpoint
// ---------------------------------------------------------------------------

// StartHTTP starts the HTTP server exposing /metrics as JSON.
// Returns immediately; the server runs in a background goroutine.
func (m *gameMonitor) StartHTTP() error {
	if m.cfg.HTTPAddr == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", m.handleMetrics)
	m.httpServer = &http.Server{
		Addr:    m.cfg.HTTPAddr,
		Handler: mux,
	}
	go func() {
		if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Best-effort log; in production this would use the Logger.
			fmt.Printf("[monitor] HTTP server error: %v\n", err)
		}
	}()
	return nil
}

// StopHTTP gracefully shuts down the HTTP server.
func (m *gameMonitor) StopHTTP(ctx context.Context) error {
	if m.httpServer == nil {
		return nil
	}
	return m.httpServer.Shutdown(ctx)
}

// handleMetrics writes the current metrics as JSON.
func (m *gameMonitor) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := m.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

// ---------------------------------------------------------------------------
// Webhook alerting
// ---------------------------------------------------------------------------

// WebhookPayload is the JSON body sent to the webhook URL.
type WebhookPayload struct {
	Level   string `json:"level"`
	Module  string `json:"module"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

// SendWebhookAlert posts an ERROR-level alert to the configured webhook URL.
// It is safe to call even when no webhook is configured (no-op).
func (m *gameMonitor) SendWebhookAlert(module, message string) {
	if m.cfg.WebhookURL == "" {
		return
	}
	payload := WebhookPayload{
		Level:   "ERROR",
		Module:  module,
		Message: message,
		Time:    time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	resp, err := m.webhookClient.Post(m.cfg.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	resp.Body.Close()
	m.webhookSent.Add(1)
}

// WebhookSentCount returns the number of webhook alerts sent (for testing).
func (m *gameMonitor) WebhookSentCount() int64 {
	return m.webhookSent.Load()
}
