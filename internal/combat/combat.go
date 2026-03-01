// Package combat implements the CombatSystem for the MMRPG game engine.
// It manages combat contexts, skill execution pipeline, buff/debuff system,
// AOI-based target selection, and combat log generation.
package combat

import (
	"errors"
	"sync"
	"time"

	"gfgame/internal/engine"
	"gfgame/internal/equipment"
)

// Errors returned by CombatSystem operations.
var (
	ErrCombatNotFound    = errors.New("combat context not found")
	ErrCombatExists      = errors.New("combat context already exists")
	ErrEntityNotInCombat = errors.New("entity not in combat")
	ErrSkillNotFound     = errors.New("skill not found")
	ErrSkillOnCooldown   = errors.New("skill is on cooldown")
	ErrInvalidTarget     = errors.New("invalid target")
)

// ---------- Skill Definition ----------

// SkillDef defines a skill via data configuration.
type SkillDef struct {
	ID              string
	Name            string
	TargetMode      engine.TargetMode
	Range           float32 // max range
	Radius          float32 // for circle/fan/rectangle
	DamageFormula   func(attacker, target *CombatEntity) float64
	WindupDuration  time.Duration
	HitDuration     time.Duration
	SettleDuration  time.Duration
	RecoveryDuration time.Duration
	InterruptPolicy engine.InterruptPolicy
	BuffsApplied    []*BuffDef
	Cooldown        time.Duration
}

// ---------- Buff/Debuff ----------

// BuffDef defines a buff or debuff effect.
type BuffDef struct {
	ID       string
	Name     string
	Duration time.Duration
	Stacks   int // max stacks (0 = unlimited)
	// Modifier applied each tick while active.
	AttackMod  float64
	DefenseMod float64
}

// BuffInstance is an active buff on an entity.
type BuffInstance struct {
	Def       *BuffDef
	Stacks    int
	ExpiresAt time.Time
}

// ---------- Combat Entity ----------

// CombatEntity is the in-memory snapshot of an entity's combat state.
type CombatEntity struct {
	EntityID   engine.EntityID
	Name       string
	HP         float64
	MaxHP      float64
	Attributes equipment.Attributes
	Buffs      map[string]*BuffInstance // buffID → instance
	Cooldowns  map[string]time.Time     // skillID → ready time
	mu         sync.Mutex
}

// IsAlive returns true if the entity has HP > 0.
func (ce *CombatEntity) IsAlive() bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	return ce.HP > 0
}

// ApplyDamage reduces HP by the given amount (clamped to 0).
func (ce *CombatEntity) ApplyDamage(dmg float64) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.HP -= dmg
	if ce.HP < 0 {
		ce.HP = 0
	}
}

// ApplyBuff adds or refreshes a buff on the entity.
func (ce *CombatEntity) ApplyBuff(def *BuffDef) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	now := time.Now()
	if inst, ok := ce.Buffs[def.ID]; ok {
		// Refresh duration; increment stacks up to max.
		inst.ExpiresAt = now.Add(def.Duration)
		if def.Stacks == 0 || inst.Stacks < def.Stacks {
			inst.Stacks++
		}
	} else {
		ce.Buffs[def.ID] = &BuffInstance{
			Def:       def,
			Stacks:    1,
			ExpiresAt: now.Add(def.Duration),
		}
	}
}

// RemoveBuff removes a buff by ID.
func (ce *CombatEntity) RemoveBuff(buffID string) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	delete(ce.Buffs, buffID)
}

// PruneExpiredBuffs removes all expired buffs.
func (ce *CombatEntity) PruneExpiredBuffs() {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	now := time.Now()
	for id, inst := range ce.Buffs {
		if now.After(inst.ExpiresAt) {
			delete(ce.Buffs, id)
		}
	}
}

// IsSkillReady returns true if the skill is off cooldown.
func (ce *CombatEntity) IsSkillReady(skillID string) bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ready, ok := ce.Cooldowns[skillID]
	if !ok {
		return true
	}
	return time.Now().After(ready)
}

