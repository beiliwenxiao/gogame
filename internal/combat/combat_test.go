package combat

import (
	"errors"
	"testing"
	"time"

	"gogame/internal/engine"
	"gogame/internal/equipment"
)

// ---------- helpers ----------

func makeEntity(id engine.EntityID, hp float64) *CombatEntity {
	return &CombatEntity{
		EntityID: id,
		Name:     "entity",
		HP:       hp,
		MaxHP:    hp,
		Attributes: equipment.Attributes{
			Attack:   100,
			Defense:  50,
			CritRate: 0.0,
		},
		Buffs:     make(map[string]*BuffInstance),
		Cooldowns: make(map[string]time.Time),
	}
}

func basicSkill() *SkillDef {
	return &SkillDef{
		ID:         "skill-basic",
		Name:       "Basic Attack",
		TargetMode: engine.TargetSingle,
		Range:      10,
		DamageFormula: func(a, t *CombatEntity) float64 {
			return a.Attributes.Attack - t.Attributes.Defense*0.5
		},
		WindupDuration:   10 * time.Millisecond,
		SettleDuration:   10 * time.Millisecond,
		RecoveryDuration: 10 * time.Millisecond,
		Cooldown:         100 * time.Millisecond,
		InterruptPolicy:  engine.InterruptCancel,
	}
}

func startTestCombat(cs CombatSystem, id string, entities ...*CombatEntity) *CombatContext {
	equipSystems := make(map[engine.EntityID]equipment.EquipmentSystem)
	for _, e := range entities {
		equipSystems[e.EntityID] = equipment.NewEquipmentSystem(e.Attributes)
	}
	ctx, _ := cs.StartCombat(id, entities, equipSystems)
	return ctx
}

// ---------- StartCombat ----------

func TestStartCombat_Basic(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	e2 := makeEntity(2, 500)

	ctx, err := cs.StartCombat("combat-1", []*CombatEntity{e1, e2},
		map[engine.EntityID]equipment.EquipmentSystem{
			1: equipment.NewEquipmentSystem(e1.Attributes),
			2: equipment.NewEquipmentSystem(e2.Attributes),
		})
	if err != nil {
		t.Fatalf("StartCombat failed: %v", err)
	}
	if ctx.ID != "combat-1" {
		t.Errorf("expected combat-1, got %q", ctx.ID)
	}
	if len(ctx.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(ctx.Entities))
	}
}

func TestStartCombat_Duplicate(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	startTestCombat(cs, "combat-1", e1)

	_, err := cs.StartCombat("combat-1", []*CombatEntity{e1},
		map[engine.EntityID]equipment.EquipmentSystem{})
	if err != ErrCombatExists {
		t.Errorf("expected ErrCombatExists, got %v", err)
	}
}

func TestStartCombat_LocksEquipment(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	es1 := equipment.NewEquipmentSystem(e1.Attributes)

	cs.StartCombat("combat-1", []*CombatEntity{e1},
		map[engine.EntityID]equipment.EquipmentSystem{1: es1})

	if !es1.IsLocked() {
		t.Error("expected equipment to be locked after StartCombat")
	}
}

// ---------- EndCombat ----------

func TestEndCombat_Basic(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	e2 := makeEntity(2, 500)
	startTestCombat(cs, "combat-1", e1, e2)

	log, err := cs.EndCombat("combat-1")
	if err != nil {
		t.Fatalf("EndCombat failed: %v", err)
	}
	_ = log

	_, ok := cs.GetContext("combat-1")
	if ok {
		t.Error("expected combat context to be removed after EndCombat")
	}
}

func TestEndCombat_UnlocksEquipment(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	es1 := equipment.NewEquipmentSystem(e1.Attributes)

	cs.StartCombat("combat-1", []*CombatEntity{e1},
		map[engine.EntityID]equipment.EquipmentSystem{1: es1})
	cs.EndCombat("combat-1")

	if es1.IsLocked() {
		t.Error("expected equipment to be unlocked after EndCombat")
	}
}

func TestEndCombat_NotFound(t *testing.T) {
	cs := NewCombatSystem()
	_, err := cs.EndCombat("nonexistent")
	if err != ErrCombatNotFound {
		t.Errorf("expected ErrCombatNotFound, got %v", err)
	}
}

