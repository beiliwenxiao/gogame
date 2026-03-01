package ecs

import (
	"testing"

	"gfgame/internal/engine"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// mockSystem is a minimal System implementation for testing.
type mockSystem struct {
	name       string
	priority   int
	required   []engine.ComponentType
	updateFn   func(tick uint64, entities []engine.EntityID, world *World)
	callCount  int
	lastTick   uint64
	lastEntIDs []engine.EntityID
}

func (s *mockSystem) Name() string                          { return s.name }
func (s *mockSystem) Priority() int                         { return s.priority }
func (s *mockSystem) RequiredComponents() []engine.ComponentType { return s.required }
func (s *mockSystem) Update(tick uint64, entities []engine.EntityID, world *World) {
	s.callCount++
	s.lastTick = tick
	s.lastEntIDs = entities
	if s.updateFn != nil {
		s.updateFn(tick, entities, world)
	}
}

// ---------------------------------------------------------------------------
// Entity creation & ID uniqueness
// ---------------------------------------------------------------------------

func TestCreateEntity_UniqueIDs(t *testing.T) {
	w := NewWorld()
	seen := make(map[engine.EntityID]bool)
	const n = 1000
	for i := 0; i < n; i++ {
		id := w.CreateEntity()
		if seen[id] {
			t.Fatalf("duplicate EntityID %d at iteration %d", id, i)
		}
		seen[id] = true
	}
	if w.Entities() != n {
		t.Fatalf("expected %d entities, got %d", n, w.Entities())
	}
}

func TestCreateEntity_IDsAreNonZero(t *testing.T) {
	w := NewWorld()
	id := w.CreateEntity()
	if id == 0 {
		t.Fatal("EntityID should not be zero")
	}
}

// ---------------------------------------------------------------------------
// Component add / remove / get
// ---------------------------------------------------------------------------

func TestAddAndGetComponent(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()

	pos := &PositionComponent{X: 1, Y: 2, Z: 3, MapID: "map1", LayerIndex: 0}
	w.AddComponent(e, pos)

	got, ok := w.GetComponent(e, CompPosition)
	if !ok {
		t.Fatal("expected to find PositionComponent")
	}
	p := got.(*PositionComponent)
	if p.X != 1 || p.Y != 2 || p.Z != 3 {
		t.Fatalf("unexpected position: %+v", p)
	}
}

func TestGetComponent_NotFound(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	_, ok := w.GetComponent(e, CompPosition)
	if ok {
		t.Fatal("expected component not found")
	}
}

func TestGetComponent_EntityNotFound(t *testing.T) {
	w := NewWorld()
	_, ok := w.GetComponent(999, CompPosition)
	if ok {
		t.Fatal("expected false for non-existent entity")
	}
}

func TestRemoveComponent(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	w.AddComponent(e, &PositionComponent{X: 1})
	w.RemoveComponent(e, CompPosition)

	_, ok := w.GetComponent(e, CompPosition)
	if ok {
		t.Fatal("component should have been removed")
	}
}

func TestAddComponent_OverwritesSameType(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	w.AddComponent(e, &PositionComponent{X: 1})
	w.AddComponent(e, &PositionComponent{X: 99})

	got, _ := w.GetComponent(e, CompPosition)
	if got.(*PositionComponent).X != 99 {
		t.Fatal("expected overwritten component value")
	}
}

func TestAddComponent_NonExistentEntity(t *testing.T) {
	w := NewWorld()
	// Should not panic.
	w.AddComponent(999, &PositionComponent{X: 1})
}

func TestRemoveComponent_NonExistentEntity(t *testing.T) {
	w := NewWorld()
	// Should not panic.
	w.RemoveComponent(999, CompPosition)
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

func TestQuery_SingleComponentType(t *testing.T) {
	w := NewWorld()
	e1 := w.CreateEntity()
	e2 := w.CreateEntity()
	e3 := w.CreateEntity()

	w.AddComponent(e1, &PositionComponent{})
	w.AddComponent(e2, &PositionComponent{})
	w.AddComponent(e3, &MovementComponent{})

	result := w.Query(CompPosition)
	if len(result) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(result))
	}
	ids := toSet(result)
	if !ids[e1] || !ids[e2] {
		t.Fatal("expected e1 and e2 in query result")
	}
}