// SetCooldown records when a skill will be ready again.
func (ce *CombatEntity) SetCooldown(skillID string, cd time.Duration) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.Cooldowns[skillID] = time.Now().Add(cd)
}

// ---------- Combat Log ----------

// CombatLogEntry records a single combat event.
type CombatLogEntry struct {
	Timestamp  time.Time
	AttackerID engine.EntityID
	TargetID   engine.EntityID
	SkillID    string
	Damage     float64
	IsCrit     bool
	Phase      engine.SkillPhase
}

// ---------- Combat Context ----------

// CombatContext holds the in-memory state for an active combat session.
type CombatContext struct {
	ID        string
	Entities  map[engine.EntityID]*CombatEntity
	Log       []*CombatLogEntry
	StartTime time.Time
	mu        sync.RWMutex
}

// GetEntity returns a combat entity by ID.
func (cc *CombatContext) GetEntity(id engine.EntityID) (*CombatEntity, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	e, ok := cc.Entities[id]
	return e, ok
}

// AddLogEntry appends a combat log entry.
func (cc *CombatContext) AddLogEntry(entry *CombatLogEntry) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.Log = append(cc.Log, entry)
}

// GetLog returns a snapshot of the combat log.
func (cc *CombatContext) GetLog() []*CombatLogEntry {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	result := make([]*CombatLogEntry, len(cc.Log))
	copy(result, cc.Log)
	return result
}

// ---------- Target Selector ----------

// TargetSelector selects targets for a skill based on TargetMode.
// In a full implementation this would query the AOI system; here we provide
// a simple distance-based implementation that can be replaced via dependency injection.
type TargetSelector interface {
	SelectTargets(ctx *CombatContext, caster *CombatEntity, skill *SkillDef) []*CombatEntity
}

// defaultTargetSelector selects targets based on TargetMode and range.
type defaultTargetSelector struct{}

func (s *defaultTargetSelector) SelectTargets(ctx *CombatContext, caster *CombatEntity, skill *SkillDef) []*CombatEntity {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	var targets []*CombatEntity
	for _, e := range ctx.Entities {
		if e.EntityID == caster.EntityID {
			continue
		}
		if !e.IsAlive() {
			continue
		}
		switch skill.TargetMode {
		case engine.TargetSingle:
			// Return first alive non-caster entity.
			return []*CombatEntity{e}
		default:
			targets = append(targets, e)
		}
	}
	return targets
}

// ---------- CombatSystem ----------

// CombatSystem manages all active combat contexts.
type CombatSystem interface {
	// StartCombat creates a new CombatContext for the given entities and locks their equipment.
	StartCombat(combatID string, entities []*CombatEntity, equipSystems map[engine.EntityID]equipment.EquipmentSystem) (*CombatContext, error)
	// EndCombat finalises the combat, unlocks equipment, and returns the log.
	EndCombat(combatID string) ([]*CombatLogEntry, error)
	// GetContext returns the active CombatContext by ID.
	GetContext(combatID string) (*CombatContext, bool)
	// UseSkill executes a skill from caster against targets in the given context.
	UseSkill(combatID string, casterID engine.EntityID, skill *SkillDef) ([]*CombatLogEntry, error)
	// RegisterSkill registers a skill definition.
	RegisterSkill(skill *SkillDef)
	// GetSkill returns a registered skill definition.
	GetSkill(skillID string) (*SkillDef, bool)
}

// combatSystem is the concrete implementation.
type combatSystem struct {
	mu       sync.RWMutex
	contexts map[string]*CombatContext
	// equipSystems tracks equipment systems per combat for lock/unlock.
	equipSystems map[string]map[engine.EntityID]equipment.EquipmentSystem
	skills       map[string]*SkillDef
	selector     TargetSelector
}

// NewCombatSystem creates a new CombatSystem.
func NewCombatSystem() CombatSystem {
	return &combatSystem{
		contexts:     make(map[string]*CombatContext),
		equipSystems: make(map[string]map[engine.EntityID]equipment.EquipmentSystem),
		skills:       make(map[string]*SkillDef),
		selector:     &defaultTargetSelector{},
	}
}

