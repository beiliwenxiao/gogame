// Package persistence provides data persistence for the MMRPG game engine.
// It implements async batch writing, auto-save, retry logic, and recovery file fallback.
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"gfgame/internal/engine"
)

// Component mirrors the engine ECS Component interface for persistence use.
type Component interface {
	Type() engine.ComponentType
}

// --- Database Models ---

// DBCharacter is the persistence model for a player character.
type DBCharacter struct {
	ID         uint64    `orm:"id,primary" json:"id"`
	Name       string    `orm:"name"       json:"name"`
	Level      int       `orm:"level"      json:"level"`
	Exp        int64     `orm:"exp"        json:"exp"`
	MapID      string    `orm:"map_id"     json:"map_id"`
	PosX       float32   `orm:"pos_x"      json:"pos_x"`
	PosY       float32   `orm:"pos_y"      json:"pos_y"`
	PosZ       float32   `orm:"pos_z"      json:"pos_z"`
	LayerIndex int       `orm:"layer_index" json:"layer_index"`
	HP         float64   `orm:"hp"         json:"hp"`
	MP         float64   `orm:"mp"         json:"mp"`
	CreatedAt  time.Time `orm:"created_at" json:"created_at"`
	UpdatedAt  time.Time `orm:"updated_at" json:"updated_at"`
}

// DBEquipment is the persistence model for an equipment item.
type DBEquipment struct {
	ID          uint64 `orm:"id,primary"    json:"id"`
	CharacterID uint64 `orm:"character_id"  json:"character_id"`
	ItemID      uint32 `orm:"item_id"       json:"item_id"`
	SlotType    int    `orm:"slot_type"     json:"slot_type"`
	Quality     int    `orm:"quality"       json:"quality"`
	Level       int    `orm:"level"         json:"level"`
	Attributes  string `orm:"attributes"    json:"attributes"` // JSON
}

// DBCombatLog is the persistence model for a combat log entry.
type DBCombatLog struct {
	ID        uint64    `orm:"id,primary" json:"id"`
	CombatID  string    `orm:"combat_id"  json:"combat_id"`
	Tick      uint64    `orm:"tick"       json:"tick"`
	Attacker  uint64    `orm:"attacker"   json:"attacker"`
	Target    uint64    `orm:"target"     json:"target"`
	SkillID   uint32    `orm:"skill_id"   json:"skill_id"`
	Damage    float64   `orm:"damage"     json:"damage"`
	IsCrit    bool      `orm:"is_crit"    json:"is_crit"`
	CreatedAt time.Time `orm:"created_at" json:"created_at"`
}

// --- Configuration ---

// PersistenceConfig holds configuration for the persistence manager.
type PersistenceConfig struct {
	AutoSaveInterval time.Duration // default 5 minutes
	MaxRetries       int           // default 3
	RecoveryFilePath string
	BatchSize        int
}

// DefaultConfig returns a PersistenceConfig with sensible defaults.
func DefaultConfig() PersistenceConfig {
	return PersistenceConfig{
		AutoSaveInterval: 5 * time.Minute,
		MaxRetries:       3,
		RecoveryFilePath: "recovery.json",
		BatchSize:        100,
	}
}

// --- DataStore abstraction ---

// DataStore abstracts the underlying storage so tests can use an in-memory mock.
type DataStore interface {
	// SaveEntities persists a batch of entity data. Returns an error on failure.
	SaveEntities(entities []EntityData) error
	// LoadEntity loads all component data for a single entity.
	LoadEntity(entityID engine.EntityID) (map[engine.ComponentType]Component, error)
}

// EntityData is the payload sent to the DataStore for persistence.
type EntityData struct {
	EntityID   engine.EntityID
	Components map[engine.ComponentType]Component
}

// --- PersistenceManager interface ---

// PersistenceManager is the public interface for the persistence module.
type PersistenceManager interface {
	SaveAsync(entities []engine.EntityID) error
	SaveSync(entities []engine.EntityID) error
	Load(entityID engine.EntityID) (map[engine.ComponentType]Component, error)
	FlushAll() error
	SaveToRecoveryFile(data interface{}) error
	Start() error
	Stop() error
}

// --- saveRequest ---

type saveRequest struct {
	entities []engine.EntityID
	errCh    chan error // nil for async (fire-and-forget), non-nil for sync
}

// --- Implementation ---

type persistenceManager struct {
	config   PersistenceConfig
	store    DataStore
	dirty    map[engine.EntityID]bool // tracks entities with unsaved changes
	dirtyMu  sync.Mutex

	saveCh   chan saveRequest
	stopCh   chan struct{}
	doneCh   chan struct{}

	// entityDataProvider is called to snapshot entity data before saving.
	// If nil, SaveAsync/SaveSync will ask the DataStore to save entity IDs directly.
	entityDataProvider func(id engine.EntityID) (map[engine.ComponentType]Component, bool)
}

// Option configures a persistenceManager.
type Option func(*persistenceManager)

// WithEntityDataProvider sets a callback that provides current entity data for saving.
func WithEntityDataProvider(fn func(engine.EntityID) (map[engine.ComponentType]Component, bool)) Option {
	return func(pm *persistenceManager) {
		pm.entityDataProvider = fn
	}
}

