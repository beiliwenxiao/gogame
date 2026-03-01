package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gfgame/internal/engine"
)

// --- In-memory DataStore mock ---

type memoryStore struct {
	mu       sync.Mutex
	data     map[engine.EntityID]map[engine.ComponentType]Component
	saveCalls int32
	failUntil int32 // fail the first N SaveEntities calls
	callCount int32
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		data: make(map[engine.EntityID]map[engine.ComponentType]Component),
	}
}

func (m *memoryStore) SaveEntities(entities []EntityData) error {
	n := atomic.AddInt32(&m.callCount, 1)
	if n <= atomic.LoadInt32(&m.failUntil) {
		return fmt.Errorf("simulated DB failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range entities {
		m.data[e.EntityID] = e.Components
	}
	atomic.AddInt32(&m.saveCalls, 1)
	return nil
}

func (m *memoryStore) LoadEntity(entityID engine.EntityID) (map[engine.ComponentType]Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	comps, ok := m.data[entityID]
	if !ok {
		return nil, fmt.Errorf("entity %d not found", entityID)
	}
	return comps, nil
}

// --- Test component ---

type testComponent struct {
	compType engine.ComponentType
	Value    string
}

func (c *testComponent) Type() engine.ComponentType { return c.compType }

// --- Helper ---

func testConfig(t *testing.T) PersistenceConfig {
	t.Helper()
	recoveryPath := filepath.Join(os.TempDir(), fmt.Sprintf("recovery_test_%d.json", time.Now().UnixNano()))
	t.Cleanup(func() { os.Remove(recoveryPath) })
	return PersistenceConfig{
		AutoSaveInterval: 50 * time.Millisecond, // fast for tests
		MaxRetries:       3,
		RecoveryFilePath: recoveryPath,
		BatchSize:        10,
	}
}

func startPM(t *testing.T, store DataStore, cfg PersistenceConfig, opts ...Option) PersistenceManager {
	t.Helper()
	pm := NewPersistenceManager(store, cfg, opts...)
	if err := pm.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = pm.Stop() })
	return pm
}

// --- Tests ---

func TestSaveSyncAndLoad(t *testing.T) {
	store := newMemoryStore()
	cfg := testConfig(t)

	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{
			1: &testComponent{compType: 1, Value: fmt.Sprintf("entity-%d", id)},
		}, true
	}

	pm := startPM(t, store, cfg, WithEntityDataProvider(provider))

	ids := []engine.EntityID{100, 200}
	if err := pm.SaveSync(ids); err != nil {
		t.Fatalf("SaveSync: %v", err)
	}

	// Verify data was persisted.
	comps, err := pm.Load(100)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tc, ok := comps[1].(*testComponent)
	if !ok || tc.Value != "entity-100" {
		t.Fatalf("unexpected component: %+v", comps[1])
	}
}

func TestSaveAsyncEventuallyPersists(t *testing.T) {
	store := newMemoryStore()
	cfg := testConfig(t)

	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{
			1: &testComponent{compType: 1, Value: "async"},
		}, true
	}

	pm := startPM(t, store, cfg, WithEntityDataProvider(provider))

	if err := pm.SaveAsync([]engine.EntityID{42}); err != nil {
		t.Fatalf("SaveAsync: %v", err)
	}

	// Wait for async processing.
	time.Sleep(100 * time.Millisecond)

	comps, err := pm.Load(42)
	if err != nil {
		t.Fatalf("Load after async: %v", err)
	}
	if comps[1].(*testComponent).Value != "async" {
		t.Fatal("unexpected value")
	}
}

func TestFlushAllPersistsDirtyEntities(t *testing.T) {
	store := newMemoryStore()
	cfg := testConfig(t)
	cfg.AutoSaveInterval = 1 * time.Hour // disable auto-save for this test

	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{
			2: &testComponent{compType: 2, Value: "flush"},
		}, true
	}

	pm := NewPersistenceManager(store, cfg, WithEntityDataProvider(provider))
	// Mark entities dirty without starting the worker.
	pmImpl := pm.(*persistenceManager)
	pmImpl.markDirty([]engine.EntityID{10, 20, 30})

	if err := pm.FlushAll(); err != nil {
		t.Fatalf("FlushAll: %v", err)
	}

	for _, id := range []engine.EntityID{10, 20, 30} {
		if _, err := store.LoadEntity(id); err != nil {
			t.Fatalf("entity %d not found after FlushAll: %v", id, err)
		}
	}
}

func TestRetryOnFailureThenSucceed(t *testing.T) {
	store := newMemoryStore()
	// Fail the first 2 calls, succeed on the 3rd.
	atomic.StoreInt32(&store.failUntil, 2)

	cfg := testConfig(t)
	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{
			1: &testComponent{compType: 1, Value: "retry-ok"},
		}, true
	}

	pm := startPM(t, store, cfg, WithEntityDataProvider(provider))

	if err := pm.SaveSync([]engine.EntityID{99}); err != nil {
		t.Fatalf("SaveSync should succeed after retries: %v", err)
	}

	comps, err := pm.Load(99)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if comps[1].(*testComponent).Value != "retry-ok" {
		t.Fatal("unexpected value after retry")
	}
}

func TestRetryExhaustedSavesToRecoveryFile(t *testing.T) {
	store := newMemoryStore()
	// Fail all 3 retries.
	atomic.StoreInt32(&store.failUntil, 100)

	cfg := testConfig(t)
	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{}, true
	}

	pm := startPM(t, store, cfg, WithEntityDataProvider(provider))

	err := pm.SaveSync([]engine.EntityID{77})
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}

	// Verify recovery file was created.
	if _, statErr := os.Stat(cfg.RecoveryFilePath); os.IsNotExist(statErr) {
		t.Fatal("recovery file should exist after all retries fail")
	}
}

