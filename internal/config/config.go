// Package config 为 MMRPG 游戏引擎提供配置管理功能。
// 支持通过 GoFrame gcfg 加载 YAML/JSON/TOML 配置，管理游戏数据表，
// 支持文件监听热重载，以及启动时配置校验。
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gfsnotify"
)

// ConfigManager 管理引擎配置的加载、校验、热重载和游戏数据表访问。
type ConfigManager interface {
	// Load 从配置目录读取所有配置文件。
	Load() error
	// Get 通过点分隔的键路径返回配置值。
	Get(key string) interface{}
	// GetGameTable 返回指定名称的游戏数据表（如 "skills"、"monsters"、"equipment"）。
	GetGameTable(tableName string) (GameTable, error)
	// HotReload 在不停止引擎的情况下重新读取已变更的配置文件。
	HotReload() error
	// Validate 检查所有已加载配置的格式和数据完整性。
	Validate() error
	// OnChange 注册一个回调，在热重载时某个配置键发生变化时触发。
	OnChange(handler func(key string))
}

// GameTable 提供对游戏数据表（技能表、怪物表等）的只读访问。
type GameTable interface {
	// GetRow 通过 ID 键返回单行数据。
	GetRow(id interface{}) (interface{}, bool)
	// GetAll 返回表中所有行。
	GetAll() []interface{}
	// Reload 从原始 JSON 字节替换表数据。
	Reload(data []byte) error
}

// ---------------------------------------------------------------------------
// GameTable 实现
// ---------------------------------------------------------------------------

// gameTable 以任意 ID 为键存储行数据。
type gameTable struct {
	mu   sync.RWMutex
	name string
	rows map[interface{}]interface{}
}

// newGameTable 创建一个空的游戏数据表。
func newGameTable(name string) *gameTable {
	return &gameTable{
		name: name,
		rows: make(map[interface{}]interface{}),
	}
}

func (t *gameTable) GetRow(id interface{}) (interface{}, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.rows[id]
	return v, ok
}

func (t *gameTable) GetAll() []interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]interface{}, 0, len(t.rows))
	for _, v := range t.rows {
		out = append(out, v)
	}
	return out
}

