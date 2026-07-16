/*************************************************************
 * Copyright (c) 2026 Liu Xiao (beiliwenxiao)
 *
 * @project   YiJian18-Server 多人实时战斗游戏后端引擎
 * @author    刘枭 (beiliwenxiao)
 * @email     beiliwenxiao@qq.com
 * @date      2026-03-01
 * @blog      https://blog.csdn.net/beiliwenxiao
 * @repo      https://github.com/beiliwenxiao/yijian18-server
 *            https://gitee.com/coderaaa/yijian18-server
 *************************************************************/

// Package monitor 为 MMRPG 游戏引擎提供日志记录、指标采集、HTTP 指标暴露和 Webhook 告警功能。
package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
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

// LogLevel 日志级别
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger 封装标准库 log，提供模块级结构化日志。
type Logger struct {
	module string
	level  LogLevel
}

// NewLogger 创建一个模块级日志记录器。
func NewLogger(module string) *Logger {
	return &Logger{module: module, level: LevelDebug}
}

// goroutineID 返回当前 goroutine 的 ID（尽力而为，仅用于日志）。
func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
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
func (l *Logger) Debug(_ context.Context, msg string, args ...interface{}) {
	if l.level <= LevelDebug {
		log.Printf("DEBUG %s %s", l.prefix(), fmt.Sprintf(msg, args...))
	}
}

// Info 以 INFO 级别记录日志。
func (l *Logger) Info(_ context.Context, msg string, args ...interface{}) {
	if l.level <= LevelInfo {
		log.Printf("INFO  %s %s", l.prefix(), fmt.Sprintf(msg, args...))
	}
}

// Warn 以 WARN 级别记录日志。
func (l *Logger) Warn(_ context.Context, msg string, args ...interface{}) {
	if l.level <= LevelWarn {
		log.Printf("WARN  %s %s", l.prefix(), fmt.Sprintf(msg, args...))
	}
}

// Error 以 ERROR 级别记录日志。
func (l *Logger) Error(_ context.Context, msg string, args ...interface{}) {
	if l.level <= LevelError {
		log.Printf("ERROR %s %s", l.prefix(), fmt.Sprintf(msg, args...))
	}
}

// SetLevel 设置日志级别。可接受的值："DEBUG"、"INFO"、"WARN"、"ERROR"。
func (l *Logger) SetLevel(level string) {
	switch level {
	case "DEBUG":
		l.level = LevelDebug
	case "INFO":
		l.level = LevelInfo
	case "WARN":
		l.level = LevelWarn
	case "ERROR":
		l.level = LevelError
	}
}

// ---------------------------------------------------------------------------
// Monitor 实现
// ---------------------------------------------------------------------------

// Config 保存监控配置。
type Config struct {
	TickRate               int
	AlertThresholdPct      float64
	AlertConsecutiveFrames int
	WebhookURL             string
	HTTPAddr               string
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

	tickDurations []time.Duration
	tickIdx       int
	tickCount     int

	consecutiveSlow int

	webhookClient *http.Client
	httpServer    *http.Server
	alertCallback func(msg string)
	webhookSent   atomic.Int64
}

// NewMonitor 创建并返回新的 Monitor。
func NewMonitor(cfg Config) *gameMonitor {
	return &gameMonitor{
		cfg:           cfg,
		tickDurations: make([]time.Duration, 100),
		webhookClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (m *gameMonitor) RecordTickDuration(d time.Duration) {
	m.mu.Lock()
	m.tickDurations[m.tickIdx] = d
	m.tickIdx = (m.tickIdx + 1) % len(m.tickDurations)
	if m.tickCount < len(m.tickDurations) {
		m.tickCount++
	}
	m.mu.Unlock()
}

func (m *gameMonitor) RecordActiveEntities(count int) {
	m.mu.Lock()
	m.activeEntities = count
	m.mu.Unlock()
}

func (m *gameMonitor) RecordNetworkThroughput(bytesIn, bytesOut int64) {
	m.mu.Lock()
	m.networkBytesIn += bytesIn
	m.networkBytesOut += bytesOut
	m.mu.Unlock()
}

func (m *gameMonitor) SetOnlineClients(n int) {
	m.mu.Lock()
	m.onlineClients = n
	m.mu.Unlock()
}

func (m *gameMonitor) SetRoomCount(n int) {
	m.mu.Lock()
	m.roomCount = n
	m.mu.Unlock()
}

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

func (m *gameMonitor) CheckPerformanceAlert(tick uint64) {
	if m.cfg.TickRate <= 0 {
		return
	}
	frameInterval := time.Second / time.Duration(m.cfg.TickRate)
	threshold := time.Duration(float64(frameInterval) * m.cfg.AlertThresholdPct)

	m.mu.Lock()
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
		m.mu.Lock()
		m.consecutiveSlow = 0
		m.mu.Unlock()
	}
}

func (m *gameMonitor) OnAlert(fn func(msg string)) {
	m.alertCallback = fn
}

// ---------------------------------------------------------------------------
// HTTP 指标端点
// ---------------------------------------------------------------------------

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

func (m *gameMonitor) StopHTTP(ctx context.Context) error {
	if m.httpServer == nil {
		return nil
	}
	return m.httpServer.Shutdown(ctx)
}

func (m *gameMonitor) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := m.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

// ---------------------------------------------------------------------------
// Webhook 告警
// ---------------------------------------------------------------------------

type WebhookPayload struct {
	Level   string `json:"level"`
	Module  string `json:"module"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

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

func (m *gameMonitor) WebhookSentCount() int64 {
	return m.webhookSent.Load()
}
