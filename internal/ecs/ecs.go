// Package ecs implements the Entity-Component-System architecture for the MMRPG game engine.
package ecs

import (
	"sort"
	"sync/atomic"
	"time"

	"gfgame/internal/engine"
)

// ComponentType constants for all core component types.
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

// Component is the interface that all ECS components must implement.
type Component interface {
	Type() engine.ComponentType
}

// System is the interface for ECS systems that process entities with specific component combinations.
type System interface {
	Name() string
	Priority() int
	RequiredComponents() []engine.ComponentType
	Update(tick uint64, entities []engine.EntityID, world *World)
}

// EntityManager defines the interface for managing entities, components, and systems.
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

// World is the ECS world that stores entities, components, and systems.
// Each Scene maintains its own World instance.
type World struct {
	entities map[engine.EntityID]map[engine.ComponentType]Component
	systems  []System
	nextID   atomic.Uint64
}

// NewWorld creates a new ECS World instance that implements EntityManager.
func NewWorld() *World {
	return &World{
		entities: make(map[engine.EntityID]map[engine.ComponentType]Component),
	}
}

// CreateEntity allocates a globally unique EntityID and registers the entity.
func (w *World) CreateEntity() engine.EntityID {
	id := engine.EntityID(w.nextID.Add(1))
	w.entities[id] = make(map[engine.ComponentType]Component)
	return id
}

// DestroyEntity removes all components from the entity and deletes it from the world.
func (w *World) DestroyEntity(id engine.EntityID) {
	delete(w.entities, id)
}

// InjectEntity registers an entity with a specific ID in the world.
// This is used for cross-scene entity transfer where the entity ID must be preserved.
// If the entity already exists, this is a no-op.
func (w *World) InjectEntity(id engine.EntityID) {
	if _, exists := w.entities[id]; exists {
		return
	}
	w.entities[id] = make(map[engine.ComponentType]Component)
}

// AddComponent attaches a component to the specified entity.
// If the entity does not exist, the call is a no-op.
func (w *World) AddComponent(id engine.EntityID, comp Component) {
	comps, ok := w.entities[id]
	if !ok {
		return
	}
	comps[comp.Type()] = comp
}

// RemoveComponent detaches a component of the given type from the entity.
func (w *World) RemoveComponent(id engine.EntityID, compType engine.ComponentType) {
	comps, ok := w.entities[id]
	if !ok {
		return
	}
	delete(comps, compType)
}

// GetComponent retrieves a component of the given type from the entity.
func (w *World) GetComponent(id engine.EntityID, compType engine.ComponentType) (Component, bool) {
	comps, ok := w.entities[id]
	if !ok {
		return nil, false
	}
	comp, found := comps[compType]
	return comp, found
}

// Query returns all entity IDs that possess every one of the specified component types.
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

// RegisterSystem adds a system to the world. Systems are kept sorted by priority
// (lower number = higher priority = runs first).
func (w *World) RegisterSystem(sys System) {
	w.systems = append(w.systems, sys)
	sort.Slice(w.systems, func(i, j int) bool {
		return w.systems[i].Priority() < w.systems[j].Priority()
	})
}

// Update executes all registered systems in priority order for the given tick.
// Each system receives only the entities that match its required component set.
func (w *World) Update(tick uint64) {
	for _, sys := range w.systems {
		entities := w.Query(sys.RequiredComponents()...)
		sys.Update(tick, entities, w)
	}
}

// Entities returns the number of live entities in the world.
func (w *World) Entities() int {
	return len(w.entities)
}

// ---------------------------------------------------------------------------
// Core Component types
// ---------------------------------------------------------------------------

// PositionComponent stores the spatial position of an entity.
type PositionComponent struct {
	X, Y, Z    float32
	MapID      string
	LayerIndex int
}

func (c *PositionComponent) Type() engine.ComponentType { return CompPosition }

// MovementComponent stores movement state.
type MovementComponent struct {
	Speed     float32
	Direction engine.Vector3
	Moving    bool
}

func (c *MovementComponent) Type() engine.ComponentType { return CompMovement }

// CombatAttributeComponent stores combat-related stats.
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

// EquipmentComponent tracks equipped items and lock state.
type EquipmentComponent struct {
	Slots  map[engine.EquipmentSlotType]*EquipmentItem
	Locked bool
}

func (c *EquipmentComponent) Type() engine.ComponentType { return CompEquipment }

// EquipmentItem represents a single piece of equipment.
type EquipmentItem struct {
	ID         uint64
	Name       string
	SlotType   engine.EquipmentSlotType
	Quality    engine.EquipmentQuality
	Level      int
	Attributes map[string]float64
}

// SkillComponent stores skill instances and cooldowns.
type SkillComponent struct {
	Skills    map[uint32]*SkillInstance
	Cooldowns map[uint32]time.Duration
}

func (c *SkillComponent) Type() engine.ComponentType { return CompSkill }

// SkillInstance represents a single skill.
type SkillInstance struct {
	ID    uint32
	Level int
}

// BuffComponent stores active buffs on an entity.
type BuffComponent struct {
	ActiveBuffs []*BuffInstance
}

func (c *BuffComponent) Type() engine.ComponentType { return CompBuff }

// BuffInstance represents a single active buff.
type BuffInstance struct {
	BuffID     uint32
	SourceID   engine.EntityID
	StartTick  uint64
	Duration   int
	StackCount int
	Effects    map[string]float64
}

// NetworkComponent associates an entity with a network session.
type NetworkComponent struct {
	SessionID string
}

func (c *NetworkComponent) Type() engine.ComponentType { return CompNetwork }

// AOIComponent stores area-of-interest data for an entity.
type AOIComponent struct {
	Radius          float32
	VisibleEntities map[engine.EntityID]bool
}

func (c *AOIComponent) Type() engine.ComponentType { return CompAOI }
