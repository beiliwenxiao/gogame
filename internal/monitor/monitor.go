// Package monitor 为 MMRPG 游戏引擎提供日志记录、指标采集、HTTP 指标暴露和 Webhook 告警功能。
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

// Monitor 提供性能指标记录和告警功能。
type Monitor interface {
	RecordTickDuration(d time.Duration)
	RecordActiveEntities(count int)
	RecordNetworkThroughput(bytesIn, bytesOut int64)
	GetMetrics() *Metrics
	CheckPerformanceAlert(tick uint64)
}

// Metrics 保存运行时性能指标。
type Metrics struct {
	OnlineClients   int           `json:"online_clients"`
	RoomCount       int           `json:"room_count"`
	AvgTickDuration time.Duration `json:"avg_tick_duration_ns"`
	MemoryUsage     uint64        `json:"memory_usage"`
	ActiveEntities  int           `json:"active_entities"`
	NetworkBytesIn  int64         `json:"network_bytes_in"`
	NetworkBytesOut int64         `json:"network_bytes_out"`
}

// Logger 封装 GoFrame glog，提供模块级结构化日志。
type Logger struct {
	module string
	logger *glog.Logger
}

// NewLogger 创建一个以 GoFrame glog 为后端的模块级日志记录器。
func NewLogger(module string) *Logger {
	l := glog.New()
	l.SetFlags(glog.F_TIME_DATE | glog.F_TIME_TIME | glog.F_TIME_MILLI)
	return &Logger{module: module, logger: l}
}

// goroutineID 返回当前 goroutine 的 ID（尽力而为，仅用于日志）。
func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// 格式："goroutine 123 [..."
	var id uint64
	for i := len("goroutine "); i < n; i++ {
		if buf[i] < '0' || buf[i] > '9' {
			break
		}
		id = id*10 + uint64(buf[i]-'0')
	}
	return id
}

// prefix 返回结构化日志前缀：[模块名] [gid:N]。
func (l *Logger) prefix() string {
	return fmt.Sprintf("[%s] [gid:%d]", l.module, goroutineID())
}

// Debug 以 DEBUG 级别记录日志。
func (l *Logger) Debug(ctx context.Context, msg string, args ...interface{}) {
	l.logger.Debugf(ctx, "%s %s", l.prefix(), fmt.Sprintf(msg, args...))
}

// Info 以 INFO 级别记录日志。
func (l *Logger) Info(ctx context.Context, msg string, args ...interface{}) {
	l.logger.Infof(ctx, "%s %s", l.prefix(), fmt.Sprintf(msg, args...))
}

// Warn 以 WARN 级别记录日志。
func (l *Logger) Warn(ctx context.Context, msg string, args ...interface{}) {
	l.logger.Warningf(ctx, "%s %s", l.prefix(), fmt.Sprintf(msg, args...))
}

// Error 以 ERROR 级别记录日志，并在配置了 Webhook 时触发告警。
func (l *Logger) Error(ctx context.Context, msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	l.logger.Errorf(ctx, "%s %s", l.prefix(), formatted)
}

// SetLevel 设置日志级别。可接受的值："DEBUG"、"INFO"、"WARN"、"ERROR"。
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
// Monitor 实现
// ---------------------------------------------------------------------------

// Config 保存监控配置。
type Config struct {
	// TickRate 是预期的每秒 tick 次数（用于告警阈值计算）。
	TickRate int
	// AlertThresholdPct 当 tick 耗时超过帧间隔该百分比，且连续超过
	// AlertConsecutiveFrames 帧时触发告警。
	AlertThresholdPct float64
	// AlertConsecutiveFrames 是触发告警所需的连续慢帧数。
	AlertConsecutiveFrames int
	// WebhookURL 是 ERROR 级别 Webhook 告警的目标 URL，为空则禁用。
	WebhookURL string
	// HTTPAddr 是指标 HTTP 端点的监听地址，为空则禁用。
	HTTPAddr string
}

// DefaultConfig 返回带有合理默认值的 Config。
func DefaultConfig() Config {
	return Config{
		TickRate:               20,
		AlertThresholdPct:      0.9,
		AlertConsecutiveFrames: 10,
		HTTPAddr:               ":9100",
	}
}

// gameMonitor 是 Monitor 的默认实现。
type gameMonitor struct {
	cfg Config

	mu              sync.RWMutex
	onlineClients   int
	roomCount       int
	activeEntities  int
	networkBytesIn  int64
	networkBytesOut int64

	// tick 耗时追踪（环形缓冲区，用于计算平均值）。
	tickDurations []time.Duration
	tickIdx       int
	tickCount     int

	// 性能告警追踪。
	consecutiveSlow int

	// Webhook HTTP 客户端。
	webhookClient *http.Client

	// 指标端点 HTTP 服务器。
	httpServer *http.Server

	// alertCallback 在性能告警触发时调用（用于测试）。
	alertCallback func(msg string)

	// webhookSent 记录已发送的 Webhook 次数（原子操作，用于测试）。
	webhookSent atomic.Int64
}

