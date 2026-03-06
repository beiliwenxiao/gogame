package scene

import (
	"testing"
	"time"

	"gogame/internal/ecs"
	"gogame/internal/engine"
)

// helper to create a basic SceneManager for tests.
func newTestManager() *sceneManager {
	sm := &sceneManager{
		scenes:            make(map[string]*Scene),
		entityScene:       make(map[engine.EntityID]string),
		stopCh:            make(chan struct{}),
		idleCheckInterval: 30 * time.Second,
	}
	return sm
}

func basicConfig(id string) SceneConfig {
	return SceneConfig{
		ID:          id,
		MapID:       "map-1",
		MaxEntities: 100,
		IdleTimeout: 5 * time.Minute,
	}
}

func TestCreateScene(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	s, err := sm.CreateScene(basicConfig("scene-1"))
	if err != nil {
		t.Fatalf("CreateScene failed: %v", err)
	}
	if s.ID != "scene-1" {
		t.Errorf("expected scene ID 'scene-1', got %q", s.ID)
	}
	if s.World == nil {
		t.Error("expected non-nil World")
	}
	if s.AOI == nil {
		t.Error("expected non-nil AOI")
	}
	if sm.ActiveSceneCount() != 1 {
		t.Errorf("expected 1 active scene, got %d", sm.ActiveSceneCount())
	}
}

func TestCreateScene_Duplicate(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	_, err := sm.CreateScene(basicConfig("scene-1"))
	if err != nil {
		t.Fatalf("first CreateScene failed: %v", err)
	}
	_, err = sm.CreateScene(basicConfig("scene-1"))
	if err != ErrSceneExists {
		t.Errorf("expected ErrSceneExists, got %v", err)
	}
}

func TestCreateScene_Defaults(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	s, err := sm.CreateScene(SceneConfig{ID: "default-scene"})
	if err != nil {
		t.Fatalf("CreateScene failed: %v", err)
	}
	if s.config.MaxEntities != DefaultMaxEntities {
		t.Errorf("expected default MaxEntities %d, got %d", DefaultMaxEntities, s.config.MaxEntities)
	}
	if s.config.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("expected default IdleTimeout %v, got %v", DefaultIdleTimeout, s.config.IdleTimeout)
	}
}

func TestGetScene(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-1"))

	s, ok := sm.GetScene("scene-1")
	if !ok || s == nil {
		t.Fatal("expected to find scene-1")
	}

	_, ok = sm.GetScene("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent scene")
	}
}