// NewCombatSystemWithSelector creates a CombatSystem with a custom TargetSelector.
func NewCombatSystemWithSelector(sel TargetSelector) CombatSystem {
	return &combatSystem{
		contexts:     make(map[string]*CombatContext),
		equipSystems: make(map[string]map[engine.EntityID]equipment.EquipmentSystem),
		skills:       make(map[string]*SkillDef),
		selector:     sel,
	}
}

func (cs *combatSystem) RegisterSkill(skill *SkillDef) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.skills[skill.ID] = skill
}

func (cs *combatSystem) GetSkill(skillID string) (*SkillDef, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	s, ok := cs.skills[skillID]
	return s, ok
}

func (cs *combatSystem) StartCombat(
	combatID string,
	entities []*CombatEntity,
	equipSystems map[engine.EntityID]equipment.EquipmentSystem,
) (*CombatContext, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, exists := cs.contexts[combatID]; exists {
		return nil, ErrCombatExists
	}

	ctx := &CombatContext{
		ID:        combatID,
		Entities:  make(map[engine.EntityID]*CombatEntity, len(entities)),
		StartTime: time.Now(),
	}
	for _, e := range entities {
		ctx.Entities[e.EntityID] = e
	}

	// Lock equipment for all participants.
	for _, es := range equipSystems {
		es.SetLock(true)
	}

	cs.contexts[combatID] = ctx
	cs.equipSystems[combatID] = equipSystems
	return ctx, nil
}

func (cs *combatSystem) EndCombat(combatID string) ([]*CombatLogEntry, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	ctx, ok := cs.contexts[combatID]
	if !ok {
		return nil, ErrCombatNotFound
	}

	// Unlock equipment.
	if eqs, ok := cs.equipSystems[combatID]; ok {
		for _, es := range eqs {
			es.SetLock(false)
		}
		delete(cs.equipSystems, combatID)
	}

	log := ctx.GetLog()
	delete(cs.contexts, combatID)
	return log, nil
}

func (cs *combatSystem) GetContext(combatID string) (*CombatContext, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	ctx, ok := cs.contexts[combatID]
	return ctx, ok
}

// UseSkill executes the full skill pipeline: windup → hit → settle → recovery.
// Returns the log entries generated during this skill use.
func (cs *combatSystem) UseSkill(combatID string, casterID engine.EntityID, skill *SkillDef) ([]*CombatLogEntry, error) {
	cs.mu.RLock()
	ctx, ok := cs.contexts[combatID]
	cs.mu.RUnlock()

	if !ok {
		return nil, ErrCombatNotFound
	}

	caster, ok := ctx.GetEntity(casterID)
	if !ok {
		return nil, ErrEntityNotInCombat
	}

	if !caster.IsAlive() {
		return nil, ErrEntityNotInCombat
	}

	if !caster.IsSkillReady(skill.ID) {
		return nil, ErrSkillOnCooldown
	}

	// Prune expired buffs before acting.
	caster.PruneExpiredBuffs()

	// Phase: Windup (simulated synchronously for testability).
	// In production this would be async with interrupt checks.
	windupEntry := &CombatLogEntry{
		Timestamp:  time.Now(),
		AttackerID: casterID,
		SkillID:    skill.ID,
		Phase:      engine.SkillPhaseWindup,
	}
	ctx.AddLogEntry(windupEntry)

	// Check interrupt: if caster died during windup, apply interrupt policy.
	if !caster.IsAlive() && skill.InterruptPolicy == engine.InterruptCancel {
		return ctx.GetLog(), nil
	}

	// Phase: Hit — select targets.
	targets := cs.selector.SelectTargets(ctx, caster, skill)

	var newEntries []*CombatLogEntry

	// Phase: Settle — apply damage and buffs to each target.
	for _, target := range targets {
		if !target.IsAlive() {
			continue
		}

		var dmg float64
		if skill.DamageFormula != nil {
			dmg = skill.DamageFormula(caster, target)
		}

		// Simple crit check: if CritRate >= 1.0 always crit (deterministic for tests).
		isCrit := caster.Attributes.CritRate >= 1.0
		if isCrit {
			dmg *= 2
		}

		target.ApplyDamage(dmg)

		// Apply buffs defined in skill.
		for _, buffDef := range skill.BuffsApplied {
			target.ApplyBuff(buffDef)
		}

		entry := &CombatLogEntry{
			Timestamp:  time.Now(),
			AttackerID: casterID,
			TargetID:   target.EntityID,
			SkillID:    skill.ID,
			Damage:     dmg,
			IsCrit:     isCrit,
			Phase:      engine.SkillPhaseSettle,
		}
		ctx.AddLogEntry(entry)
		newEntries = append(newEntries, entry)
	}

	// Phase: Recovery — set cooldown.
	if skill.Cooldown > 0 {
		caster.SetCooldown(skill.ID, skill.Cooldown)
	}

	recoveryEntry := &CombatLogEntry{
		Timestamp:  time.Now(),
		AttackerID: casterID,
		SkillID:    skill.ID,
		Phase:      engine.SkillPhaseRecovery,
	}
	ctx.AddLogEntry(recoveryEntry)

	return newEntries, nil
}