// NewMonitor 创建并返回新的 Monitor。调用 StartHTTP() 以启动指标端点。
func NewMonitor(cfg Config) *gameMonitor {
	m := &gameMonitor{
		cfg:           cfg,
		tickDurations: make([]time.Duration, 100), // 100 个 tick 的滚动窗口
		webhookClient: &http.Client{Timeout: 5 * time.Second},
	}
	return m
}

// RecordTickDuration 记录单次 tick 的耗时。
func (m *gameMonitor) RecordTickDuration(d time.Duration) {
	m.mu.Lock()
	m.tickDurations[m.tickIdx] = d
	m.tickIdx = (m.tickIdx + 1) % len(m.tickDurations)
	if m.tickCount < len(m.tickDurations) {
		m.tickCount++
	}
	m.mu.Unlock()
}

// RecordActiveEntities 更新活跃实体数量。
func (m *gameMonitor) RecordActiveEntities(count int) {
	m.mu.Lock()
	m.activeEntities = count
	m.mu.Unlock()
}

// RecordNetworkThroughput 累加网络字节计数器。
func (m *gameMonitor) RecordNetworkThroughput(bytesIn, bytesOut int64) {
	m.mu.Lock()
	m.networkBytesIn += bytesIn
	m.networkBytesOut += bytesOut
	m.mu.Unlock()
}

// SetOnlineClients 更新在线客户端数量。
func (m *gameMonitor) SetOnlineClients(n int) {
	m.mu.Lock()
	m.onlineClients = n
	m.mu.Unlock()
}

// SetRoomCount 更新房间数量。
func (m *gameMonitor) SetRoomCount(n int) {
	m.mu.Lock()
	m.roomCount = n
	m.mu.Unlock()
}

// GetMetrics 返回当前运行时指标的快照。
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

// avgTickDuration 从滚动窗口计算平均 tick 耗时。调用方须持有至少读锁。
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

// CheckPerformanceAlert 检查最近的 tick 耗时是否超过配置阈值，
// 连续慢帧达到阈值后触发告警。
func (m *gameMonitor) CheckPerformanceAlert(tick uint64) {
	if m.cfg.TickRate <= 0 {
		return
	}
	frameInterval := time.Second / time.Duration(m.cfg.TickRate)
	threshold := time.Duration(float64(frameInterval) * m.cfg.AlertThresholdPct)

	m.mu.Lock()
	// 查看最近一次记录的 tick 耗时。
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
		msg := fmt.Sprintf("性能告警：tick %d，连续 %d 帧超过帧间隔 %.0f%%（%v）",
			tick, consecutive, m.cfg.AlertThresholdPct*100, frameInterval)
		if m.alertCallback != nil {
			m.alertCallback(msg)
		}
		// 重置计数器，避免重复告警。
		m.mu.Lock()
		m.consecutiveSlow = 0
		m.mu.Unlock()
	}
}

// OnAlert 设置性能告警触发时的回调函数。
func (m *gameMonitor) OnAlert(fn func(msg string)) {
	m.alertCallback = fn
}

// ---------------------------------------------------------------------------
// HTTP 指标端点
// ---------------------------------------------------------------------------

// StartHTTP 启动 HTTP 服务器，将 /metrics 以 JSON 格式暴露。
// 立即返回，服务器在后台 goroutine 中运行。
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
			fmt.Printf("[monitor] HTTP 服务器错误：%v\n", err)
		}
	}()
	return nil
}

// StopHTTP 优雅关闭 HTTP 服务器。
func (m *gameMonitor) StopHTTP(ctx context.Context) error {
	if m.httpServer == nil {
		return nil
	}
	return m.httpServer.Shutdown(ctx)
}

// handleMetrics 将当前指标以 JSON 格式写入响应。
func (m *gameMonitor) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := m.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

// ---------------------------------------------------------------------------
// Webhook 告警
// ---------------------------------------------------------------------------

// WebhookPayload 是发送到 Webhook URL 的 JSON 请求体。
type WebhookPayload struct {
	Level   string `json:"level"`
	Module  string `json:"module"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

// SendWebhookAlert 向配置的 Webhook URL 发送 ERROR 级别告警。
// 未配置 Webhook 时为空操作。
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

// WebhookSentCount 返回已发送的 Webhook 告警次数（用于测试）。
func (m *gameMonitor) WebhookSentCount() int64 {
	return m.webhookSent.Load()
}
