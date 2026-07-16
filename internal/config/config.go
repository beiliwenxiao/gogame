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

// Package config 为 MMRPG 游戏引擎提供配置管理功能。
// 支持加载 JSON 配置文件，管理游戏数据表，
// 支持文件监听热重载，以及启动时配置校验。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ConfigManager 管理引擎配置的加载、校验、热重载和游戏数据表访问。
type ConfigManager interface {
	Load() error
	Get(key string) interface{}
	GetGameTable(tableName string) (GameTable, error)
	HotReload() error
	Validate() error
	OnChange(handler func(key string))
}

// GameTable 提供对游戏数据表的只读访问。
type GameTable interface {
	GetRow(id interface{}) (interface{}, bool)
	GetAll() []interface{}
	Reload(data []byte) error
}

// ---------------------------------------------------------------------------
// GameTable 实现
// ---------------------------------------------------------------------------

type gameTable struct {
	mu   sync.RWMutex
	name string
	rows map[interface{}]interface{}
}

func newGameTable(name string) *gameTable {
	return &gameTable{name: name, rows: make(map[interface{}]interface{})}
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

type configManager struct {
	mu             sync.RWMutex
	configDir      string
	configData     map[string]interface{} // 主配置数据
	tables         map[string]*gameTable
	tableDir       string
	changeHandlers []func(key string)
	watcher        *fsnotify.Watcher
}

type Option func(*configManager)

func WithConfigDir(dir string) Option {
	return func(cm *configManager) { cm.configDir = dir }
}

func WithTableDir(dir string) Option {
	return func(cm *configManager) { cm.tableDir = dir }
}

func NewConfigManager(opts ...Option) ConfigManager {
	cm := &configManager{
		configDir:  "config",
		tableDir:   "config/tables",
		tables:     make(map[string]*gameTable),
		configData: make(map[string]interface{}),
	}
	for _, o := range opts {
		o(cm)
	}
	return cm
}

// Load 读取主配置文件和游戏数据表。
func (cm *configManager) Load() error {
	// 尝试加载主配置文件（支持 config.json）
	configFile := filepath.Join(cm.configDir, "config.json")
	if data, err := os.ReadFile(configFile); err == nil {
		var cfg map[string]interface{}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("config：解析 %q 失败：%w", configFile, err)
		}
		cm.mu.Lock()
		cm.configData = cfg
		cm.mu.Unlock()
	}
	// 加载游戏数据表
	return cm.loadTables()
}

func (cm *configManager) loadTables() error {
	info, err := os.Stat(cm.tableDir)
	if err != nil {
		if os.IsNotExist(err) {
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
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		tableName := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(cm.tableDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("config：无法读取数据表文件 %q：%w", entry.Name(), err)
		}
		tbl := newGameTable(tableName)
		if err := tbl.Reload(data); err != nil {
			return err
		}
		cm.tables[tableName] = tbl
	}
	return nil
}

// Get 通过点分隔的键路径返回配置值。
func (cm *configManager) Get(key string) interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	// 简单的点分隔键查找
	parts := strings.Split(key, ".")
	var current interface{} = cm.configData
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}

func (cm *configManager) GetGameTable(tableName string) (GameTable, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	tbl, ok := cm.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("config：游戏数据表 %q 未找到", tableName)
	}
	return tbl, nil
}

// HotReload 重新加载主配置和所有游戏数据表。
func (cm *configManager) HotReload() error {
	if err := cm.Load(); err != nil {
		return fmt.Errorf("config：热重载失败：%w", err)
	}
	cm.notifyChange("*")
	return nil
}

// Validate 校验配置完整性。
func (cm *configManager) Validate() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	// 检查配置目录是否存在
	if _, err := os.Stat(cm.configDir); err != nil {
		return fmt.Errorf("config：配置目录 %q 不存在：%w", cm.configDir, err)
	}
	return nil
}

// OnChange 注册配置变更回调。
func (cm *configManager) OnChange(handler func(key string)) {
	cm.mu.Lock()
	cm.changeHandlers = append(cm.changeHandlers, handler)
	cm.mu.Unlock()
}

func (cm *configManager) notifyChange(key string) {
	cm.mu.RLock()
	handlers := make([]func(string), len(cm.changeHandlers))
	copy(handlers, cm.changeHandlers)
	cm.mu.RUnlock()
	for _, h := range handlers {
		h(key)
	}
}

// StartWatching 启动文件监听，配置文件变更时自动热重载。
func (cm *configManager) StartWatching() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("config：创建文件监听器失败：%w", err)
	}
	cm.watcher = w

	// 监听配置目录
	if err := w.Add(cm.configDir); err != nil {
		w.Close()
		return fmt.Errorf("config：监听目录 %q 失败：%w", cm.configDir, err)
	}
	// 如果数据表目录存在且不同于配置目录，也监听
	if cm.tableDir != cm.configDir {
		if info, err := os.Stat(cm.tableDir); err == nil && info.IsDir() {
			if err := w.Add(cm.tableDir); err != nil {
				w.Close()
				return fmt.Errorf("config：监听目录 %q 失败：%w", cm.tableDir, err)
			}
		}
	}

	go func() {
		// 防抖：500ms 内多次变更只触发一次重载
		var debounce *time.Timer
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				if !strings.HasSuffix(event.Name, ".json") {
					continue
				}
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					if err := cm.HotReload(); err != nil {
						fmt.Printf("[config] 热重载失败：%v\n", err)
					} else {
						fmt.Printf("[config] 配置已热重载：%s\n", filepath.Base(event.Name))
					}
				})
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				fmt.Printf("[config] 文件监听错误：%v\n", err)
			}
		}
	}()
	return nil
}

// StopWatching 停止文件监听。
func (cm *configManager) StopWatching() error {
	if cm.watcher != nil {
		return cm.watcher.Close()
	}
	return nil
}