// NewPersistenceManager creates a new PersistenceManager backed by the given DataStore.
func NewPersistenceManager(store DataStore, config PersistenceConfig, opts ...Option) PersistenceManager {
	pm := &persistenceManager{
		config: config,
		store:  store,
		dirty:  make(map[engine.EntityID]bool),
		saveCh: make(chan saveRequest, 256),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	for _, o := range opts {
		o(pm)
	}
	return pm
}

// Start launches the background save worker and auto-save timer.
func (pm *persistenceManager) Start() error {
	go pm.runWorker()
	return nil
}

// Stop signals the worker to drain remaining saves and exit.
func (pm *persistenceManager) Stop() error {
	close(pm.stopCh)
	<-pm.doneCh
	return nil
}

// SaveAsync enqueues entities for asynchronous persistence. Non-blocking.
func (pm *persistenceManager) SaveAsync(entities []engine.EntityID) error {
	pm.markDirty(entities)
	select {
	case pm.saveCh <- saveRequest{entities: entities}:
		return nil
	default:
		return fmt.Errorf("persistence: save queue full")
	}
}

// SaveSync persists entities synchronously, blocking until complete.
func (pm *persistenceManager) SaveSync(entities []engine.EntityID) error {
	pm.markDirty(entities)
	errCh := make(chan error, 1)
	pm.saveCh <- saveRequest{entities: entities, errCh: errCh}
	return <-errCh
}

// Load delegates to the DataStore.
func (pm *persistenceManager) Load(entityID engine.EntityID) (map[engine.ComponentType]Component, error) {
	return pm.store.LoadEntity(entityID)
}

// FlushAll persists all dirty entities synchronously.
func (pm *persistenceManager) FlushAll() error {
	pm.dirtyMu.Lock()
	ids := make([]engine.EntityID, 0, len(pm.dirty))
	for id := range pm.dirty {
		ids = append(ids, id)
	}
	pm.dirtyMu.Unlock()

	if len(ids) == 0 {
		return nil
	}
	return pm.saveWithRetry(ids)
}

// SaveToRecoveryFile writes arbitrary data to the configured recovery file.
func (pm *persistenceManager) SaveToRecoveryFile(data interface{}) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence: marshal recovery data: %w", err)
	}
	if err := os.WriteFile(pm.config.RecoveryFilePath, b, 0644); err != nil {
		return fmt.Errorf("persistence: write recovery file: %w", err)
	}
	return nil
}

// --- internal helpers ---

func (pm *persistenceManager) markDirty(ids []engine.EntityID) {
	pm.dirtyMu.Lock()
	for _, id := range ids {
		pm.dirty[id] = true
	}
	pm.dirtyMu.Unlock()
}

func (pm *persistenceManager) clearDirty(ids []engine.EntityID) {
	pm.dirtyMu.Lock()
	for _, id := range ids {
		delete(pm.dirty, id)
	}
	pm.dirtyMu.Unlock()
}

// runWorker is the background goroutine that processes save requests and auto-save ticks.
func (pm *persistenceManager) runWorker() {
	defer close(pm.doneCh)

	autoSaveTicker := time.NewTicker(pm.config.AutoSaveInterval)
	defer autoSaveTicker.Stop()

	for {
		select {
		case req := <-pm.saveCh:
			err := pm.saveWithRetry(req.entities)
			if req.errCh != nil {
				req.errCh <- err
			}

		case <-autoSaveTicker.C:
			_ = pm.FlushAll()

		case <-pm.stopCh:
			// Drain remaining requests.
			pm.drainQueue()
			// Final flush of all dirty data.
			_ = pm.FlushAll()
			return
		}
	}
}

// drainQueue processes all pending save requests in the channel.
func (pm *persistenceManager) drainQueue() {
	for {
		select {
		case req := <-pm.saveCh:
			err := pm.saveWithRetry(req.entities)
			if req.errCh != nil {
				req.errCh <- err
			}
		default:
			return
		}
	}
}

// saveWithRetry attempts to save entities with up to MaxRetries attempts.
// On final failure, it saves to the recovery file.
func (pm *persistenceManager) saveWithRetry(ids []engine.EntityID) error {
	data := pm.collectEntityData(ids)
	if len(data) == 0 {
		return nil
	}

	// Batch the data according to BatchSize.
	batches := pm.splitBatches(data)

	var lastErr error
	for _, batch := range batches {
		var err error
		for attempt := 0; attempt < pm.config.MaxRetries; attempt++ {
			err = pm.store.SaveEntities(batch)
			if err == nil {
				break
			}
		}
		if err != nil {
			lastErr = err
			// Save failed batch to recovery file.
			_ = pm.SaveToRecoveryFile(batch)
		}
	}

	if lastErr == nil {
		pm.clearDirty(ids)
	}
	return lastErr
}

func (pm *persistenceManager) collectEntityData(ids []engine.EntityID) []EntityData {
	if pm.entityDataProvider == nil {
		// Without a provider, create minimal EntityData entries.
		result := make([]EntityData, 0, len(ids))
		for _, id := range ids {
			result = append(result, EntityData{EntityID: id})
		}
		return result
	}
	result := make([]EntityData, 0, len(ids))
	for _, id := range ids {
		comps, ok := pm.entityDataProvider(id)
		if !ok {
			continue
		}
		result = append(result, EntityData{EntityID: id, Components: comps})
	}
	return result
}

func (pm *persistenceManager) splitBatches(data []EntityData) [][]EntityData {
	batchSize := pm.config.BatchSize
	if batchSize <= 0 {
		batchSize = len(data)
	}
	var batches [][]EntityData
	for i := 0; i < len(data); i += batchSize {
		end := i + batchSize
		if end > len(data) {
			end = len(data)
		}
		batches = append(batches, data[i:end])
	}
	return batches
}