// ---------- GetContext ----------

func TestGetContext(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	startTestCombat(cs, "combat-1", e1)

	ctx, ok := cs.GetContext("combat-1")
	if !ok || ctx == nil {
		t.Fatal("expected to find combat-1")
	}

	_, ok = cs.GetContext("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent combat")
	}
}

// ---------- RegisterSkill / GetSkill ----------

func TestRegisterAndGetSkill(t *testing.T) {
	cs := NewCombatSystem()
	skill := basicSkill()
	cs.RegisterSkill(skill)

	got, ok := cs.GetSkill("skill-basic")
	if !ok || got.ID != "skill-basic" {
		t.Errorf("expected skill-basic, got %v", got)
	}

	_, ok = cs.GetSkill("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent skill")
	}
}

// ---------- UseSkill ----------

func TestUseSkill_DealsDamage(t *testing.T) {
	cs := NewCombatSystem()
	attacker := makeEntity(1, 500)
	defender := makeEntity(2, 500)
	startTestCombat(cs, "c1", attacker, defender)

	skill := basicSkill()
	entries, err := cs.UseSkill("c1", attacker.EntityID, skill)
	if err != nil {
		t.Fatalf("UseSkill failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry")
	}

	// damage = 100 - 50*0.5 = 75
	if entries[0].Damage != 75 {
		t.Errorf("expected damage 75, got %v", entries[0].Damage)
	}
	if defender.HP != 425 {
		t.Errorf("expected defender HP 425, got %v", defender.HP)
	}
}

func TestUseSkill_CritDoublesDamage(t *testing.T) {
	cs := NewCombatSystem()
	attacker := makeEntity(1, 500)
	attacker.Attributes.CritRate = 1.0 // always crit
	defender := makeEntity(2, 500)
	startTestCombat(cs, "c1", attacker, defender)

	skill := basicSkill()
	entries, err := cs.UseSkill("c1", attacker.EntityID, skill)
	if err != nil {
		t.Fatalf("UseSkill failed: %v", err)
	}
	if !entries[0].IsCrit {
		t.Error("expected crit")
	}
	// damage = (100 - 25) * 2 = 150
	if entries[0].Damage != 150 {
		t.Errorf("expected crit damage 150, got %v", entries[0].Damage)
	}
}

func TestUseSkill_Cooldown(t *testing.T) {
	cs := NewCombatSystem()
	attacker := makeEntity(1, 500)
	defender := makeEntity(2, 500)
	startTestCombat(cs, "c1", attacker, defender)

	skill := basicSkill()
	skill.Cooldown = 1 * time.Second

	cs.UseSkill("c1", attacker.EntityID, skill)

	_, err := cs.UseSkill("c1", attacker.EntityID, skill)
	if err != ErrSkillOnCooldown {
		t.Errorf("expected ErrSkillOnCooldown, got %v", err)
	}
}

func TestUseSkill_CombatNotFound(t *testing.T) {
	cs := NewCombatSystem()
	_, err := cs.UseSkill("nonexistent", 1, basicSkill())
	if err != ErrCombatNotFound {
		t.Errorf("expected ErrCombatNotFound, got %v", err)
	}
}

func TestUseSkill_EntityNotInCombat(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	startTestCombat(cs, "c1", e1)

	_, err := cs.UseSkill("c1", engine.EntityID(99), basicSkill())
	if err != ErrEntityNotInCombat {
		t.Errorf("expected ErrEntityNotInCombat, got %v", err)
	}
}

func TestUseSkill_DeadCasterBlocked(t *testing.T) {
	cs := NewCombatSystem()
	attacker := makeEntity(1, 0) // already dead
	defender := makeEntity(2, 500)
	startTestCombat(cs, "c1", attacker, defender)

	_, err := cs.UseSkill("c1", attacker.EntityID, basicSkill())
	if err != ErrEntityNotInCombat {
		t.Errorf("expected ErrEntityNotInCombat for dead caster, got %v", err)
	}
}

