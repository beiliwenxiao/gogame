// Package ecs 实现了 MMRPG 游戏引擎的实体-组件-系统（ECS）架构。
package ecs

import (
	"sort"
	"sync/atomic"
	"time"

	"gogame/internal/engine"
)

// 核心组件类型常量。
const (
	CompPosition        engine.ComponentType = 1
	CompMovement        engine.ComponentType = 2
	CompCombatAttribute engine.ComponentType = 3
	CompEquipment       engine.ComponentType = 4
	CompSkill           engine.ComponentType = 5
	CompBuff            engine.ComponentType = 6
	CompNetwork         engine.ComponentType = 7
	CompAOI             engine.ComponentType = 8
)

// Component 是所有 ECS 组件必须实现的接口。
type Component interface {
	Type() engine.ComponentType
}

// System 是 ECS 系统的接口，用于处理具有特定组件组合的实体。
type System interface {
	Name() string
	Priority() int
	RequiredComponents() []engine.ComponentType
	Update(tick uint64, entities []engine.EntityID, world *World)
}

// EntityManager 定义了管理实体、组件和系统的接口。
type EntityManager interface {
	CreateEntity() engine.EntityID
	DestroyEntity(id engine.EntityID)
	AddComponent(id engine.EntityID, comp Component)
	RemoveComponent(id engine.EntityID, compType engine.ComponentType)
	GetComponent(id engine.EntityID, compType engine.ComponentType) (Component, bool)
	Query(compTypes ...engine.ComponentType) []engine.EntityID
	RegisterSystem(sys System)
	Update(tick uint64)
}

// World 是存储实体、组件和系统的 ECS 世界。
// 每个 Scene 维护自己的 World 实例。
type World struct {
	entities map[engine.EntityID]map[engine.ComponentType]Component
	systems  []System
	nextID   atomic.Uint64
}

// NewWorld 创建一个新的 ECS World 实例，实现 EntityManager 接口。
func NewWorld() *World {
	return &World{
		entities: make(map[engine.EntityID]map[engine.ComponentType]Component),
	}
}

// CreateEntity 分配一个全局唯一的 EntityID 并注册该实体。
func (w *World) CreateEntity() engine.EntityID {
	id := engine.EntityID(w.nextID.Add(1))
	w.entities[id] = make(map[engine.ComponentType]Component)
	return id
}

// DestroyEntity 移除实体的所有组件并将其从世界中删除。
func (w *World) DestroyEntity(id engine.EntityID) {
	delete(w.entities, id)
}

// InjectEntity 以指定 ID 在世界中注册一个实体。
// 用于跨场景实体迁移时保留原有实体 ID。
// 若实体已存在，则此操作为空操作。
func (w *World) InjectEntity(id engine.EntityID) {
	if _, exists := w.entities[id]; exists {
		return
	}
	w.entities[id] = make(map[engine.ComponentType]Component)
}

// AddComponent 将组件附加到指定实体。
// 若实体不存在，则此操作为空操作。
func (w *World) AddComponent(id engine.EntityID, comp Component) {
	comps, ok := w.entities[id]
	if !ok {
		return
	}
	comps[comp.Type()] = comp
}

// RemoveComponent 从实体上移除指定类型的组件。
func (w *World) RemoveComponent(id engine.EntityID, compType engine.ComponentType) {
	comps, ok := w.entities[id]
	if !ok {
		return
	}
	delete(comps, compType)
}

// GetComponent 从实体上获取指定类型的组件。
func (w *World) GetComponent(id engine.EntityID, compType engine.ComponentType) (Component, bool) {
	comps, ok := w.entities[id]
	if !ok {
		return nil, false
	}
	comp, found := comps[compType]
	return comp, found
}

// Query 返回拥有所有指定组件类型的实体 ID 列表。
func (w *World) Query(compTypes ...engine.ComponentType) []engine.EntityID {
	var result []engine.EntityID
	for id, comps := range w.entities {
		if hasAll(comps, compTypes) {
			result = append(result, id)
		}
	}
	return result
}