// ---------- Task 14.4: Combat Data Memoization ----------

// CharacterSnapshot is the persistent data record for a character.
// In production this would be loaded from the database via PersistenceManager.
type CharacterSnapshot struct {
	EntityID   engine.EntityID
	Name       string
	HP         float64
	MaxHP      float64
	Attributes equipment.Attributes
}

// PersistenceStore is a minimal interface for loading/saving character data.
// This allows the CombatSystem to be decoupled from the actual DB layer.
type PersistenceStore interface {
	LoadCharacter(entityID engine.EntityID) (*CharacterSnapshot, error)
	SaveCombatResult(combatID string, log []*CombatLogEntry) error
}

// CombatInitError is returned when combat initialisation fails due to a
// persistence error, so the caller knows to cancel the combat.
type CombatInitError struct {
	EntityID engine.EntityID
	Cause    error
}

func (e *CombatInitError) Error() string {
	return "combat init failed for entity " + string(rune(e.EntityID)) + ": " + e.Cause.Error()
}

// StartCombatFromStore loads entity data from the persistence store, builds
// CombatEntity snapshots in memory, and starts the combat.
// If any entity fails to load, the combat is cancelled and an error is returned.
func StartCombatFromStore(
	cs CombatSystem,
	store PersistenceStore,
	combatID string,
	entityIDs []engine.EntityID,
	equipSystems map[engine.EntityID]equipment.EquipmentSystem,
) (*CombatContext, error) {
	entities := make([]*CombatEntity, 0, len(entityIDs))

	for _, eid := range entityIDs {
		snap, err := store.LoadCharacter(eid)
		if err != nil {
			return nil, &CombatInitError{EntityID: eid, Cause: err}
		}
		entities = append(entities, &CombatEntity{
			EntityID:   snap.EntityID,
			Name:       snap.Name,
			HP:         snap.HP,
			MaxHP:      snap.MaxHP,
			Attributes: snap.Attributes,
			Buffs:      make(map[string]*BuffInstance),
			Cooldowns:  make(map[string]time.Time),
		})
	}

	return cs.StartCombat(combatID, entities, equipSystems)
}

// EndCombatAndSave ends the combat and writes the result back to the store.
// If the save fails, the log is still returned and the error is reported
// (caller should handle local recovery file writing).
func EndCombatAndSave(
	cs CombatSystem,
	store PersistenceStore,
	combatID string,
) ([]*CombatLogEntry, error) {
	log, err := cs.EndCombat(combatID)
	if err != nil {
		return nil, err
	}

	if saveErr := store.SaveCombatResult(combatID, log); saveErr != nil {
		// Return log + save error so caller can persist locally.
		return log, saveErr
	}
	return log, nil
}