func TestUseSkill_GeneratesLog(t *testing.T) {
	cs := NewCombatSystem()
	attacker := makeEntity(1, 500)
	defender := makeEntity(2, 500)
	startTestCombat(cs, "c1", attacker, defender)

	cs.UseSkill("c1", attacker.EntityID, basicSkill())

	ctx, _ := cs.GetContext("c1")
	log := ctx.GetLog()
	// Expect: windup + settle + recovery = at least 3 entries.
	if len(log) < 3 {
		t.Errorf("expected at least 3 log entries, got %d", len(log))
	}
}

// ---------- Buff System ----------

func TestApplyBuff_Basic(t *testing.T) {
	e := makeEntity(1, 500)
	def := &BuffDef{ID: "poison", Name: "Poison", Duration: 5 * time.Second}
	e.ApplyBuff(def)

	if _, ok := e.Buffs["poison"]; !ok {
		t.Error("expected poison buff to be applied")
	}
}

func TestApplyBuff_Stacks(t *testing.T) {
	e := makeEntity(1, 500)
	def := &BuffDef{ID: "bleed", Name: "Bleed", Duration: 5 * time.Second, Stacks: 3}
	e.ApplyBuff(def)
	e.ApplyBuff(def)
	e.ApplyBuff(def)
	e.ApplyBuff(def) // should cap at 3

	if e.Buffs["bleed"].Stacks != 3 {
		t.Errorf("expected 3 stacks, got %d", e.Buffs["bleed"].Stacks)
	}
}

func TestRemoveBuff(t *testing.T) {
	e := makeEntity(1, 500)
	def := &BuffDef{ID: "slow", Duration: 5 * time.Second}
	e.ApplyBuff(def)
	e.RemoveBuff("slow")

	if _, ok := e.Buffs["slow"]; ok {
		t.Error("expected buff to be removed")
	}
}

func TestPruneExpiredBuffs(t *testing.T) {
	e := makeEntity(1, 500)
	def := &BuffDef{ID: "temp", Duration: 1 * time.Millisecond}
	e.ApplyBuff(def)

	time.Sleep(5 * time.Millisecond)
	e.PruneExpiredBuffs()

	if _, ok := e.Buffs["temp"]; ok {
		t.Error("expected expired buff to be pruned")
	}
}

func TestUseSkill_AppliesBuff(t *testing.T) {
	cs := NewCombatSystem()
	attacker := makeEntity(1, 500)
	defender := makeEntity(2, 500)
	startTestCombat(cs, "c1", attacker, defender)

	skill := basicSkill()
	skill.BuffsApplied = []*BuffDef{
		{ID: "slow", Name: "Slow", Duration: 5 * time.Second},
	}

	cs.UseSkill("c1", attacker.EntityID, skill)

	if _, ok := defender.Buffs["slow"]; !ok {
		t.Error("expected slow buff on defender after skill use")
	}
}

// ---------- CombatEntity helpers ----------

func TestApplyDamage_ClampToZero(t *testing.T) {
	e := makeEntity(1, 100)
	e.ApplyDamage(200)
	if e.HP != 0 {
		t.Errorf("expected HP 0, got %v", e.HP)
	}
}

func TestIsAlive(t *testing.T) {
	e := makeEntity(1, 100)
	if !e.IsAlive() {
		t.Error("expected entity to be alive")
	}
	e.ApplyDamage(100)
	if e.IsAlive() {
		t.Error("expected entity to be dead")
	}
}

// ---------- Task 14.2: Equipment Lock During Combat ----------

func TestEquipmentLock_DuringCombat_RejectsEquip(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	es1 := equipment.NewEquipmentSystem(e1.Attributes)

	cs.StartCombat("c-lock", []*CombatEntity{e1},
		map[engine.EntityID]equipment.EquipmentSystem{1: es1})

	sword := &equipment.EquipmentItem{
		ID:       "sword",
		SlotType: engine.SlotWeapon,
		Quality:  engine.QualityNormal,
	}
	_, err := es1.Equip(sword)
	if err != equipment.ErrEquipmentLocked {
		t.Errorf("expected ErrEquipmentLocked during combat, got %v", err)
	}
}

