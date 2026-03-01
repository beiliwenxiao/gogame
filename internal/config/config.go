// Package config provides configuration management for the MMRPG game engine.
// It supports YAML/JSON/TOML config loading via GoFrame gcfg, game table management,
// hot reload with file watching, and startup validation.
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

// ConfigManager manages engine configuration loading, validation, hot reload,
// and game table access.
type ConfigManager interface {
	// Load reads all configuration files from the configured directory.
	Load() error
	// Get returns a configuration value by dot-separated key path.
	Get(key string) interface{}
	// GetGameTable returns a named game table (e.g. "skills", "monsters", "equipment").
	GetGameTable(tableName string) (GameTable, error)
	// HotReload re-reads changed configuration files without stopping the engine.
	HotReload() error
	// Validate checks all loaded configuration for format and data integrity.
	Validate() error
	// OnChange registers a callback invoked when a config key changes during hot reload.
	OnChange(handler func(key string))
}

// GameTable provides read access to a game data table (skill table, monster table, etc.).
type GameTable interface {
	// GetRow returns a single row by its ID key.
	GetRow(id interface{}) (interface{}, bool)
	// GetAll returns all rows in the table.
	GetAll() []interface{}
	// Reload replaces the table data from raw JSON bytes.
	Reload(data []byte) error
}

// ---------------------------------------------------------------------------
// GameTable implementation
// ---------------------------------------------------------------------------

// gameTable stores rows keyed by an arbitrary ID.
type gameTable struct {
	mu   sync.RWMutex
	name string
	rows map[interface{}]interface{}
}

// newGameTable creates an empty game table.
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

// Reload parses JSON data as an array of objects. Each object must contain an
// "id" field which is used as the row key.
func (t *gameTable) Reload(data []byte) error {
	var rows []map[string]interface{}
	if err := json.Unmarshal(data, &rows); err != nil {
		return fmt.Errorf("game table %q: invalid JSON: %w", t.name, err)
	}

	newRows := make(map[interface{}]interface{}, len(rows))
	for i, row := range rows {
		id, ok := row["id"]
		if !ok {
			return fmt.Errorf("game table %q: row %d missing 'id' field", t.name, i)
		}
		newRows[id] = row
	}

	t.mu.Lock()
	t.rows = newRows
	t.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// ConfigManager implementation
// ---------------------------------------------------------------------------

// configManager is the default ConfigManager implementation.
type configManager struct {
	mu sync.RWMutex

	// configDir is the root directory for config files.
	configDir string

	// gcfg adapter for YAML/JSON/TOML loading.
	adapter *gcfg.Config

	// tables holds loaded game tables keyed by table name.
	tables map[string]*gameTable

	// tableDir is the directory containing game table JSON files.
	tableDir string

	// changeHandlers are callbacks notified on config changes.
	changeHandlers []func(key string)

	// watchCallbacks tracks gfsnotify callbacks for cleanup.
	watchCallbacks []*gfsnotify.Callback
}

// Option configures a configManager.
type Option func(*configManager)

// WithConfigDir sets the root configuration directory (default: "config").
func WithConfigDir(dir string) Option {
	return func(cm *configManager) { cm.configDir = dir }
}

// WithTableDir sets the game table directory (default: "config/tables").
func WithTableDir(dir string) Option {
	return func(cm *configManager) { cm.tableDir = dir }
}

// NewConfigManager creates a new ConfigManager with the given options.
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

// Load reads the main config via gcfg and loads all game tables from tableDir.
func (cm *configManager) Load() error {
	// Initialize gcfg adapter pointing at configDir.
	adapterFile, err := gcfg.NewAdapterFile()
	if err != nil {
		return fmt.Errorf("config: failed to create adapter: %w", err)
	}
	if err := adapterFile.SetPath(cm.configDir); err != nil {
		return fmt.Errorf("config: failed to set config path %q: %w", cm.configDir, err)
	}
	cm.adapter = gcfg.NewWithAdapter(adapterFile)

	// Load game tables from tableDir.
	if err := cm.loadTables(); err != nil {
		return err
	}

	return nil
}

// loadTables scans tableDir for JSON files and loads each as a GameTable.
func (cm *configManager) loadTables() error {
	info, err := os.Stat(cm.tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No tables directory — that's fine.
			return nil
		}
		return fmt.Errorf("config: cannot stat table dir %q: %w", cm.tableDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config: table path %q is not a directory", cm.tableDir)
	}

	entries, err := os.ReadDir(cm.tableDir)
	if err != nil {
		return fmt.Errorf("config: cannot read table dir %q: %w", cm.tableDir, err)
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
			return fmt.Errorf("config: cannot read table file %q: %w", name, err)
		}
		tbl := newGameTable(tableName)
		if err := tbl.Reload(data); err != nil {
			return err
		}
		cm.tables[tableName] = tbl
	}
	return nil
}