func hasAll(comps map[engine.ComponentType]Component, required []engine.ComponentType) bool {
	for _, ct := range required {
		if _, ok := comps[ct]; !ok {
			return false
		}
	}
	return true
}

// RegisterSystem 向世界添加一个系统。系统按优先级排序
// （数值越小 = 优先级越高 = 越先执行）。
func (w *World) RegisterSystem(sys System) {
	w.systems = append(w.systems, sys)
	sort.Slice(w.systems, func(i, j int) bool {
		return w.systems[i].Priority() < w.systems[j].Priority()
	})
}

// Update 按优先级顺序执行所有已注册系统，处理给定的 tick。
// 每个系统只接收匹配其所需组件集的实体。
func (w *World) Update(tick uint64) {
	for _, sys := range w.systems {
		entities := w.Query(sys.RequiredComponents()...)
		sys.Update(tick, entities, w)
	}
}

// Entities 返回世界中存活实体的数量。
func (w *World) Entities() int {
	return len(w.entities)
}

// ---------------------------------------------------------------------------
// 核心组件类型
// ---------------------------------------------------------------------------

// PositionComponent 存储实体的空间位置。
type PositionComponent struct {
	X, Y, Z    float32
	MapID      string
	LayerIndex int
}

func (c *PositionComponent) Type() engine.ComponentType { return CompPosition }

// MovementComponent 存储移动状态。
type MovementComponent struct {
	Speed     float32
	Direction engine.Vector3
	Moving    bool
}

func (c *MovementComponent) Type() engine.ComponentType { return CompMovement }

// CombatAttributeComponent 存储战斗相关属性。
type CombatAttributeComponent struct {
	HP, MaxHP      float64
	MP, MaxMP      float64
	Attack         float64
	Defense        float64
	CritRate       float64
	CritDamage     float64
	Speed          float64
}

func (c *CombatAttributeComponent) Type() engine.ComponentType { return CompCombatAttribute }

// EquipmentComponent 跟踪已装备的物品及锁定状态。
type EquipmentComponent struct {
	Slots  map[engine.EquipmentSlotType]*EquipmentItem
	Locked bool
}

func (c *EquipmentComponent) Type() engine.ComponentType { return CompEquipment }

// EquipmentItem 表示单件装备。
type EquipmentItem struct {
	ID         uint64
	Name       string
	SlotType   engine.EquipmentSlotType
	Quality    engine.EquipmentQuality
	Level      int
	Attributes map[string]float64
}

// SkillComponent 存储技能实例和冷却时间。
type SkillComponent struct {
	Skills    map[uint32]*SkillInstance
	Cooldowns map[uint32]time.Duration
}

func (c *SkillComponent) Type() engine.ComponentType { return CompSkill }

// SkillInstance 表示单个技能实例。
type SkillInstance struct {
	ID    uint32
	Level int
}

// BuffComponent 存储实体上的激活 Buff。
type BuffComponent struct {
	ActiveBuffs []*BuffInstance
}

func (c *BuffComponent) Type() engine.ComponentType { return CompBuff }

// BuffInstance 表示单个激活的 Buff。
type BuffInstance struct {
	BuffID     uint32
	SourceID   engine.EntityID
	StartTick  uint64
	Duration   int
	StackCount int
	Effects    map[string]float64
}

// NetworkComponent 将实体与网络会话关联。
type NetworkComponent struct {
	SessionID string
}

func (c *NetworkComponent) Type() engine.ComponentType { return CompNetwork }

// AOIComponent 存储实体的兴趣区域数据。
type AOIComponent struct {
	Radius          float32
	VisibleEntities map[engine.EntityID]bool
}

func (c *AOIComponent) Type() engine.ComponentType { return CompAOI }