func TestEquipmentLock_DuringCombat_RejectsUnequip(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	es1 := equipment.NewEquipmentSystem(e1.Attributes)

	// Equip before combat starts.
	sword := &equipment.EquipmentItem{
		ID:       "sword",
		SlotType: engine.SlotWeapon,
		Quality:  engine.QualityNormal,
	}
	es1.Equip(sword)

	cs.StartCombat("c-lock2", []*CombatEntity{e1},
		map[engine.EntityID]equipment.EquipmentSystem{1: es1})

	_, err := es1.Unequip(engine.SlotWeapon)
	if err != equipment.ErrEquipmentLocked {
		t.Errorf("expected ErrEquipmentLocked on Unequip during combat, got %v", err)
	}
}

func TestEquipmentLock_AfterCombatEnd_AllowsEquip(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	es1 := equipment.NewEquipmentSystem(e1.Attributes)

	cs.StartCombat("c-lock3", []*CombatEntity{e1},
		map[engine.EntityID]equipment.EquipmentSystem{1: es1})
	cs.EndCombat("c-lock3")

	sword := &equipment.EquipmentItem{
		ID:       "sword",
		SlotType: engine.SlotWeapon,
		Quality:  engine.QualityNormal,
	}
	_, err := es1.Equip(sword)
	if err != nil {
		t.Errorf("expected Equip to succeed after combat ends, got %v", err)
	}
}

// Simulate abnormal exit (EndCombat called on disconnect) — lock must be released.
func TestEquipmentLock_AbnormalExit_UnlocksOnEnd(t *testing.T) {
	cs := NewCombatSystem()
	e1 := makeEntity(1, 500)
	e2 := makeEntity(2, 500)
	es1 := equipment.NewEquipmentSystem(e1.Attributes)
	es2 := equipment.NewEquipmentSystem(e2.Attributes)

	cs.StartCombat("c-abnormal", []*CombatEntity{e1, e2},
		map[engine.EntityID]equipment.EquipmentSystem{1: es1, 2: es2})

	// Simulate disconnect: EndCombat is called before combat naturally finishes.
	cs.EndCombat("c-abnormal")

	if es1.IsLocked() || es2.IsLocked() {
		t.Error("expected both equipment systems to be unlocked after abnormal EndCombat")
	}
}

// ---------- Task 14.4: Combat Data Memoization Tests ----------

// mockStore implements PersistenceStore for testing.
type mockStore struct {
	characters map[engine.EntityID]*CharacterSnapshot
	savedLogs  map[string][]*CombatLogEntry
	loadErr    error
	saveErr    error
}

func newMockStore() *mockStore {
	return &mockStore{
		characters: make(map[engine.EntityID]*CharacterSnapshot),
		savedLogs:  make(map[string][]*CombatLogEntry),
	}
}

func (m *mockStore) LoadCharacter(id engine.EntityID) (*CharacterSnapshot, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	snap, ok := m.characters[id]
	if !ok {
		return nil, ErrEntityNotInCombat
	}
	return snap, nil
}

func (m *mockStore) SaveCombatResult(combatID string, log []*CombatLogEntry) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.savedLogs[combatID] = log
	return nil
}

func TestStartCombatFromStore_Basic(t *testing.T) {
	store := newMockStore()
	store.characters[1] = &CharacterSnapshot{
		EntityID: 1, Name: "hero", HP: 500, MaxHP: 500,
		Attributes: equipment.Attributes{Attack: 100, Defense: 50},
	}
	store.characters[2] = &CharacterSnapshot{
		EntityID: 2, Name: "monster", HP: 300, MaxHP: 300,
		Attributes: equipment.Attributes{Attack: 80, Defense: 30},
	}

	cs := NewCombatSystem()
	ctx, err := StartCombatFromStore(cs, store, "c-store-1",
		[]engine.EntityID{1, 2},
		map[engine.EntityID]equipment.EquipmentSystem{})
	if err != nil {
		t.Fatalf("StartCombatFromStore failed: %v", err)
	}
	if len(ctx.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(ctx.Entities))
	}

	e1, ok := ctx.GetEntity(1)
	if !ok || e1.HP != 500 {
		t.Errorf("expected entity 1 HP 500, got %v", e1.HP)
	}
}