// Reload 将 JSON 数据解析为对象数组。每个对象必须包含 "id" 字段作为行键。
func (t *gameTable) Reload(data []byte) error {
	var rows []map[string]interface{}
	if err := json.Unmarshal(data, &rows); err != nil {
		return fmt.Errorf("游戏数据表 %q：JSON 格式无效：%w", t.name, err)
	}

	newRows := make(map[interface{}]interface{}, len(rows))
	for i, row := range rows {
		id, ok := row["id"]
		if !ok {
			return fmt.Errorf("游戏数据表 %q：第 %d 行缺少 'id' 字段", t.name, i)
		}
		newRows[id] = row
	}

	t.mu.Lock()
	t.rows = newRows
	t.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// ConfigManager 实现
// ---------------------------------------------------------------------------

// configManager 是 ConfigManager 的默认实现。
type configManager struct {
	mu sync.RWMutex

	// configDir 是配置文件的根目录。
	configDir string

	// gcfg 适配器，用于加载 YAML/JSON/TOML。
	adapter *gcfg.Config

	// tables 存储已加载的游戏数据表，以表名为键。
	tables map[string]*gameTable

	// tableDir 是存放游戏数据表 JSON 文件的目录。
	tableDir string

	// changeHandlers 是配置变更时触发的回调列表。
	changeHandlers []func(key string)

	// watchCallbacks 记录 gfsnotify 回调，用于清理。
	watchCallbacks []*gfsnotify.Callback
}

// Option 用于配置 configManager。
type Option func(*configManager)

// WithConfigDir 设置配置根目录（默认："config"）。
func WithConfigDir(dir string) Option {
	return func(cm *configManager) { cm.configDir = dir }
}

// WithTableDir 设置游戏数据表目录（默认："config/tables"）。
func WithTableDir(dir string) Option {
	return func(cm *configManager) { cm.tableDir = dir }
}

// NewConfigManager 使用给定选项创建新的 ConfigManager。
func NewConfigManager(opts ...Option) ConfigManager {
	cm := &configManager{
		configDir: "config",
		tableDir:  "config/tables",
		tables:    make(map[string]*gameTable),
	}
	for _, o := range opts {
		o(cm)
	}
	return cm
}

// Load 通过 gcfg 读取主配置，并从 tableDir 加载所有游戏数据表。
func (cm *configManager) Load() error {
	// 初始化 gcfg 适配器，指向 configDir。
	adapterFile, err := gcfg.NewAdapterFile()
	if err != nil {
		return fmt.Errorf("config：创建适配器失败：%w", err)
	}
	if err := adapterFile.SetPath(cm.configDir); err != nil {
		return fmt.Errorf("config：设置配置路径 %q 失败：%w", cm.configDir, err)
	}
	cm.adapter = gcfg.NewWithAdapter(adapterFile)

	// 从 tableDir 加载游戏数据表。
	if err := cm.loadTables(); err != nil {
		return err
	}

	return nil
}

// loadTables 扫描 tableDir 中的 JSON 文件，将每个文件加载为 GameTable。
func (cm *configManager) loadTables() error {
	info, err := os.Stat(cm.tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			// 数据表目录不存在，忽略。
			return nil
		}
		return fmt.Errorf("config：无法访问数据表目录 %q：%w", cm.tableDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config：数据表路径 %q 不是目录", cm.tableDir)
	}

	entries, err := os.ReadDir(cm.tableDir)
	if err != nil {
		return fmt.Errorf("config：无法读取数据表目录 %q：%w", cm.tableDir, err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		tableName := strings.TrimSuffix(name, ".json")
		data, err := os.ReadFile(filepath.Join(cm.tableDir, name))
		if err != nil {
			return fmt.Errorf("config：无法读取数据表文件 %q：%w", name, err)
		}
		tbl := newGameTable(tableName)
		if err := tbl.Reload(data); err != nil {
			return err
		}
		cm.tables[tableName] = tbl
	}
	return nil
}

// Get 通过 gcfg 适配器按键返回配置值。
func (cm *configManager) Get(key string) interface{} {
	if cm.adapter == nil {
		return nil
	}
	ctx := context.Background()
	v, err := cm.adapter.Get(ctx, key)
	if err != nil {
		return nil
	}
	return v.Val()
}

// GetGameTable 按名称返回已加载的游戏数据表。
func (cm *configManager) GetGameTable(tableName string) (GameTable, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	tbl, ok := cm.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("config：游戏数据表 %q 未找到", tableName)
	}
	return tbl, nil
}

// HotReload 重新读取所有游戏数据表文件并通知变更处理器。
func (cm *configManager) HotReload() error {
	info, err := os.Stat(cm.tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("config：无法访问数据表目录 %q：%w", cm.tableDir, err)
	}
	if !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(cm.tableDir)
	if err != nil {
		return fmt.Errorf("config：无法读取数据表目录 %q：%w", cm.tableDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		tableName := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(cm.tableDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("config：无法读取数据表文件 %q：%w", entry.Name(), err)
		}

		cm.mu.Lock()
		tbl, exists := cm.tables[tableName]
		if !exists {
			tbl = newGameTable(tableName)
			cm.tables[tableName] = tbl
		}
		cm.mu.Unlock()

		if err := tbl.Reload(data); err != nil {
			return err
		}

		cm.notifyChange("table." + tableName)
	}
	return nil
}

// Validate 检查配置目录是否存在，以及所有数据表文件是否为有效 JSON。
func (cm *configManager) Validate() error {
	// 校验配置目录。
	if _, err := os.Stat(cm.configDir); err != nil {
		return fmt.Errorf("config：配置目录 %q：%w", cm.configDir, err)
	}

	// 校验数据表文件。
	info, err := os.Stat(cm.tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 没有数据表目录是可接受的
		}
		return fmt.Errorf("config：数据表目录 %q：%w", cm.tableDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config：数据表路径 %q 不是目录", cm.tableDir)
	}

	entries, err := os.ReadDir(cm.tableDir)
	if err != nil {
		return fmt.Errorf("config：无法读取数据表目录 %q：%w", cm.tableDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(cm.tableDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("config：无法读取 %q：%w", filePath, err)
		}
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return fmt.Errorf("config：%q 中 JSON 格式无效：%w", filePath, err)
		}
		for i, row := range rows {
			if _, ok := row["id"]; !ok {
				return fmt.Errorf("config：%q 第 %d 行缺少必填字段 'id'", filePath, i)
			}
		}
	}
	return nil
}

// OnChange 注册一个回调，在配置键变更时触发。
func (cm *configManager) OnChange(handler func(key string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.changeHandlers = append(cm.changeHandlers, handler)
}

// notifyChange 调用所有已注册的变更处理器。
func (cm *configManager) notifyChange(key string) {
	cm.mu.RLock()
	handlers := make([]func(key string), len(cm.changeHandlers))
	copy(handlers, cm.changeHandlers)
	cm.mu.RUnlock()

	for _, h := range handlers {
		h(key)
	}
}

// StartWatching 开始对 tableDir 进行文件系统监听以支持热重载。
// 使用 GoFrame 的 gfsnotify 检测文件变更，并在小防抖窗口后触发 HotReload。
func (cm *configManager) StartWatching() error {
	if _, err := os.Stat(cm.tableDir); os.IsNotExist(err) {
		return nil // 没有需要监听的目录
	}

	var debounceTimer *time.Timer
	var debounceMu sync.Mutex

	cb, err := gfsnotify.Add(cm.tableDir, func(event *gfsnotify.Event) {
		debounceMu.Lock()
		defer debounceMu.Unlock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(200*time.Millisecond, func() {
			_ = cm.HotReload()
		})
	})
	if err != nil {
		return fmt.Errorf("config：监听 %q 失败：%w", cm.tableDir, err)
	}

	cm.mu.Lock()
	cm.watchCallbacks = append(cm.watchCallbacks, cb)
	cm.mu.Unlock()
	return nil
}

// StopWatching 移除所有文件监听器。
func (cm *configManager) StopWatching() {
	cm.mu.Lock()
	callbacks := cm.watchCallbacks
	cm.watchCallbacks = nil
	cm.mu.Unlock()

	for _, cb := range callbacks {
		_ = gfsnotify.RemoveCallback(cb.Id)
	}
}