// Get returns a config value by key using the gcfg adapter.
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

// GetGameTable returns a loaded game table by name.
func (cm *configManager) GetGameTable(tableName string) (GameTable, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	tbl, ok := cm.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("config: game table %q not found", tableName)
	}
	return tbl, nil
}

// HotReload re-reads all game table files and notifies change handlers.
func (cm *configManager) HotReload() error {
	info, err := os.Stat(cm.tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("config: cannot stat table dir %q: %w", cm.tableDir, err)
	}
	if !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(cm.tableDir)
	if err != nil {
		return fmt.Errorf("config: cannot read table dir %q: %w", cm.tableDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		tableName := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(cm.tableDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("config: cannot read table file %q: %w", entry.Name(), err)
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

// Validate checks that the config directory exists and all table files are valid JSON.
func (cm *configManager) Validate() error {
	// Validate config directory.
	if _, err := os.Stat(cm.configDir); err != nil {
		return fmt.Errorf("config: config directory %q: %w", cm.configDir, err)
	}

	// Validate table files.
	info, err := os.Stat(cm.tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no tables dir is acceptable
		}
		return fmt.Errorf("config: table directory %q: %w", cm.tableDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config: table path %q is not a directory", cm.tableDir)
	}

	entries, err := os.ReadDir(cm.tableDir)
	if err != nil {
		return fmt.Errorf("config: cannot read table dir %q: %w", cm.tableDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(cm.tableDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("config: cannot read %q: %w", filePath, err)
		}
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return fmt.Errorf("config: invalid JSON in %q: %w", filePath, err)
		}
		for i, row := range rows {
			if _, ok := row["id"]; !ok {
				return fmt.Errorf("config: %q row %d missing required 'id' field", filePath, i)
			}
		}
	}
	return nil
}

// OnChange registers a callback that fires when a config key changes.
func (cm *configManager) OnChange(handler func(key string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.changeHandlers = append(cm.changeHandlers, handler)
}

// notifyChange invokes all registered change handlers.
func (cm *configManager) notifyChange(key string) {
	cm.mu.RLock()
	handlers := make([]func(key string), len(cm.changeHandlers))
	copy(handlers, cm.changeHandlers)
	cm.mu.RUnlock()

	for _, h := range handlers {
		h(key)
	}
}

// StartWatching begins file-system watching on the tableDir for hot reload.
// It uses GoFrame's gfsnotify to detect file changes and triggers HotReload
// with a small debounce window.
func (cm *configManager) StartWatching() error {
	if _, err := os.Stat(cm.tableDir); os.IsNotExist(err) {
		return nil // nothing to watch
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
		return fmt.Errorf("config: failed to watch %q: %w", cm.tableDir, err)
	}

	cm.mu.Lock()
	cm.watchCallbacks = append(cm.watchCallbacks, cb)
	cm.mu.Unlock()
	return nil
}

// StopWatching removes all file watchers.
func (cm *configManager) StopWatching() {
	cm.mu.Lock()
	callbacks := cm.watchCallbacks
	cm.watchCallbacks = nil
	cm.mu.Unlock()

	for _, cb := range callbacks {
		_ = gfsnotify.RemoveCallback(cb.Id)
	}
}