func TestSaveToRecoveryFile(t *testing.T) {
	cfg := testConfig(t)
	store := newMemoryStore()
	pm := NewPersistenceManager(store, cfg)

	data := map[string]string{"key": "value"}
	if err := pm.SaveToRecoveryFile(data); err != nil {
		t.Fatalf("SaveToRecoveryFile: %v", err)
	}

	b, err := os.ReadFile(cfg.RecoveryFilePath)
	if err != nil {
		t.Fatalf("read recovery file: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["key"] != "value" {
		t.Fatalf("unexpected recovery data: %v", result)
	}
}

func TestGracefulShutdownFlushes(t *testing.T) {
	store := newMemoryStore()
	cfg := testConfig(t)
	cfg.AutoSaveInterval = 1 * time.Hour

	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{
			1: &testComponent{compType: 1, Value: "shutdown"},
		}, true
	}

	pm := NewPersistenceManager(store, cfg, WithEntityDataProvider(provider))
	_ = pm.Start()

	// Enqueue async saves.
	_ = pm.SaveAsync([]engine.EntityID{1, 2, 3})
	time.Sleep(20 * time.Millisecond) // let worker pick them up

	// Mark more dirty.
	pmImpl := pm.(*persistenceManager)
	pmImpl.markDirty([]engine.EntityID{4, 5})

	// Stop should flush everything.
	if err := pm.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// All entities should be persisted.
	for _, id := range []engine.EntityID{1, 2, 3, 4, 5} {
		if _, err := store.LoadEntity(id); err != nil {
			t.Errorf("entity %d not found after shutdown: %v", id, err)
		}
	}
}

func TestAutoSaveTriggered(t *testing.T) {
	store := newMemoryStore()
	cfg := testConfig(t)
	cfg.AutoSaveInterval = 30 * time.Millisecond

	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{
			1: &testComponent{compType: 1, Value: "auto"},
		}, true
	}

	pm := NewPersistenceManager(store, cfg, WithEntityDataProvider(provider))
	_ = pm.Start()

	// Mark dirty without sending through saveCh.
	pmImpl := pm.(*persistenceManager)
	pmImpl.markDirty([]engine.EntityID{500})

	// Wait for auto-save to trigger.
	time.Sleep(150 * time.Millisecond)

	_ = pm.Stop()

	if _, err := store.LoadEntity(500); err != nil {
		t.Fatalf("entity 500 not auto-saved: %v", err)
	}
}

func TestBatchSplitting(t *testing.T) {
	store := newMemoryStore()
	cfg := testConfig(t)
	cfg.BatchSize = 3

	provider := func(id engine.EntityID) (map[engine.ComponentType]Component, bool) {
		return map[engine.ComponentType]Component{
			1: &testComponent{compType: 1, Value: "batch"},
		}, true
	}

	pm := startPM(t, store, cfg, WithEntityDataProvider(provider))

	ids := []engine.EntityID{1, 2, 3, 4, 5, 6, 7}
	if err := pm.SaveSync(ids); err != nil {
		t.Fatalf("SaveSync: %v", err)
	}

	// All 7 entities should be saved (across 3 batches: 3+3+1).
	for _, id := range ids {
		if _, err := store.LoadEntity(id); err != nil {
			t.Errorf("entity %d not found: %v", id, err)
		}
	}

	// Verify multiple save calls were made (3 batches).
	calls := atomic.LoadInt32(&store.saveCalls)
	if calls != 3 {
		t.Errorf("expected 3 batch save calls, got %d", calls)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AutoSaveInterval != 5*time.Minute {
		t.Errorf("expected 5m auto-save interval, got %v", cfg.AutoSaveInterval)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected 3 max retries, got %d", cfg.MaxRetries)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("expected batch size 100, got %d", cfg.BatchSize)
	}
}

func TestLoadNonExistentEntity(t *testing.T) {
	store := newMemoryStore()
	cfg := testConfig(t)
	pm := NewPersistenceManager(store, cfg)

	_, err := pm.Load(999)
	if err == nil {
		t.Fatal("expected error loading non-existent entity")
	}
}

func TestDBModelsExist(t *testing.T) {
	// Verify DB model structs can be instantiated with expected fields.
	now := time.Now()

	char := DBCharacter{
		ID: 1, Name: "hero", Level: 10, Exp: 5000,
		MapID: "map1", PosX: 1.0, PosY: 2.0, PosZ: 3.0,
		LayerIndex: 0, HP: 100, MP: 50,
		CreatedAt: now, UpdatedAt: now,
	}
	if char.Name != "hero" || char.Level != 10 {
		t.Fatal("DBCharacter fields incorrect")
	}

	equip := DBEquipment{
		ID: 1, CharacterID: 1, ItemID: 100,
		SlotType: 0, Quality: 2, Level: 5,
		Attributes: `{"attack":10}`,
	}
	if equip.ItemID != 100 {
		t.Fatal("DBEquipment fields incorrect")
	}

	log := DBCombatLog{
		ID: 1, CombatID: "combat-1", Tick: 100,
		Attacker: 1, Target: 2, SkillID: 10,
		Damage: 50.5, IsCrit: true, CreatedAt: now,
	}
	if log.Damage != 50.5 || !log.IsCrit {
		t.Fatal("DBCombatLog fields incorrect")
	}
}
