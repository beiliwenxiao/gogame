package equipment

import (
	"testing"

	"gogame/internal/engine"
)

func baseAttrs() Attributes {
	return Attributes{Attack: 100, Defense: 50, CritRate: 0.05, HP: 1000, Speed: 10}
}

func makeItem(id string, slot engine.EquipmentSlotType, quality engine.EquipmentQuality) *EquipmentItem {
	return &EquipmentItem{
		ID:       id,
		Name:     id,
		SlotType: slot,
		Quality:  quality,
		Level:    1,
		Bonus:    Attributes{Attack: 10, Defense: 5, CritRate: 0.01},
	}
}

func TestEquip_Basic(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())

	sword := makeItem("sword-1", engine.SlotWeapon, engine.QualityNormal)
	old, err := es.Equip(sword)
	if err != nil {
		t.Fatalf("Equip failed: %v", err)
	}
	if old != nil {
		t.Error("expected no previous item")
	}

	got := es.GetEquipped(engine.SlotWeapon)
	if got == nil || got.ID != "sword-1" {
		t.Errorf("expected sword-1 in weapon slot, got %v", got)
	}
}

func TestEquip_SwapOldItem(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())

	sword1 := makeItem("sword-1", engine.SlotWeapon, engine.QualityNormal)
	sword2 := makeItem("sword-2", engine.SlotWeapon, engine.QualityRare)

	es.Equip(sword1)
	old, err := es.Equip(sword2)
	if err != nil {
		t.Fatalf("Equip swap failed: %v", err)
	}
	if old == nil || old.ID != "sword-1" {
		t.Errorf("expected old item sword-1, got %v", old)
	}

	got := es.GetEquipped(engine.SlotWeapon)
	if got.ID != "sword-2" {
		t.Errorf("expected sword-2 in slot, got %q", got.ID)
	}
}

func TestEquip_NilItem(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())
	_, err := es.Equip(nil)
	if err != ErrItemNotFound {
		t.Errorf("expected ErrItemNotFound, got %v", err)
	}
}

func TestUnequip_Basic(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())

	helmet := makeItem("helm-1", engine.SlotHelmet, engine.QualityEpic)
	es.Equip(helmet)

	item, err := es.Unequip(engine.SlotHelmet)
	if err != nil {
		t.Fatalf("Unequip failed: %v", err)
	}
	if item.ID != "helm-1" {
		t.Errorf("expected helm-1, got %q", item.ID)
	}

	if es.GetEquipped(engine.SlotHelmet) != nil {
		t.Error("expected slot to be empty after unequip")
	}
}

func TestUnequip_EmptySlot(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())
	_, err := es.Unequip(engine.SlotBoots)
	if err != ErrSlotEmpty {
		t.Errorf("expected ErrSlotEmpty, got %v", err)
	}
}

func TestGetAllEquipped(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())

	es.Equip(makeItem("sword-1", engine.SlotWeapon, engine.QualityNormal))
	es.Equip(makeItem("helm-1", engine.SlotHelmet, engine.QualityRare))
	es.Equip(makeItem("armor-1", engine.SlotArmor, engine.QualityEpic))

	all := es.GetAllEquipped()
	if len(all) != 3 {
		t.Errorf("expected 3 equipped items, got %d", len(all))
	}
	if all[engine.SlotWeapon].ID != "sword-1" {
		t.Errorf("expected sword-1 in weapon slot")
	}
}

func TestCalculateAttributes_NoEquipment(t *testing.T) {
	base := baseAttrs()
	es := NewEquipmentSystem(base)

	attrs := es.CalculateAttributes()
	if attrs.Attack != base.Attack {
		t.Errorf("expected attack %v, got %v", base.Attack, attrs.Attack)
	}
}

func TestCalculateAttributes_WithEquipment(t *testing.T) {
	base := Attributes{Attack: 100, Defense: 50}
	es := NewEquipmentSystem(base)

	sword := &EquipmentItem{
		ID: "sword", SlotType: engine.SlotWeapon,
		Bonus: Attributes{Attack: 30, Defense: 10},
	}
	helm := &EquipmentItem{
		ID: "helm", SlotType: engine.SlotHelmet,
		Bonus: Attributes{Attack: 5, Defense: 20},
	}
	es.Equip(sword)
	es.Equip(helm)

	attrs := es.CalculateAttributes()
	if attrs.Attack != 135 {
		t.Errorf("expected attack 135, got %v", attrs.Attack)
	}
	if attrs.Defense != 80 {
		t.Errorf("expected defense 80, got %v", attrs.Defense)
	}
}

