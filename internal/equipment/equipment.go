// Package equipment implements the EquipmentSystem for the MMRPG game engine.
// It manages equipment items, slot management, attribute calculation, and lock state.
package equipment

import (
	"errors"
	"sync"

	"gfgame/internal/engine"
)

// Errors returned by EquipmentSystem operations.
var (
	ErrSlotMismatch    = errors.New("equipment slot type mismatch")
	ErrSlotEmpty       = errors.New("equipment slot is empty")
	ErrEquipmentLocked = errors.New("equipment locked during combat")
	ErrItemNotFound    = errors.New("equipment item not found")
)

// Attributes holds the combat attributes of a character.
type Attributes struct {
	Attack    float64
	Defense   float64
	CritRate  float64 // 0.0 ~ 1.0
	HP        float64
	Speed     float64
}

// Add returns the sum of two Attributes.
func (a Attributes) Add(b Attributes) Attributes {
	return Attributes{
		Attack:   a.Attack + b.Attack,
		Defense:  a.Defense + b.Defense,
		CritRate: a.CritRate + b.CritRate,
		HP:       a.HP + b.HP,
		Speed:    a.Speed + b.Speed,
	}
}

// EquipmentItem represents a single equipment item.
type EquipmentItem struct {
	ID       string
	Name     string
	SlotType engine.EquipmentSlotType
	Quality  engine.EquipmentQuality
	Level    int
	Bonus    Attributes // stat bonuses granted when equipped
	locked   bool
}

// IsLocked returns whether the item is locked.
func (e *EquipmentItem) IsLocked() bool { return e.locked }

// SetLock sets the lock state of the item.
func (e *EquipmentItem) SetLock(locked bool) { e.locked = locked }

// EquipmentSystem manages equipment for a single character entity.
type EquipmentSystem interface {
	// Equip puts an item into its designated slot.
	// If the slot already has an item, the old item is unequipped first.
	Equip(item *EquipmentItem) (*EquipmentItem, error)
	// Unequip removes the item from the given slot and returns it.
	Unequip(slot engine.EquipmentSlotType) (*EquipmentItem, error)
	// GetEquipped returns the item in the given slot, or nil if empty.
	GetEquipped(slot engine.EquipmentSlotType) *EquipmentItem
	// GetAllEquipped returns a snapshot of all currently equipped items.
	GetAllEquipped() map[engine.EquipmentSlotType]*EquipmentItem
	// CalculateAttributes returns base attributes plus all equipped item bonuses.
	CalculateAttributes() Attributes
	// SetLock locks or unlocks all equipment operations (used during combat).
	SetLock(locked bool)
	// IsLocked returns whether equipment operations are currently locked.
	IsLocked() bool
}

// equipmentSystem is the concrete implementation of EquipmentSystem.
type equipmentSystem struct {
	mu         sync.RWMutex
	slots      map[engine.EquipmentSlotType]*EquipmentItem
	base       Attributes // base character attributes (without equipment)
	combatLock bool
}

// NewEquipmentSystem creates a new EquipmentSystem with the given base attributes.
func NewEquipmentSystem(base Attributes) EquipmentSystem {
	return &equipmentSystem{
		slots: make(map[engine.EquipmentSlotType]*EquipmentItem),
		base:  base,
	}
}

// Equip equips an item. Returns the previously equipped item (or nil) if a swap occurred.
func (es *equipmentSystem) Equip(item *EquipmentItem) (*EquipmentItem, error) {
	if item == nil {
		return nil, ErrItemNotFound
	}

	es.mu.Lock()
	defer es.mu.Unlock()

	if es.combatLock {
		return nil, ErrEquipmentLocked
	}

	// Validate slot type.
	if item.SlotType < engine.SlotWeapon || item.SlotType > engine.SlotRing {
		return nil, ErrSlotMismatch
	}

	old := es.slots[item.SlotType]
	es.slots[item.SlotType] = item
	return old, nil
}

// Unequip removes and returns the item in the given slot.
func (es *equipmentSystem) Unequip(slot engine.EquipmentSlotType) (*EquipmentItem, error) {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.combatLock {
		return nil, ErrEquipmentLocked
	}

	item, ok := es.slots[slot]
	if !ok || item == nil {
		return nil, ErrSlotEmpty
	}

	delete(es.slots, slot)
	return item, nil
}

// GetEquipped returns the item in the given slot, or nil if empty.
func (es *equipmentSystem) GetEquipped(slot engine.EquipmentSlotType) *EquipmentItem {
	es.mu.RLock()
	defer es.mu.RUnlock()
	return es.slots[slot]
}

// GetAllEquipped returns a snapshot of all equipped items.
func (es *equipmentSystem) GetAllEquipped() map[engine.EquipmentSlotType]*EquipmentItem {
	es.mu.RLock()
	defer es.mu.RUnlock()
	result := make(map[engine.EquipmentSlotType]*EquipmentItem, len(es.slots))
	for k, v := range es.slots {
		result[k] = v
	}
	return result
}

// CalculateAttributes returns base + sum of all equipped item bonuses.
func (es *equipmentSystem) CalculateAttributes() Attributes {
	es.mu.RLock()
	defer es.mu.RUnlock()

	total := es.base
	for _, item := range es.slots {
		if item != nil {
			total = total.Add(item.Bonus)
		}
	}
	return total
}

// SetLock sets the combat lock state.
func (es *equipmentSystem) SetLock(locked bool) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.combatLock = locked
}

// IsLocked returns the current combat lock state.
func (es *equipmentSystem) IsLocked() bool {
	es.mu.RLock()
	defer es.mu.RUnlock()
	return es.combatLock
}