func TestUnloadScene(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-1"))

	err := sm.UnloadScene("scene-1")
	if err != nil {
		t.Fatalf("UnloadScene failed: %v", err)
	}
	if sm.ActiveSceneCount() != 0 {
		t.Errorf("expected 0 active scenes, got %d", sm.ActiveSceneCount())
	}

	err = sm.UnloadScene("scene-1")
	if err != ErrSceneNotFound {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestEnterScene(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-1"))

	err := sm.EnterScene(engine.EntityID(1), "scene-1")
	if err != nil {
		t.Fatalf("EnterScene failed: %v", err)
	}

	s, _ := sm.GetScene("scene-1")
	if !s.HasEntity(engine.EntityID(1)) {
		t.Error("expected entity 1 to be in scene-1")
	}
	if s.EntityCount() != 1 {
		t.Errorf("expected 1 entity, got %d", s.EntityCount())
	}
}

func TestEnterScene_NotFound(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	err := sm.EnterScene(engine.EntityID(1), "nonexistent")
	if err != ErrSceneNotFound {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestEnterScene_AdmissionDenied(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	cfg := basicConfig("scene-1")
	cfg.AdmissionCheck = func(id engine.EntityID) bool {
		return id != engine.EntityID(42) // deny entity 42
	}
	sm.CreateScene(cfg)

	err := sm.EnterScene(engine.EntityID(42), "scene-1")
	if err != ErrAdmissionDenied {
		t.Errorf("expected ErrAdmissionDenied, got %v", err)
	}

	// Entity 1 should be admitted.
	err = sm.EnterScene(engine.EntityID(1), "scene-1")
	if err != nil {
		t.Fatalf("expected entity 1 to be admitted, got %v", err)
	}
}

func TestEnterScene_AtCapacity(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	cfg := basicConfig("scene-1")
	cfg.MaxEntities = 2
	sm.CreateScene(cfg)

	sm.EnterScene(engine.EntityID(1), "scene-1")
	sm.EnterScene(engine.EntityID(2), "scene-1")

	err := sm.EnterScene(engine.EntityID(3), "scene-1")
	if err != ErrSceneAtCapacity {
		t.Errorf("expected ErrSceneAtCapacity, got %v", err)
	}
}

func TestTransferScene(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	// Create entity in scene-a's World with components.
	sceneA, _ := sm.GetScene("scene-a")
	entityID := sceneA.World.CreateEntity()
	sceneA.World.AddComponent(entityID, &ecs.PositionComponent{X: 10, Y: 20, Z: 30, MapID: "map-1", LayerIndex: 0})
	sceneA.World.AddComponent(entityID, &ecs.CombatAttributeComponent{HP: 100, MaxHP: 100, Attack: 50})

	// Register entity in scene-a.
	sm.EnterScene(entityID, "scene-a")

	// Transfer to scene-b.
	err := sm.TransferScene(entityID, "scene-a", "scene-b")
	if err != nil {
		t.Fatalf("TransferScene failed: %v", err)
	}

	// Verify entity is no longer in scene-a.
	if sceneA.HasEntity(entityID) {
		t.Error("entity should not be in scene-a after transfer")
	}

	// Verify entity is in scene-b.
	sceneB, _ := sm.GetScene("scene-b")
	if !sceneB.HasEntity(entityID) {
		t.Error("entity should be in scene-b after transfer")
	}

	// Verify components were preserved.
	posComp, ok := sceneB.World.GetComponent(entityID, ecs.CompPosition)
	if !ok {
		t.Fatal("expected PositionComponent in scene-b")
	}
	pos := posComp.(*ecs.PositionComponent)
	if pos.X != 10 || pos.Y != 20 || pos.Z != 30 {
		t.Errorf("position mismatch: got (%v, %v, %v)", pos.X, pos.Y, pos.Z)
	}

	combatComp, ok := sceneB.World.GetComponent(entityID, ecs.CompCombatAttribute)
	if !ok {
		t.Fatal("expected CombatAttributeComponent in scene-b")
	}
	combat := combatComp.(*ecs.CombatAttributeComponent)
	if combat.HP != 100 || combat.Attack != 50 {
		t.Errorf("combat attributes mismatch: HP=%v, Attack=%v", combat.HP, combat.Attack)
	}

	// Verify entity is NOT in scene-a's World.
	_, ok = sceneA.World.GetComponent(entityID, ecs.CompPosition)
	if ok {
		t.Error("entity components should not exist in scene-a's World after transfer")
	}
}

func TestTransferScene_NotInSource(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	err := sm.TransferScene(engine.EntityID(999), "scene-a", "scene-b")
	if err != ErrEntityNotInSource {
		t.Errorf("expected ErrEntityNotInSource, got %v", err)
	}
}

func TestTransferScene_AdmissionDenied(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	cfgB := basicConfig("scene-b")
	cfgB.AdmissionCheck = func(id engine.EntityID) bool { return false }
	sm.CreateScene(cfgB)

	sm.EnterScene(engine.EntityID(1), "scene-a")

	err := sm.TransferScene(engine.EntityID(1), "scene-a", "scene-b")
	if err != ErrAdmissionDenied {
		t.Errorf("expected ErrAdmissionDenied, got %v", err)
	}

	// Entity should still be in scene-a.
	sceneA, _ := sm.GetScene("scene-a")
	if !sceneA.HasEntity(engine.EntityID(1)) {
		t.Error("entity should still be in scene-a after failed transfer")
	}
}

func TestTransferScene_TargetAtCapacity(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	cfgB := basicConfig("scene-b")
	cfgB.MaxEntities = 0 // will be set to default, let's use 1
	cfgB.MaxEntities = 1
	sm.CreateScene(cfgB)

	sm.EnterScene(engine.EntityID(1), "scene-a")
	sm.EnterScene(engine.EntityID(2), "scene-b")

	err := sm.TransferScene(engine.EntityID(1), "scene-a", "scene-b")
	if err != ErrSceneAtCapacity {
		t.Errorf("expected ErrSceneAtCapacity, got %v", err)
	}
}

func TestUnloadScene_CleansEntityMapping(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-1"))
	sm.EnterScene(engine.EntityID(1), "scene-1")
	sm.EnterScene(engine.EntityID(2), "scene-1")

	sm.UnloadScene("scene-1")

	// Entity mappings should be cleaned up.
	if _, ok := sm.entityScene[engine.EntityID(1)]; ok {
		t.Error("entity 1 mapping should be removed after scene unload")
	}
	if _, ok := sm.entityScene[engine.EntityID(2)]; ok {
		t.Error("entity 2 mapping should be removed after scene unload")
	}
}

func TestIdleSceneAutoUnload(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	cfg := basicConfig("idle-scene")
	cfg.IdleTimeout = 50 * time.Millisecond
	sm.CreateScene(cfg)

	// Scene should exist initially.
	if sm.ActiveSceneCount() != 1 {
		t.Fatalf("expected 1 scene, got %d", sm.ActiveSceneCount())
	}

	// Wait for the idle timeout to pass.
	time.Sleep(100 * time.Millisecond)

	// Manually trigger idle check (don't rely on the goroutine timing).
	sm.checkIdleScenes()

	if sm.ActiveSceneCount() != 0 {
		t.Errorf("expected idle scene to be unloaded, got %d active scenes", sm.ActiveSceneCount())
	}
}

func TestIdleScene_NotUnloadedWithEntities(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	cfg := basicConfig("busy-scene")
	cfg.IdleTimeout = 50 * time.Millisecond
	sm.CreateScene(cfg)
	sm.EnterScene(engine.EntityID(1), "busy-scene")

	time.Sleep(100 * time.Millisecond)
	sm.checkIdleScenes()

	// Scene should NOT be unloaded because it has entities.
	if sm.ActiveSceneCount() != 1 {
		t.Errorf("expected scene with entities to remain, got %d active scenes", sm.ActiveSceneCount())
	}
}

func TestActiveSceneCount(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	if sm.ActiveSceneCount() != 0 {
		t.Errorf("expected 0, got %d", sm.ActiveSceneCount())
	}

	sm.CreateScene(basicConfig("s1"))
	sm.CreateScene(basicConfig("s2"))
	sm.CreateScene(basicConfig("s3"))

	if sm.ActiveSceneCount() != 3 {
		t.Errorf("expected 3, got %d", sm.ActiveSceneCount())
	}

	sm.UnloadScene("s2")
	if sm.ActiveSceneCount() != 2 {
		t.Errorf("expected 2, got %d", sm.ActiveSceneCount())
	}
}

func TestSceneBoundaryZones(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	zones := []*SceneBoundaryZone{
		{
			NeighborSceneID: "scene-b",
			LocalMin:        engine.Vector3{X: 90, Y: 0, Z: 0},
			LocalMax:        engine.Vector3{X: 100, Y: 100, Z: 100},
			RemoteMin:       engine.Vector3{X: 0, Y: 0, Z: 0},
			RemoteMax:       engine.Vector3{X: 10, Y: 100, Z: 100},
			PreloadDistance: 20,
		},
	}

	cfg := basicConfig("scene-a")
	cfg.BoundaryZones = zones
	s, err := sm.CreateScene(cfg)
	if err != nil {
		t.Fatalf("CreateScene failed: %v", err)
	}

	if len(s.BoundaryZones) != 1 {
		t.Fatalf("expected 1 boundary zone, got %d", len(s.BoundaryZones))
	}
	if s.BoundaryZones[0].NeighborSceneID != "scene-b" {
		t.Errorf("expected neighbor scene-b, got %q", s.BoundaryZones[0].NeighborSceneID)
	}
}

func TestTransferScene_MultipleComponents(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("src"))
	sm.CreateScene(basicConfig("dst"))

	srcScene, _ := sm.GetScene("src")
	eid := srcScene.World.CreateEntity()

	// Add multiple component types.
	srcScene.World.AddComponent(eid, &ecs.PositionComponent{X: 1, Y: 2, Z: 3, MapID: "m1", LayerIndex: 1})
	srcScene.World.AddComponent(eid, &ecs.MovementComponent{Speed: 5.0, Direction: engine.Vector3{X: 1}, Moving: true})
	srcScene.World.AddComponent(eid, &ecs.CombatAttributeComponent{HP: 80, MaxHP: 100, MP: 50, MaxMP: 50, Attack: 30, Defense: 20, CritRate: 0.1, CritDamage: 1.5, Speed: 10})
	srcScene.World.AddComponent(eid, &ecs.NetworkComponent{SessionID: "sess-123"})

	sm.EnterScene(eid, "src")

	err := sm.TransferScene(eid, "src", "dst")
	if err != nil {
		t.Fatalf("TransferScene failed: %v", err)
	}

	dstScene, _ := sm.GetScene("dst")

	// Verify all components transferred.
	pos, ok := dstScene.World.GetComponent(eid, ecs.CompPosition)
	if !ok {
		t.Fatal("missing PositionComponent")
	}
	p := pos.(*ecs.PositionComponent)
	if p.X != 1 || p.Y != 2 || p.Z != 3 || p.MapID != "m1" || p.LayerIndex != 1 {
		t.Errorf("PositionComponent mismatch: %+v", p)
	}

	mov, ok := dstScene.World.GetComponent(eid, ecs.CompMovement)
	if !ok {
		t.Fatal("missing MovementComponent")
	}
	m := mov.(*ecs.MovementComponent)
	if m.Speed != 5.0 || !m.Moving {
		t.Errorf("MovementComponent mismatch: %+v", m)
	}

	ca, ok := dstScene.World.GetComponent(eid, ecs.CompCombatAttribute)
	if !ok {
		t.Fatal("missing CombatAttributeComponent")
	}
	c := ca.(*ecs.CombatAttributeComponent)
	if c.HP != 80 || c.Attack != 30 || c.CritRate != 0.1 {
		t.Errorf("CombatAttributeComponent mismatch: %+v", c)
	}

	net, ok := dstScene.World.GetComponent(eid, ecs.CompNetwork)
	if !ok {
		t.Fatal("missing NetworkComponent")
	}
	n := net.(*ecs.NetworkComponent)
	if n.SessionID != "sess-123" {
		t.Errorf("NetworkComponent mismatch: SessionID=%q", n.SessionID)
	}
}


// ============================================================
// Task 9.3: Seamless Transition Tests
// ============================================================

func TestConfigureBoundaryZone(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	zone := &SceneBoundaryZone{
		NeighborSceneID: "scene-b",
		LocalMin:        engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:        engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin:       engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax:       engine.Vector3{X: 10, Y: 100, Z: 100},
		PreloadDistance: 20,
	}

	err := sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)
	if err != nil {
		t.Fatalf("ConfigureBoundaryZone failed: %v", err)
	}

	// Verify zone is stored.
	s, _ := sm.GetScene("scene-a")
	if len(s.boundaryMap) != 1 {
		t.Errorf("expected 1 boundary zone, got %d", len(s.boundaryMap))
	}
	if _, ok := s.boundaryMap["scene-b"]; !ok {
		t.Error("expected boundary zone for scene-b")
	}
}

func TestConfigureBoundaryZone_SceneNotFound(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	zone := &SceneBoundaryZone{}
	err := sm.ConfigureBoundaryZone("nonexistent", "scene-b", zone)
	if err != ErrSceneNotFound {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestGetBoundaryZone(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	zone := &SceneBoundaryZone{
		LocalMin:        engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:        engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin:       engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax:       engine.Vector3{X: 10, Y: 100, Z: 100},
		PreloadDistance: 15,
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	bz, ok := sm.GetBoundaryZone("scene-a", "scene-b")
	if !ok {
		t.Fatal("expected to find boundary zone")
	}
	if bz.PreloadDistance != 15 {
		t.Errorf("expected PreloadDistance 15, got %v", bz.PreloadDistance)
	}

	// Non-existent neighbor.
	_, ok = sm.GetBoundaryZone("scene-a", "scene-c")
	if ok {
		t.Error("expected no boundary zone for scene-c")
	}

	// Non-existent scene.
	_, ok = sm.GetBoundaryZone("nonexistent", "scene-b")
	if ok {
		t.Error("expected no boundary zone for nonexistent scene")
	}
}

func TestIsInBoundaryZone(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	zone := &SceneBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	sceneA, _ := sm.GetScene("scene-a")
	eid := sceneA.World.CreateEntity()
	sceneA.World.AddComponent(eid, &ecs.PositionComponent{X: 95, Y: 50, Z: 50})
	sm.EnterScene(eid, "scene-a")

	// Entity at (95, 50, 50) should be in the boundary zone [90-100, 0-100, 0-100].
	inZone, neighborID := sm.IsInBoundaryZone(eid, "scene-a")
	if !inZone {
		t.Error("expected entity to be in boundary zone")
	}
	if neighborID != "scene-b" {
		t.Errorf("expected neighbor scene-b, got %q", neighborID)
	}

	// Move entity outside boundary zone.
	eid2 := sceneA.World.CreateEntity()
	sceneA.World.AddComponent(eid2, &ecs.PositionComponent{X: 50, Y: 50, Z: 50})
	sm.EnterScene(eid2, "scene-a")

	inZone, neighborID = sm.IsInBoundaryZone(eid2, "scene-a")
	if inZone {
		t.Error("expected entity NOT to be in boundary zone")
	}
	if neighborID != "" {
		t.Errorf("expected empty neighbor ID, got %q", neighborID)
	}
}

func TestIsInBoundaryZone_EntityNotInScene(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))

	inZone, _ := sm.IsInBoundaryZone(engine.EntityID(999), "scene-a")
	if inZone {
		t.Error("expected false for entity not in scene")
	}
}

func TestIsInBoundaryZone_SceneNotFound(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	inZone, _ := sm.IsInBoundaryZone(engine.EntityID(1), "nonexistent")
	if inZone {
		t.Error("expected false for nonexistent scene")
	}
}

func TestTriggerBoundaryPreload(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))

	zone := &SceneBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	sceneA, _ := sm.GetScene("scene-a")
	eid := sceneA.World.CreateEntity()
	sm.EnterScene(eid, "scene-a")

	// scene-b doesn't exist yet — TriggerBoundaryPreload should create it.
	err := sm.TriggerBoundaryPreload(eid, "scene-a", "scene-b")
	if err != nil {
		t.Fatalf("TriggerBoundaryPreload failed: %v", err)
	}

	// Verify scene-b was created.
	sceneB, ok := sm.GetScene("scene-b")
	if !ok {
		t.Fatal("expected scene-b to be created by preload")
	}
	if sceneB.World == nil {
		t.Error("expected scene-b to have a World")
	}

	// Verify preloaded flag.
	if !sceneA.preloaded["scene-b"] {
		t.Error("expected scene-a to mark scene-b as preloaded")
	}
}

func TestTriggerBoundaryPreload_ExistingNeighbor(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	zone := &SceneBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	sceneA, _ := sm.GetScene("scene-a")
	eid := sceneA.World.CreateEntity()
	sm.EnterScene(eid, "scene-a")

	// scene-b already exists — should just register AOI.
	err := sm.TriggerBoundaryPreload(eid, "scene-a", "scene-b")
	if err != nil {
		t.Fatalf("TriggerBoundaryPreload failed: %v", err)
	}

	// Should still have 2 scenes.
	if sm.ActiveSceneCount() != 2 {
		t.Errorf("expected 2 scenes, got %d", sm.ActiveSceneCount())
	}
}

func TestTriggerBoundaryPreload_NoBoundaryZone(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))

	err := sm.TriggerBoundaryPreload(engine.EntityID(1), "scene-a", "scene-c")
	if err != ErrBoundaryZoneNotFound {
		t.Errorf("expected ErrBoundaryZoneNotFound, got %v", err)
	}
}