func TestCalculateAttributes_AfterUnequip(t *testing.T) {
	base := Attributes{Attack: 100}
	es := NewEquipmentSystem(base)

	sword := &EquipmentItem{
		ID: "sword", SlotType: engine.SlotWeapon,
		Bonus: Attributes{Attack: 50},
	}
	es.Equip(sword)
	es.Unequip(engine.SlotWeapon)

	attrs := es.CalculateAttributes()
	if attrs.Attack != 100 {
		t.Errorf("expected attack 100 after unequip, got %v", attrs.Attack)
	}
}

func TestCalculateAttributes_SwapRecalculates(t *testing.T) {
	base := Attributes{Attack: 100}
	es := NewEquipmentSystem(base)

	sword1 := &EquipmentItem{ID: "s1", SlotType: engine.SlotWeapon, Bonus: Attributes{Attack: 50}}
	sword2 := &EquipmentItem{ID: "s2", SlotType: engine.SlotWeapon, Bonus: Attributes{Attack: 80}}

	es.Equip(sword1)
	es.Equip(sword2) // swap

	attrs := es.CalculateAttributes()
	if attrs.Attack != 180 {
		t.Errorf("expected attack 180 after swap, got %v", attrs.Attack)
	}
}

func TestSetLock_BlocksEquip(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())
	es.SetLock(true)

	if !es.IsLocked() {
		t.Error("expected IsLocked to return true")
	}

	_, err := es.Equip(makeItem("sword-1", engine.SlotWeapon, engine.QualityNormal))
	if err != ErrEquipmentLocked {
		t.Errorf("expected ErrEquipmentLocked on Equip, got %v", err)
	}
}

func TestSetLock_BlocksUnequip(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())
	es.Equip(makeItem("sword-1", engine.SlotWeapon, engine.QualityNormal))
	es.SetLock(true)

	_, err := es.Unequip(engine.SlotWeapon)
	if err != ErrEquipmentLocked {
		t.Errorf("expected ErrEquipmentLocked on Unequip, got %v", err)
	}
}

func TestSetLock_UnlockAllowsOperations(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())
	es.SetLock(true)
	es.SetLock(false)

	if es.IsLocked() {
		t.Error("expected IsLocked to return false after unlock")
	}

	_, err := es.Equip(makeItem("sword-1", engine.SlotWeapon, engine.QualityNormal))
	if err != nil {
		t.Errorf("expected Equip to succeed after unlock, got %v", err)
	}
}

func TestAllSlots(t *testing.T) {
	es := NewEquipmentSystem(baseAttrs())

	slots := []struct {
		id   string
		slot engine.EquipmentSlotType
	}{
		{"weapon", engine.SlotWeapon},
		{"helmet", engine.SlotHelmet},
		{"armor", engine.SlotArmor},
		{"boots", engine.SlotBoots},
		{"necklace", engine.SlotNecklace},
		{"ring", engine.SlotRing},
	}

	for _, s := range slots {
		item := makeItem(s.id, s.slot, engine.QualityNormal)
		_, err := es.Equip(item)
		if err != nil {
			t.Errorf("Equip %s failed: %v", s.id, err)
		}
	}

	all := es.GetAllEquipped()
	if len(all) != 6 {
		t.Errorf("expected 6 equipped items, got %d", len(all))
	}
}

func TestItemLock(t *testing.T) {
	item := makeItem("sword-1", engine.SlotWeapon, engine.QualityLegendary)
	if item.IsLocked() {
		t.Error("new item should not be locked")
	}
	item.SetLock(true)
	if !item.IsLocked() {
		t.Error("item should be locked after SetLock(true)")
	}
	item.SetLock(false)
	if item.IsLocked() {
		t.Error("item should be unlocked after SetLock(false)")
	}
}
