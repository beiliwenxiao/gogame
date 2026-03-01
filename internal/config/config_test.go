package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// helper: create a temp directory with config and tables sub-dirs.
// Returns a cleanup function that should be deferred.
func setupTestDirs(t *testing.T) (configDir, tableDir string, cleanup func()) {
	t.Helper()
	configDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatal(err)
	}
	tableDir = filepath.Join(configDir, "tables")
	if err := os.MkdirAll(tableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cleanup = func() { os.RemoveAll(configDir) }
	return configDir, tableDir, cleanup
}

// helper: write a JSON file into the given directory.
func writeJSON(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGameTable_ReloadAndGetRow(t *testing.T) {
	tbl := newGameTable("skills")
	data := `[{"id":1,"name":"Fireball","damage":100},{"id":2,"name":"Heal","damage":0}]`
	if err := tbl.Reload([]byte(data)); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// GetRow with float64 key (JSON numbers decode as float64).
	row, ok := tbl.GetRow(float64(1))
	if !ok {
		t.Fatal("expected row with id=1")
	}
	m := row.(map[string]interface{})
	if m["name"] != "Fireball" {
		t.Errorf("expected Fireball, got %v", m["name"])
	}

	// GetAll should return 2 rows.
	all := tbl.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 rows, got %d", len(all))
	}
}

func TestGameTable_ReloadInvalidJSON(t *testing.T) {
	tbl := newGameTable("bad")
	err := tbl.Reload([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGameTable_ReloadMissingID(t *testing.T) {
	tbl := newGameTable("noid")
	err := tbl.Reload([]byte(`[{"name":"NoID"}]`))
	if err == nil {
		t.Fatal("expected error for missing id field")
	}
}

func TestConfigManager_LoadAndGetGameTable(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	writeJSON(t, tableDir, "monsters.json",
		`[{"id":1,"name":"Goblin","hp":100},{"id":2,"name":"Dragon","hp":9999}]`)

	cm := NewConfigManager(
		WithConfigDir(configDir),
		WithTableDir(tableDir),
	)
	if err := cm.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	tbl, err := cm.GetGameTable("monsters")
	if err != nil {
		t.Fatalf("GetGameTable failed: %v", err)
	}

	row, ok := tbl.GetRow(float64(2))
	if !ok {
		t.Fatal("expected Dragon row")
	}
	m := row.(map[string]interface{})
	if m["name"] != "Dragon" {
		t.Errorf("expected Dragon, got %v", m["name"])
	}
}

func TestConfigManager_GetGameTable_NotFound(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	cm := NewConfigManager(WithConfigDir(configDir), WithTableDir(tableDir))
	if err := cm.Load(); err != nil {
		t.Fatal(err)
	}
	_, err := cm.GetGameTable("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing table")
	}
}

func TestConfigManager_Validate_Valid(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	writeJSON(t, tableDir, "skills.json",
		`[{"id":1,"name":"Slash"}]`)

	cm := NewConfigManager(WithConfigDir(configDir), WithTableDir(tableDir))
	if err := cm.Validate(); err != nil {
		t.Fatalf("Validate should pass: %v", err)
	}
}

func TestConfigManager_Validate_InvalidJSON(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	writeJSON(t, tableDir, "bad.json", `{broken`)

	cm := NewConfigManager(WithConfigDir(configDir), WithTableDir(tableDir))
	if err := cm.Validate(); err == nil {
		t.Fatal("expected validation error for bad JSON")
	}
}

func TestConfigManager_Validate_MissingID(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	writeJSON(t, tableDir, "noid.json", `[{"name":"NoID"}]`)

	cm := NewConfigManager(WithConfigDir(configDir), WithTableDir(tableDir))
	if err := cm.Validate(); err == nil {
		t.Fatal("expected validation error for missing id")
	}
}

func TestConfigManager_HotReload(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	writeJSON(t, tableDir, "items.json",
		`[{"id":1,"name":"Sword"}]`)

	cm := NewConfigManager(WithConfigDir(configDir), WithTableDir(tableDir))
	if err := cm.Load(); err != nil {
		t.Fatal(err)
	}

	// Verify initial data.
	tbl, _ := cm.GetGameTable("items")
	row, _ := tbl.GetRow(float64(1))
	if row.(map[string]interface{})["name"] != "Sword" {
		t.Fatal("expected Sword")
	}

	// Update the file and hot reload.
	writeJSON(t, tableDir, "items.json",
		`[{"id":1,"name":"Excalibur"},{"id":2,"name":"Shield"}]`)

	if err := cm.HotReload(); err != nil {
		t.Fatalf("HotReload failed: %v", err)
	}

	// Verify updated data.
	tbl, _ = cm.GetGameTable("items")
	row, _ = tbl.GetRow(float64(1))
	if row.(map[string]interface{})["name"] != "Excalibur" {
		t.Error("expected Excalibur after hot reload")
	}
	row, ok := tbl.GetRow(float64(2))
	if !ok {
		t.Error("expected Shield row after hot reload")
	}
}

func TestConfigManager_OnChange(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	writeJSON(t, tableDir, "buffs.json",
		`[{"id":1,"name":"Haste"}]`)

	cm := NewConfigManager(WithConfigDir(configDir), WithTableDir(tableDir))
	if err := cm.Load(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var changedKeys []string
	cm.OnChange(func(key string) {
		mu.Lock()
		changedKeys = append(changedKeys, key)
		mu.Unlock()
	})

	// Hot reload triggers change notification.
	writeJSON(t, tableDir, "buffs.json",
		`[{"id":1,"name":"Haste"},{"id":2,"name":"Shield"}]`)
	if err := cm.HotReload(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(changedKeys) == 0 {
		t.Error("expected at least one change notification")
	}
	found := false
	for _, k := range changedKeys {
		if k == "table.buffs" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected change key 'table.buffs', got %v", changedKeys)
	}
}

func TestConfigManager_MultipleOnChangeHandlers(t *testing.T) {
	configDir, tableDir, cleanup := setupTestDirs(t)
	defer cleanup()
	writeJSON(t, tableDir, "test.json", `[{"id":1,"name":"A"}]`)

	cm := NewConfigManager(WithConfigDir(configDir), WithTableDir(tableDir))
	if err := cm.Load(); err != nil {
		t.Fatal(err)
	}

	var count1, count2 int
	cm.OnChange(func(key string) { count1++ })
	cm.OnChange(func(key string) { count2++ })

	if err := cm.HotReload(); err != nil {
		t.Fatal(err)
	}

	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both handlers called once, got %d and %d", count1, count2)
	}
}

func TestConfigManager_LoadNoTableDir(t *testing.T) {
	configDir, err := os.MkdirTemp("", "config_test_notabledir_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(configDir)
	// tableDir doesn't exist — should be fine.
	cm := NewConfigManager(
		WithConfigDir(configDir),
		WithTableDir(filepath.Join(configDir, "nonexistent")),
	)
	if err := cm.Load(); err != nil {
		t.Fatalf("Load should succeed without table dir: %v", err)
	}
}

func TestConfigManager_Get_NilWhenNoAdapter(t *testing.T) {
	cm := NewConfigManager()
	// Get before Load should return nil.
	if v := cm.Get("anything"); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}
