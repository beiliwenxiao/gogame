// Package persistence 为 MMRPG 游戏引擎提供数据持久化功能。
// 实现了异步批量写入、自动保存、重试逻辑和恢复文件回退机制。
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"gfgame/internal/engine"
)

// Component 镜像引擎 ECS 的 Component 接口，供持久化模块使用。
type Component interface {
	Type() engine.ComponentType
}

// --- 数据库模型 ---

// DBCharacter 是玩家角色的持久化模型。
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

// DBEquipment 是装备物品的持久化模型。
type DBEquipment struct {
	ID          uint64 `orm:"id,primary"    json:"id"`
	CharacterID uint64 `orm:"character_id"  json:"character_id"`
	ItemID      uint32 `orm:"item_id"       json:"item_id"`
	SlotType    int    `orm:"slot_type"     json:"slot_type"`
	Quality     int    `orm:"quality"       json:"quality"`
	Level       int    `orm:"level"         json:"level"`
	Attributes  string `orm:"attributes"    json:"attributes"` // JSON 格式
}

// DBCombatLog 是战斗日志条目的持久化模型。
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

// --- 配置 ---

// PersistenceConfig 保存持久化管理器的配置。
type PersistenceConfig struct {
	AutoSaveInterval time.Duration // 默认 5 分钟
	MaxRetries       int           // 默认 3 次
	RecoveryFilePath string
	BatchSize        int
}

// DefaultConfig 返回带有合理默认值的 PersistenceConfig。
func DefaultConfig() PersistenceConfig {
	return PersistenceConfig{
		AutoSaveInterval: 5 * time.Minute,
		MaxRetries:       3,
		RecoveryFilePath: "recovery.json",
		BatchSize:        100,
	}
}

// --- DataStore 抽象 ---

// DataStore 抽象底层存储，使测试可以使用内存模拟实现。
type DataStore interface {
	// SaveEntities 批量持久化实体数据，失败时返回错误。
	SaveEntities(entities []EntityData) error
	// LoadEntity 加载单个实体的所有组件数据。
	LoadEntity(entityID engine.EntityID) (map[engine.ComponentType]Component, error)
}

// EntityData 是发送给 DataStore 进行持久化的载荷。
type EntityData struct {
	EntityID   engine.EntityID
	Components map[engine.ComponentType]Component
}

// --- PersistenceManager 接口 ---

// PersistenceManager 是持久化模块的公共接口。
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
	errCh    chan error // 异步时为 nil（即发即忘），同步时非 nil
}

// --- 实现 ---

type persistenceManager struct {
	config   PersistenceConfig
	store    DataStore
	dirty    map[engine.EntityID]bool // 追踪有未保存变更的实体
	dirtyMu  sync.Mutex

	saveCh   chan saveRequest
	stopCh   chan struct{}
	doneCh   chan struct{}

	// entityDataProvider 在保存前用于快照实体数据。
	// 为 nil 时，SaveAsync/SaveSync 直接将实体 ID 传给 DataStore。
	entityDataProvider func(id engine.EntityID) (map[engine.ComponentType]Component, bool)
}

// Option 用于配置 persistenceManager。
type Option func(*persistenceManager)

// WithEntityDataProvider 设置提供当前实体数据的回调函数。
func WithEntityDataProvider(fn func(engine.EntityID) (map[engine.ComponentType]Component, bool)) Option {
	return func(pm *persistenceManager) {
		pm.entityDataProvider = fn
	}
}

// NewPersistenceManager 创建由给定 DataStore 支撑的新 PersistenceManager。
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

// Start 启动后台保存 Worker 和自动保存定时器。
func (pm *persistenceManager) Start() error {
	go pm.runWorker()
	return nil
}

// Stop 通知 Worker 排空剩余保存任务后退出。
func (pm *persistenceManager) Stop() error {
	close(pm.stopCh)
	<-pm.doneCh
	return nil
}

// SaveAsync 将实体加入异步持久化队列，非阻塞。
func (pm *persistenceManager) SaveAsync(entities []engine.EntityID) error {
	pm.markDirty(entities)
	select {
	case pm.saveCh <- saveRequest{entities: entities}:
		return nil
	default:
		return fmt.Errorf("persistence：保存队列已满")
	}
}

// SaveSync 同步持久化实体，阻塞直到完成。
func (pm *persistenceManager) SaveSync(entities []engine.EntityID) error {
	pm.markDirty(entities)
	errCh := make(chan error, 1)
	pm.saveCh <- saveRequest{entities: entities, errCh: errCh}
	return <-errCh
}

// Load 委托给 DataStore 加载实体数据。
func (pm *persistenceManager) Load(entityID engine.EntityID) (map[engine.ComponentType]Component, error) {
	return pm.store.LoadEntity(entityID)
}

// FlushAll 同步持久化所有脏实体。
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

// SaveToRecoveryFile 将任意数据写入配置的恢复文件。
func (pm *persistenceManager) SaveToRecoveryFile(data interface{}) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence：序列化恢复数据失败：%w", err)
	}
	if err := os.WriteFile(pm.config.RecoveryFilePath, b, 0644); err != nil {
		return fmt.Errorf("persistence：写入恢复文件失败：%w", err)
	}
	return nil
}

// --- 内部辅助函数 ---

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

// runWorker 是处理保存请求和自动保存定时器的后台 goroutine。
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
			// 排空剩余请求。
			pm.drainQueue()
			// 最终刷新所有脏数据。
			_ = pm.FlushAll()
			return
		}
	}
}

// drainQueue 处理 channel 中所有待处理的保存请求。
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

// saveWithRetry 最多重试 MaxRetries 次保存实体。
// 最终失败时将数据写入恢复文件。
func (pm *persistenceManager) saveWithRetry(ids []engine.EntityID) error {
	data := pm.collectEntityData(ids)
	if len(data) == 0 {
		return nil
	}

	// 按 BatchSize 分批处理。
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
			// 将失败批次写入恢复文件。
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
		// 没有提供者时，创建最小 EntityData 条目。
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