func TestTriggerBoundaryPreload_SceneNotFound(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	err := sm.TriggerBoundaryPreload(engine.EntityID(1), "nonexistent", "scene-b")
	if err != ErrSceneNotFound {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestSeamlessTransfer(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	// Configure boundary zone.
	zone := &SceneBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	// Create entity with components in scene-a.
	sceneA, _ := sm.GetScene("scene-a")
	eid := sceneA.World.CreateEntity()
	sceneA.World.AddComponent(eid, &ecs.PositionComponent{X: 95, Y: 50, Z: 50, MapID: "map-1"})
	sceneA.World.AddComponent(eid, &ecs.CombatAttributeComponent{HP: 200, MaxHP: 200, Attack: 75})
	sm.EnterScene(eid, "scene-a")

	// Seamless transfer.
	err := sm.SeamlessTransfer(eid, "scene-a", "scene-b")
	if err != nil {
		t.Fatalf("SeamlessTransfer failed: %v", err)
	}

	// Entity should be in scene-b, not scene-a.
	if sceneA.HasEntity(eid) {
		t.Error("entity should not be in scene-a after seamless transfer")
	}
	sceneB, _ := sm.GetScene("scene-b")
	if !sceneB.HasEntity(eid) {
		t.Error("entity should be in scene-b after seamless transfer")
	}

	// Components should be preserved.
	posComp, ok := sceneB.World.GetComponent(eid, ecs.CompPosition)
	if !ok {
		t.Fatal("expected PositionComponent in scene-b")
	}
	pos := posComp.(*ecs.PositionComponent)
	if pos.X != 95 || pos.Y != 50 || pos.Z != 50 {
		t.Errorf("position mismatch: (%v, %v, %v)", pos.X, pos.Y, pos.Z)
	}

	combatComp, ok := sceneB.World.GetComponent(eid, ecs.CompCombatAttribute)
	if !ok {
		t.Fatal("expected CombatAttributeComponent in scene-b")
	}
	combat := combatComp.(*ecs.CombatAttributeComponent)
	if combat.HP != 200 || combat.Attack != 75 {
		t.Errorf("combat mismatch: HP=%v, Attack=%v", combat.HP, combat.Attack)
	}
}

func TestSeamlessTransfer_NoBoundaryZone(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	sm.EnterScene(engine.EntityID(1), "scene-a")

	// No boundary zone configured — should fail.
	err := sm.SeamlessTransfer(engine.EntityID(1), "scene-a", "scene-b")
	if err != ErrBoundaryZoneNotFound {
		t.Errorf("expected ErrBoundaryZoneNotFound, got %v", err)
	}
}

func TestSeamlessTransfer_SceneNotFound(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	err := sm.SeamlessTransfer(engine.EntityID(1), "nonexistent", "scene-b")
	if err != ErrSceneNotFound {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestSeamlessTransfer_TargetNotFound(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))

	zone := &SceneBoundaryZone{
		LocalMin: engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax: engine.Vector3{X: 100, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	sm.EnterScene(engine.EntityID(1), "scene-a")

	err := sm.SeamlessTransfer(engine.EntityID(1), "scene-a", "scene-b")
	if err != ErrSceneNotFound {
		t.Errorf("expected ErrSceneNotFound, got %v", err)
	}
}

func TestCrossBoundaryAOIVisibility(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	// Configure boundary zones on both sides.
	zoneA := &SceneBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zoneA)

	// Trigger preload to register cross-boundary AOI in both directions.
	sceneA, _ := sm.GetScene("scene-a")
	eid1 := sceneA.World.CreateEntity()
	sm.EnterScene(eid1, "scene-a")
	sm.TriggerBoundaryPreload(eid1, "scene-a", "scene-b")

	// Add entity in scene-a's boundary zone.
	sceneA.AOI.Add(eid1, engine.Vector3{X: 95, Y: 50, Z: 50}, 20)

	// Add entity in scene-b's boundary zone.
	sceneB, _ := sm.GetScene("scene-b")
	eid2 := sceneB.World.CreateEntity()
	sm.EnterScene(eid2, "scene-b")
	sceneB.AOI.Add(eid2, engine.Vector3{X: 5, Y: 50, Z: 50}, 20)

	// Entity in scene-a's boundary zone should see cross-boundary entities.
	crossEntities := sceneA.AOI.GetCrossBoundaryVisibleEntities(eid1)
	// Should include eid2 from scene-b.
	found := false
	for _, ce := range crossEntities {
		if ce.EntityID == eid2 && ce.SourceID == "scene-b" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected entity in scene-a to see cross-boundary entity from scene-b")
	}
}

func TestIsInBoundaryZone_NoPositionComponent(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))

	zone := &SceneBoundaryZone{
		LocalMin: engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax: engine.Vector3{X: 100, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	// Entity without PositionComponent.
	sceneA, _ := sm.GetScene("scene-a")
	eid := sceneA.World.CreateEntity()
	sm.EnterScene(eid, "scene-a")

	inZone, _ := sm.IsInBoundaryZone(eid, "scene-a")
	if inZone {
		t.Error("expected false for entity without PositionComponent")
	}
}

func TestSeamlessTransfer_PreservesAllComponents(t *testing.T) {
	sm := newTestManager()
	defer sm.Stop()

	sm.CreateScene(basicConfig("scene-a"))
	sm.CreateScene(basicConfig("scene-b"))

	zone := &SceneBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	}
	sm.ConfigureBoundaryZone("scene-a", "scene-b", zone)

	sceneA, _ := sm.GetScene("scene-a")
	eid := sceneA.World.CreateEntity()
	sceneA.World.AddComponent(eid, &ecs.PositionComponent{X: 95, Y: 50, Z: 50, MapID: "m1", LayerIndex: 2})
	sceneA.World.AddComponent(eid, &ecs.MovementComponent{Speed: 8.0, Direction: engine.Vector3{X: 1, Y: 0, Z: 0}, Moving: true})
	sceneA.World.AddComponent(eid, &ecs.CombatAttributeComponent{HP: 150, MaxHP: 200, MP: 80, MaxMP: 100, Attack: 60, Defense: 30, CritRate: 0.15, CritDamage: 2.0, Speed: 12})
	sceneA.World.AddComponent(eid, &ecs.NetworkComponent{SessionID: "sess-abc"})
	sm.EnterScene(eid, "scene-a")

	err := sm.SeamlessTransfer(eid, "scene-a", "scene-b")
	if err != nil {
		t.Fatalf("SeamlessTransfer failed: %v", err)
	}

	sceneB, _ := sm.GetScene("scene-b")

	// Verify all components.
	pos, ok := sceneB.World.GetComponent(eid, ecs.CompPosition)
	if !ok {
		t.Fatal("missing PositionComponent")
	}
	p := pos.(*ecs.PositionComponent)
	if p.X != 95 || p.Y != 50 || p.Z != 50 || p.MapID != "m1" || p.LayerIndex != 2 {
		t.Errorf("PositionComponent mismatch: %+v", p)
	}

	mov, ok := sceneB.World.GetComponent(eid, ecs.CompMovement)
	if !ok {
		t.Fatal("missing MovementComponent")
	}
	m := mov.(*ecs.MovementComponent)
	if m.Speed != 8.0 || !m.Moving {
		t.Errorf("MovementComponent mismatch: %+v", m)
	}

	ca, ok := sceneB.World.GetComponent(eid, ecs.CompCombatAttribute)
	if !ok {
		t.Fatal("missing CombatAttributeComponent")
	}
	c := ca.(*ecs.CombatAttributeComponent)
	if c.HP != 150 || c.Attack != 60 || c.CritRate != 0.15 {
		t.Errorf("CombatAttributeComponent mismatch: %+v", c)
	}

	net, ok := sceneB.World.GetComponent(eid, ecs.CompNetwork)
	if !ok {
		t.Fatal("missing NetworkComponent")
	}
	n := net.(*ecs.NetworkComponent)
	if n.SessionID != "sess-abc" {
		t.Errorf("NetworkComponent mismatch: SessionID=%q", n.SessionID)
	}
}