func TestStartCombatFromStore_LoadError_CancelsCombat(t *testing.T) {
	store := newMockStore()
	store.loadErr = errors.New("db connection failed")

	cs := NewCombatSystem()
	_, err := StartCombatFromStore(cs, store, "c-store-fail",
		[]engine.EntityID{1},
		map[engine.EntityID]equipment.EquipmentSystem{})
	if err == nil {
		t.Fatal("expected error when load fails")
	}

	// Combat should NOT have been created.
	_, ok := cs.GetContext("c-store-fail")
	if ok {
		t.Error("expected combat context to not exist after load failure")
	}
}

func TestStartCombatFromStore_IsolatesFromStore(t *testing.T) {
	store := newMockStore()
	store.characters[1] = &CharacterSnapshot{
		EntityID: 1, Name: "hero", HP: 500, MaxHP: 500,
		Attributes: equipment.Attributes{Attack: 100},
	}

	cs := NewCombatSystem()
	ctx, _ := StartCombatFromStore(cs, store, "c-iso",
		[]engine.EntityID{1},
		map[engine.EntityID]equipment.EquipmentSystem{})

	// Modify the store snapshot — combat entity should be unaffected.
	store.characters[1].HP = 1
	store.characters[1].Attributes.Attack = 999

	e1, _ := ctx.GetEntity(1)
	if e1.HP != 500 {
		t.Errorf("expected combat HP 500 (isolated), got %v", e1.HP)
	}
	if e1.Attributes.Attack != 100 {
		t.Errorf("expected combat attack 100 (isolated), got %v", e1.Attributes.Attack)
	}
}

func TestEndCombatAndSave_Basic(t *testing.T) {
	store := newMockStore()
	store.characters[1] = &CharacterSnapshot{EntityID: 1, HP: 500, MaxHP: 500,
		Attributes: equipment.Attributes{Attack: 100, Defense: 50}}
	store.characters[2] = &CharacterSnapshot{EntityID: 2, HP: 300, MaxHP: 300,
		Attributes: equipment.Attributes{Attack: 80, Defense: 30}}

	cs := NewCombatSystem()
	StartCombatFromStore(cs, store, "c-save",
		[]engine.EntityID{1, 2},
		map[engine.EntityID]equipment.EquipmentSystem{})

	log, err := EndCombatAndSave(cs, store, "c-save")
	if err != nil {
		t.Fatalf("EndCombatAndSave failed: %v", err)
	}
	_ = log

	if _, ok := store.savedLogs["c-save"]; !ok {
		t.Error("expected combat log to be saved to store")
	}
}

func TestEndCombatAndSave_SaveError_ReturnsLog(t *testing.T) {
	store := newMockStore()
	store.characters[1] = &CharacterSnapshot{EntityID: 1, HP: 500, MaxHP: 500,
		Attributes: equipment.Attributes{}}
	store.saveErr = errors.New("db write failed")

	cs := NewCombatSystem()
	StartCombatFromStore(cs, store, "c-savefail",
		[]engine.EntityID{1},
		map[engine.EntityID]equipment.EquipmentSystem{})

	log, err := EndCombatAndSave(cs, store, "c-savefail")
	// Should return both the log AND the error.
	if err == nil {
		t.Error("expected save error to be returned")
	}
	if log == nil {
		t.Error("expected log to be returned even on save error")
	}
}

func TestCombatContext_DoesNotAffectStore(t *testing.T) {
	store := newMockStore()
	store.characters[1] = &CharacterSnapshot{EntityID: 1, HP: 500, MaxHP: 500,
		Attributes: equipment.Attributes{Attack: 100, Defense: 50}}
	store.characters[2] = &CharacterSnapshot{EntityID: 2, HP: 300, MaxHP: 300,
		Attributes: equipment.Attributes{Attack: 80, Defense: 30}}

	cs := NewCombatSystem()
	ctx, _ := StartCombatFromStore(cs, store, "c-iso2",
		[]engine.EntityID{1, 2},
		map[engine.EntityID]equipment.EquipmentSystem{})

	// Deal damage in combat context.
	e2, _ := ctx.GetEntity(2)
	e2.ApplyDamage(200)

	// Store snapshot should be unchanged.
	if store.characters[2].HP != 300 {
		t.Errorf("expected store HP 300 (unchanged), got %v", store.characters[2].HP)
	}
}