func TestQuery_MultipleComponentTypes(t *testing.T) {
	w := NewWorld()
	e1 := w.CreateEntity()
	e2 := w.CreateEntity()

	w.AddComponent(e1, &PositionComponent{})
	w.AddComponent(e1, &MovementComponent{})
	w.AddComponent(e2, &PositionComponent{})

	result := w.Query(CompPosition, CompMovement)
	if len(result) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result))
	}
	if result[0] != e1 {
		t.Fatalf("expected entity %d, got %d", e1, result[0])
	}
}

func TestQuery_NoMatch(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	w.AddComponent(e, &PositionComponent{})

	result := w.Query(CompMovement)
	if len(result) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(result))
	}
}

func TestQuery_EmptyWorld(t *testing.T) {
	w := NewWorld()
	result := w.Query(CompPosition)
	if len(result) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(result))
	}
}

func TestQuery_NoComponentTypes(t *testing.T) {
	w := NewWorld()
	w.CreateEntity()
	w.CreateEntity()
	// Query with no component types should match all entities.
	result := w.Query()
	if len(result) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(result))
	}
}

func toSet(ids []engine.EntityID) map[engine.EntityID]bool {
	m := make(map[engine.EntityID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// ---------------------------------------------------------------------------
// DestroyEntity
// ---------------------------------------------------------------------------

func TestDestroyEntity_RemovesAllComponents(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	w.AddComponent(e, &PositionComponent{X: 1})
	w.AddComponent(e, &MovementComponent{Speed: 5})
	w.AddComponent(e, &CombatAttributeComponent{HP: 100})

	w.DestroyEntity(e)

	for _, ct := range []engine.ComponentType{CompPosition, CompMovement, CompCombatAttribute} {
		if _, ok := w.GetComponent(e, ct); ok {
			t.Fatalf("component type %d should not exist after destroy", ct)
		}
	}
}

func TestDestroyEntity_RemovedFromQueries(t *testing.T) {
	w := NewWorld()
	e1 := w.CreateEntity()
	e2 := w.CreateEntity()
	w.AddComponent(e1, &PositionComponent{})
	w.AddComponent(e2, &PositionComponent{})

	w.DestroyEntity(e1)

	result := w.Query(CompPosition)
	if len(result) != 1 || result[0] != e2 {
		t.Fatalf("destroyed entity should not appear in query, got %v", result)
	}
}

func TestDestroyEntity_RemovedFromSystemUpdate(t *testing.T) {
	w := NewWorld()
	e1 := w.CreateEntity()
	e2 := w.CreateEntity()
	w.AddComponent(e1, &PositionComponent{})
	w.AddComponent(e2, &PositionComponent{})

	sys := &mockSystem{
		name:     "test",
		priority: 1,
		required: []engine.ComponentType{CompPosition},
	}
	w.RegisterSystem(sys)

	w.DestroyEntity(e1)
	w.Update(1)

	if len(sys.lastEntIDs) != 1 || sys.lastEntIDs[0] != e2 {
		t.Fatalf("destroyed entity should not be passed to system, got %v", sys.lastEntIDs)
	}
}

func TestDestroyEntity_ReducesEntityCount(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	w.CreateEntity()
	w.DestroyEntity(e)
	if w.Entities() != 1 {
		t.Fatalf("expected 1 entity, got %d", w.Entities())
	}
}

func TestDestroyEntity_NonExistent(t *testing.T) {
	w := NewWorld()
	// Should not panic.
	w.DestroyEntity(999)
}

// ---------------------------------------------------------------------------
// System registration & priority ordering
// ---------------------------------------------------------------------------

func TestRegisterSystem_PriorityOrder(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	w.AddComponent(e, &PositionComponent{})

	var order []string
	s1 := &mockSystem{
		name:     "low-priority",
		priority: 10,
		required: []engine.ComponentType{CompPosition},
		updateFn: func(_ uint64, _ []engine.EntityID, _ *World) { order = append(order, "low") },
	}
	s2 := &mockSystem{
		name:     "high-priority",
		priority: 1,
		required: []engine.ComponentType{CompPosition},
		updateFn: func(_ uint64, _ []engine.EntityID, _ *World) { order = append(order, "high") },
	}
	// Register low-priority first, then high-priority.
	w.RegisterSystem(s1)
	w.RegisterSystem(s2)

	w.Update(1)

	if len(order) != 2 || order[0] != "high" || order[1] != "low" {
		t.Fatalf("expected [high, low], got %v", order)
	}
}

func TestSystem_ReceivesCorrectTick(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()
	w.AddComponent(e, &PositionComponent{})

	sys := &mockSystem{
		name:     "tick-check",
		priority: 1,
		required: []engine.ComponentType{CompPosition},
	}
	w.RegisterSystem(sys)

	w.Update(42)
	if sys.lastTick != 42 {
		t.Fatalf("expected tick 42, got %d", sys.lastTick)
	}
}

func TestSystem_OnlyReceivesMatchingEntities(t *testing.T) {
	w := NewWorld()
	e1 := w.CreateEntity()
	e2 := w.CreateEntity()
	w.AddComponent(e1, &PositionComponent{})
	w.AddComponent(e1, &MovementComponent{})
	w.AddComponent(e2, &PositionComponent{})

	sys := &mockSystem{
		name:     "movement-sys",
		priority: 1,
		required: []engine.ComponentType{CompPosition, CompMovement},
	}
	w.RegisterSystem(sys)
	w.Update(1)

	if len(sys.lastEntIDs) != 1 || sys.lastEntIDs[0] != e1 {
		t.Fatalf("expected only e1, got %v", sys.lastEntIDs)
	}
}

// ---------------------------------------------------------------------------
// Core component type values
// ---------------------------------------------------------------------------

func TestComponentTypeValues(t *testing.T) {
	tests := []struct {
		comp     Component
		expected engine.ComponentType
	}{
		{&PositionComponent{}, CompPosition},
		{&MovementComponent{}, CompMovement},
		{&CombatAttributeComponent{}, CompCombatAttribute},
		{&EquipmentComponent{Slots: make(map[engine.EquipmentSlotType]*EquipmentItem)}, CompEquipment},
		{&SkillComponent{Skills: make(map[uint32]*SkillInstance)}, CompSkill},
		{&BuffComponent{}, CompBuff},
		{&NetworkComponent{}, CompNetwork},
		{&AOIComponent{VisibleEntities: make(map[engine.EntityID]bool)}, CompAOI},
	}
	for _, tt := range tests {
		if tt.comp.Type() != tt.expected {
			t.Errorf("component %T: expected type %d, got %d", tt.comp, tt.expected, tt.comp.Type())
		}
	}
}

// ---------------------------------------------------------------------------
// Multiple component types on one entity
// ---------------------------------------------------------------------------

func TestEntity_MultipleComponents(t *testing.T) {
	w := NewWorld()
	e := w.CreateEntity()

	w.AddComponent(e, &PositionComponent{X: 10, Y: 20, Z: 30})
	w.AddComponent(e, &MovementComponent{Speed: 5, Moving: true})
	w.AddComponent(e, &CombatAttributeComponent{HP: 100, MaxHP: 100, Attack: 50})
	w.AddComponent(e, &NetworkComponent{SessionID: "sess-1"})

	// Verify each component is independently retrievable.
	pos, ok := w.GetComponent(e, CompPosition)
	if !ok || pos.(*PositionComponent).X != 10 {
		t.Fatal("position component mismatch")
	}
	mov, ok := w.GetComponent(e, CompMovement)
	if !ok || !mov.(*MovementComponent).Moving {
		t.Fatal("movement component mismatch")
	}
	combat, ok := w.GetComponent(e, CompCombatAttribute)
	if !ok || combat.(*CombatAttributeComponent).Attack != 50 {
		t.Fatal("combat attribute component mismatch")
	}
	net, ok := w.GetComponent(e, CompNetwork)
	if !ok || net.(*NetworkComponent).SessionID != "sess-1" {
		t.Fatal("network component mismatch")
	}
}

// ---------------------------------------------------------------------------
// World.Update with no systems / no entities
// ---------------------------------------------------------------------------

func TestUpdate_NoSystems(t *testing.T) {
	w := NewWorld()
	w.CreateEntity()
	// Should not panic.
	w.Update(1)
}

func TestUpdate_NoEntities(t *testing.T) {
	w := NewWorld()
	sys := &mockSystem{
		name:     "empty",
		priority: 1,
		required: []engine.ComponentType{CompPosition},
	}
	w.RegisterSystem(sys)
	w.Update(1)
	if sys.callCount != 1 {
		t.Fatal("system should still be called even with no matching entities")
	}
	if len(sys.lastEntIDs) != 0 {
		t.Fatal("expected empty entity list")
	}
}
