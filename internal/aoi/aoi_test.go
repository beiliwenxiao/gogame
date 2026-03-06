package aoi

import (
	"sync"
	"testing"
	"time"

	"gogame/internal/engine"
)

// newTestAOI creates an AOIManager backed by a grid spatial index for testing.
func newTestAOI() AOIManager {
	return NewAOIManager(NewGridSpatialIndex(GridConfig{CellSize: 50}))
}

// ---------- Add & GetVisibleEntities ----------

func TestAdd_SingleEntity(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 100)

	vis := mgr.GetVisibleEntities(1)
	if len(vis) != 0 {
		t.Fatalf("single entity should see nothing, got %v", vis)
	}
}

func TestAdd_TwoEntitiesInRange(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 100)
	mgr.Add(2, v3(10, 0, 10), 100)

	vis1 := mgr.GetVisibleEntities(1)
	vis2 := mgr.GetVisibleEntities(2)

	if !containsID(vis1, 2) {
		t.Fatalf("entity 1 should see entity 2, got %v", vis1)
	}
	if !containsID(vis2, 1) {
		t.Fatalf("entity 2 should see entity 1, got %v", vis2)
	}
}

func TestAdd_TwoEntitiesOutOfRange(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 10)
	mgr.Add(2, v3(100, 0, 100), 10)

	vis1 := mgr.GetVisibleEntities(1)
	vis2 := mgr.GetVisibleEntities(2)

	if containsID(vis1, 2) {
		t.Fatalf("entity 1 should NOT see entity 2, got %v", vis1)
	}
	if containsID(vis2, 1) {
		t.Fatalf("entity 2 should NOT see entity 1, got %v", vis2)
	}
}

func TestAdd_AsymmetricRadius(t *testing.T) {
	mgr := newTestAOI()
	// Entity 1 has large radius, entity 2 has small radius.
	// Distance between them is ~14.14.
	mgr.Add(1, v3(0, 0, 0), 100)
	mgr.Add(2, v3(10, 0, 10), 5) // radius 5, can't see entity 1 at distance ~14

	vis1 := mgr.GetVisibleEntities(1)
	vis2 := mgr.GetVisibleEntities(2)

	if !containsID(vis1, 2) {
		t.Fatalf("entity 1 (radius 100) should see entity 2, got %v", vis1)
	}
	if containsID(vis2, 1) {
		t.Fatalf("entity 2 (radius 5) should NOT see entity 1 at distance ~14, got %v", vis2)
	}
}

// ---------- Enter/Leave View Events ----------

func TestEnterViewEvent_OnAdd(t *testing.T) {
	mgr := newTestAOI()

	var mu sync.Mutex
	var events []eventPair
	mgr.OnEnterView(func(watcher, target engine.EntityID) {
		mu.Lock()
		events = append(events, eventPair{watcher, target})
		mu.Unlock()
	})

	mgr.Add(1, v3(0, 0, 0), 100)
	mgr.Add(2, v3(10, 0, 10), 100)

	mu.Lock()
	defer mu.Unlock()

	// Expect two enter events: (2 sees 1) and (1 sees 2).
	if len(events) < 2 {
		t.Fatalf("expected at least 2 enter events, got %d: %v", len(events), events)
	}

	found12 := false
	found21 := false
	for _, e := range events {
		if e.watcher == 1 && e.target == 2 {
			found12 = true
		}
		if e.watcher == 2 && e.target == 1 {
			found21 = true
		}
	}
	if !found12 {
		t.Fatal("missing enter event: watcher=1, target=2")
	}
	if !found21 {
		t.Fatal("missing enter event: watcher=2, target=1")
	}
}

func TestLeaveViewEvent_OnRemove(t *testing.T) {
	mgr := newTestAOI()

	mgr.Add(1, v3(0, 0, 0), 100)
	mgr.Add(2, v3(10, 0, 10), 100)

	var mu sync.Mutex
	var events []eventPair
	mgr.OnLeaveView(func(watcher, target engine.EntityID) {
		mu.Lock()
		events = append(events, eventPair{watcher, target})
		mu.Unlock()
	})

	mgr.Remove(2)

	mu.Lock()
	defer mu.Unlock()

	// Expect leave events: (1 leaves 2's view) and (2 leaves 1's view).
	found12 := false
	found21 := false
	for _, e := range events {
		if e.watcher == 1 && e.target == 2 {
			found12 = true
		}
		if e.watcher == 2 && e.target == 1 {
			found21 = true
		}
	}
	if !found12 {
		t.Fatal("missing leave event: watcher=1, target=2")
	}
	if !found21 {
		t.Fatal("missing leave event: watcher=2, target=1")
	}
}

func TestEnterLeaveEvents_OnMove(t *testing.T) {
	mgr := newTestAOI()

	mgr.Add(1, v3(0, 0, 0), 20)
	mgr.Add(2, v3(100, 0, 100), 20)

	// Initially out of range.
	vis := mgr.GetVisibleEntities(1)
	if containsID(vis, 2) {
		t.Fatal("entities should not be visible initially")
	}

	var enterEvents, leaveEvents []eventPair
	var mu sync.Mutex
	mgr.OnEnterView(func(w, tgt engine.EntityID) {
		mu.Lock()
		enterEvents = append(enterEvents, eventPair{w, tgt})
		mu.Unlock()
	})
	mgr.OnLeaveView(func(w, tgt engine.EntityID) {
		mu.Lock()
		leaveEvents = append(leaveEvents, eventPair{w, tgt})
		mu.Unlock()
	})

	// Move entity 1 close to entity 2.
	mgr.UpdatePosition(1, v3(95, 0, 95))

	mu.Lock()
	if len(enterEvents) < 2 {
		t.Fatalf("expected at least 2 enter events after move, got %d", len(enterEvents))
	}
	mu.Unlock()

	// Now move entity 1 far away.
	mgr.UpdatePosition(1, v3(500, 0, 500))

	mu.Lock()
	if len(leaveEvents) < 2 {
		t.Fatalf("expected at least 2 leave events after move away, got %d", len(leaveEvents))
	}
	mu.Unlock()
}

// ---------- Remove ----------

func TestRemove_CleansUpVisibility(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 100)
	mgr.Add(2, v3(10, 0, 10), 100)
	mgr.Add(3, v3(5, 0, 5), 100)

	mgr.Remove(2)

	vis1 := mgr.GetVisibleEntities(1)
	vis3 := mgr.GetVisibleEntities(3)

	if containsID(vis1, 2) {
		t.Fatal("entity 1 should not see removed entity 2")
	}
	if containsID(vis3, 2) {
		t.Fatal("entity 3 should not see removed entity 2")
	}
}

func TestAOIRemove_NonExistent(t *testing.T) {
	mgr := newTestAOI()
	// Should not panic.
	mgr.Remove(999)
}

// ---------- UpdatePosition ----------

func TestUpdatePosition_EntersRange(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 20)
	mgr.Add(2, v3(200, 0, 200), 20)

	// Move entity 2 close to entity 1.
	mgr.UpdatePosition(2, v3(5, 0, 5))

	vis1 := mgr.GetVisibleEntities(1)
	vis2 := mgr.GetVisibleEntities(2)

	if !containsID(vis1, 2) {
		t.Fatalf("entity 1 should see entity 2 after move, got %v", vis1)
	}
	if !containsID(vis2, 1) {
		t.Fatalf("entity 2 should see entity 1 after move, got %v", vis2)
	}
}

func TestUpdatePosition_LeavesRange(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 20)
	mgr.Add(2, v3(5, 0, 5), 20)

	// Move entity 2 far away.
	mgr.UpdatePosition(2, v3(500, 0, 500))

	vis1 := mgr.GetVisibleEntities(1)
	vis2 := mgr.GetVisibleEntities(2)

	if containsID(vis1, 2) {
		t.Fatal("entity 1 should NOT see entity 2 after it moved away")
	}
	if containsID(vis2, 1) {
		t.Fatal("entity 2 should NOT see entity 1 after moving away")
	}
}

func TestUpdatePosition_NonExistent(t *testing.T) {
	mgr := newTestAOI()
	// Should not panic.
	mgr.UpdatePosition(999, v3(0, 0, 0))
}

// ---------- SetAOIRadius ----------

func TestSetAOIRadius_ExpandsVisibility(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 5)
	mgr.Add(2, v3(20, 0, 0), 5)

	// Initially out of range.
	vis := mgr.GetVisibleEntities(1)
	if containsID(vis, 2) {
		t.Fatal("entity 1 should not see entity 2 with radius 5")
	}

	// Expand radius.
	mgr.SetAOIRadius(1, 50)

	vis = mgr.GetVisibleEntities(1)
	if !containsID(vis, 2) {
		t.Fatalf("entity 1 should see entity 2 after radius expansion, got %v", vis)
	}
}

func TestSetAOIRadius_ShrinksVisibility(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 100)
	mgr.Add(2, v3(20, 0, 0), 100)

	vis := mgr.GetVisibleEntities(1)
	if !containsID(vis, 2) {
		t.Fatal("entity 1 should see entity 2 with radius 100")
	}

	// Shrink radius.
	mgr.SetAOIRadius(1, 5)

	vis = mgr.GetVisibleEntities(1)
	if containsID(vis, 2) {
		t.Fatal("entity 1 should NOT see entity 2 after radius shrink to 5")
	}
}

func TestSetAOIRadius_NonExistent(t *testing.T) {
	mgr := newTestAOI()
	// Should not panic.
	mgr.SetAOIRadius(999, 50)
}

// ---------- GetVisibleEntities ----------

func TestGetVisibleEntities_NonExistent(t *testing.T) {
	mgr := newTestAOI()
	vis := mgr.GetVisibleEntities(999)
	if vis != nil {
		t.Fatalf("expected nil for non-existent entity, got %v", vis)
	}
}

// ---------- Multiple entities ----------

func TestMultipleEntities_VisibilityConsistency(t *testing.T) {
	mgr := newTestAOI()

	// Add 5 entities in a cluster.
	mgr.Add(1, v3(0, 0, 0), 50)
	mgr.Add(2, v3(10, 0, 0), 50)
	mgr.Add(3, v3(0, 0, 10), 50)
	mgr.Add(4, v3(10, 0, 10), 50)
	mgr.Add(5, v3(200, 0, 200), 50) // far away

	// Entities 1-4 should see each other but not 5.
	for _, id := range []engine.EntityID{1, 2, 3, 4} {
		vis := mgr.GetVisibleEntities(id)
		if containsID(vis, 5) {
			t.Fatalf("entity %d should not see entity 5", id)
		}
		for _, other := range []engine.EntityID{1, 2, 3, 4} {
			if other == id {
				continue
			}
			if !containsID(vis, other) {
				t.Fatalf("entity %d should see entity %d, got %v", id, other, vis)
			}
		}
	}

	// Entity 5 should see nobody.
	vis5 := mgr.GetVisibleEntities(5)
	if len(vis5) != 0 {
		t.Fatalf("entity 5 should see nobody, got %v", vis5)
	}
}

// ---------- Performance: 1000+ entities ----------

func TestPerformance_1000Entities(t *testing.T) {
	spatial := NewGridSpatialIndex(GridConfig{CellSize: 50})
	mgr := NewAOIManager(spatial)

	const n = 1000
	// Add 1000 entities spread across a 1000x1000 area.
	for i := 0; i < n; i++ {
		x := float32(i % 100) * 10
		z := float32(i/100) * 10
		mgr.Add(engine.EntityID(i+1), engine.Vector3{X: x, Y: 0, Z: z}, 50)
	}

	// Measure time to update all positions (simulating one tick).
	start := time.Now()
	for i := 0; i < n; i++ {
		x := float32(i%100)*10 + 1 // slight movement
		z := float32(i/100)*10 + 1
		mgr.UpdatePosition(engine.EntityID(i+1), engine.Vector3{X: x, Y: 0, Z: z})
	}
	elapsed := time.Since(start)

	// Should complete within a reasonable time (50ms per tick at 20 ticks/sec).
	if elapsed > 5*time.Second {
		t.Fatalf("1000 entity AOI updates took %v, too slow", elapsed)
	}
	t.Logf("1000 entity AOI updates completed in %v", elapsed)
}

// ---------- Event pairing ----------

func TestEventPairing_EnterAndLeave(t *testing.T) {
	mgr := newTestAOI()

	type eventRecord struct {
		watcher engine.EntityID
		target  engine.EntityID
	}
	var enters, leaves []eventRecord
	var mu sync.Mutex

	mgr.OnEnterView(func(w, tgt engine.EntityID) {
		mu.Lock()
		enters = append(enters, eventRecord{w, tgt})
		mu.Unlock()
	})
	mgr.OnLeaveView(func(w, tgt engine.EntityID) {
		mu.Lock()
		leaves = append(leaves, eventRecord{w, tgt})
		mu.Unlock()
	})

	mgr.Add(1, v3(0, 0, 0), 30)
	mgr.Add(2, v3(10, 0, 0), 30)

	// Both should see each other → 2 enter events.
	mu.Lock()
	enterCount := len(enters)
	mu.Unlock()
	if enterCount != 2 {
		t.Fatalf("expected 2 enter events, got %d", enterCount)
	}

	// Remove entity 2 → should produce leave events.
	mgr.Remove(2)

	mu.Lock()
	leaveCount := len(leaves)
	mu.Unlock()
	if leaveCount != 2 {
		t.Fatalf("expected 2 leave events after remove, got %d", leaveCount)
	}
}

// ---------- helpers ----------

type eventPair struct {
	watcher engine.EntityID
	target  engine.EntityID
}

// ---------- Cross-Boundary AOI Tests ----------

// newTestAOIPair creates two AOI managers and registers them as neighbors
// with the given boundary zones.
func newTestAOIPair() (AOIManager, AOIManager) {
	mgrA := NewAOIManager(NewGridSpatialIndex(GridConfig{CellSize: 50}))
	mgrB := NewAOIManager(NewGridSpatialIndex(GridConfig{CellSize: 50}))
	return mgrA, mgrB
}

func TestRegisterNeighborAOI_Basic(t *testing.T) {
	mgrA, mgrB := newTestAOIPair()

	zone := &AOIBoundaryZone{
		LocalMin:  v3(90, 0, 0),
		LocalMax:  v3(100, 100, 100),
		RemoteMin: v3(0, 0, 0),
		RemoteMax: v3(10, 100, 100),
	}

	// Should not panic.
	mgrA.RegisterNeighborAOI("B", mgrB, zone)
}

func TestUnregisterNeighborAOI_Basic(t *testing.T) {
	mgrA, mgrB := newTestAOIPair()

	zone := &AOIBoundaryZone{
		LocalMin:  v3(90, 0, 0),
		LocalMax:  v3(100, 100, 100),
		RemoteMin: v3(0, 0, 0),
		RemoteMax: v3(10, 100, 100),
	}

	mgrA.RegisterNeighborAOI("B", mgrB, zone)
	mgrA.UnregisterNeighborAOI("B")

	// After unregister, boundary entities should return nil.
	entities := mgrA.GetBoundaryEntities("B")
	if entities != nil {
		t.Fatalf("expected nil after unregister, got %v", entities)
	}
}

func TestGetBoundaryEntities_ReturnsEntitiesInLocalZone(t *testing.T) {
	mgrA, mgrB := newTestAOIPair()

	// A's boundary zone: entities near x=90..100 are in the boundary.
	zoneOnA := &AOIBoundaryZone{
		LocalMin:  v3(90, 0, 0),
		LocalMax:  v3(100, 100, 100),
		RemoteMin: v3(0, 0, 0),
		RemoteMax: v3(10, 100, 100),
	}
	mgrA.RegisterNeighborAOI("B", mgrB, zoneOnA)

	// Add entities to A: one inside boundary, one outside.
	mgrA.Add(1, v3(95, 50, 50), 20) // inside boundary zone
	mgrA.Add(2, v3(50, 50, 50), 20) // outside boundary zone
	mgrA.Add(3, v3(92, 10, 10), 20) // inside boundary zone

	entities := mgrA.GetBoundaryEntities("B")
	if len(entities) != 2 {
		t.Fatalf("expected 2 boundary entities, got %d: %v", len(entities), entities)
	}
	if !containsID(entities, 1) {
		t.Fatal("expected entity 1 in boundary")
	}
	if !containsID(entities, 3) {
		t.Fatal("expected entity 3 in boundary")
	}
	if containsID(entities, 2) {
		t.Fatal("entity 2 should NOT be in boundary")
	}
}

func TestGetBoundaryEntities_UnknownNeighbor(t *testing.T) {
	mgr := newTestAOI()
	entities := mgr.GetBoundaryEntities("nonexistent")
	if entities != nil {
		t.Fatalf("expected nil for unknown neighbor, got %v", entities)
	}
}

func TestGetCrossBoundaryVisibleEntities_LocalOnly(t *testing.T) {
	mgr := newTestAOI()
	mgr.Add(1, v3(0, 0, 0), 100)
	mgr.Add(2, v3(10, 0, 10), 100)

	result := mgr.GetCrossBoundaryVisibleEntities(1)
	if len(result) != 1 {
		t.Fatalf("expected 1 cross-boundary visible entity, got %d", len(result))
	}
	if result[0].EntityID != 2 {
		t.Fatalf("expected entity 2, got %d", result[0].EntityID)
	}
	if result[0].SourceID != "" {
		t.Fatalf("expected empty SourceID for local entity, got %q", result[0].SourceID)
	}
}

func TestGetCrossBoundaryVisibleEntities_WithNeighbor(t *testing.T) {
	mgrA, mgrB := newTestAOIPair()

	// A's boundary zone near x=90..100, B's boundary zone near x=0..10.
	zoneOnA := &AOIBoundaryZone{
		LocalMin:  v3(90, 0, 0),
		LocalMax:  v3(100, 100, 100),
		RemoteMin: v3(0, 0, 0),
		RemoteMax: v3(10, 100, 100),
	}
	mgrA.RegisterNeighborAOI("B", mgrB, zoneOnA)

	// B registers A as neighbor (reverse direction).
	zoneOnB := &AOIBoundaryZone{
		LocalMin:  v3(0, 0, 0),
		LocalMax:  v3(10, 100, 100),
		RemoteMin: v3(90, 0, 0),
		RemoteMax: v3(100, 100, 100),
	}
	mgrB.RegisterNeighborAOI("A", mgrA, zoneOnB)

	// Entity 1 in A's boundary zone.
	mgrA.Add(1, v3(95, 50, 50), 20)
	// Entity 2 in B's boundary zone.
	mgrB.Add(2, v3(5, 50, 50), 20)

	// Entity 1 is in A's local boundary zone for neighbor B,
	// so it should see entity 2 from B's remote boundary zone.
	result := mgrA.GetCrossBoundaryVisibleEntities(1)

	foundRemote := false
	for _, e := range result {
		if e.EntityID == 2 && e.SourceID == "B" {
			foundRemote = true
			break
		}
	}
	if !foundRemote {
		t.Fatalf("expected to see entity 2 from neighbor B, got %+v", result)
	}
}

func TestGetCrossBoundaryVisibleEntities_NotInBoundaryZone(t *testing.T) {
	mgrA, mgrB := newTestAOIPair()

	zoneOnA := &AOIBoundaryZone{
		LocalMin:  v3(90, 0, 0),
		LocalMax:  v3(100, 100, 100),
		RemoteMin: v3(0, 0, 0),
		RemoteMax: v3(10, 100, 100),
	}
	mgrA.RegisterNeighborAOI("B", mgrB, zoneOnA)

	// Entity 1 is NOT in the boundary zone.
	mgrA.Add(1, v3(50, 50, 50), 20)
	// Entity 2 in B's boundary zone.
	mgrB.Add(2, v3(5, 50, 50), 20)

	result := mgrA.GetCrossBoundaryVisibleEntities(1)

	// Should not see any remote entities since entity 1 is not in boundary zone.
	for _, e := range result {
		if e.SourceID != "" {
			t.Fatalf("entity not in boundary zone should not see remote entities, got %+v", result)
		}
	}
}

func TestGetCrossBoundaryVisibleEntities_NonExistentEntity(t *testing.T) {
	mgr := newTestAOI()
	result := mgr.GetCrossBoundaryVisibleEntities(999)
	if result != nil {
		t.Fatalf("expected nil for non-existent entity, got %v", result)
	}
}

func TestGetCrossBoundaryVisibleEntities_BidirectionalVisibility(t *testing.T) {
	mgrA, mgrB := newTestAOIPair()

	// Set up bidirectional boundary zones.
	zoneOnA := &AOIBoundaryZone{
		LocalMin:  v3(90, 0, 0),
		LocalMax:  v3(100, 100, 100),
		RemoteMin: v3(0, 0, 0),
		RemoteMax: v3(10, 100, 100),
	}
	zoneOnB := &AOIBoundaryZone{
		LocalMin:  v3(0, 0, 0),
		LocalMax:  v3(10, 100, 100),
		RemoteMin: v3(90, 0, 0),
		RemoteMax: v3(100, 100, 100),
	}
	mgrA.RegisterNeighborAOI("B", mgrB, zoneOnA)
	mgrB.RegisterNeighborAOI("A", mgrA, zoneOnB)

	// Entity 1 in A's boundary zone, entity 2 in B's boundary zone.
	mgrA.Add(1, v3(95, 50, 50), 20)
	mgrB.Add(2, v3(5, 50, 50), 20)

	// A's entity 1 should see B's entity 2.
	resultA := mgrA.GetCrossBoundaryVisibleEntities(1)
	foundBFromA := false
	for _, e := range resultA {
		if e.EntityID == 2 && e.SourceID == "B" {
			foundBFromA = true
		}
	}
	if !foundBFromA {
		t.Fatalf("entity 1 in A should see entity 2 from B, got %+v", resultA)
	}

	// B's entity 2 should see A's entity 1.
	resultB := mgrB.GetCrossBoundaryVisibleEntities(2)
	foundAFromB := false
	for _, e := range resultB {
		if e.EntityID == 1 && e.SourceID == "A" {
			foundAFromB = true
		}
	}
	if !foundAFromB {
		t.Fatalf("entity 2 in B should see entity 1 from A, got %+v", resultB)
	}
}

// ============================================================
// Occlusion Tests (Task 10.4)
// ============================================================

func makeHeightMap(width, height int, defaultH float32) [][]float32 {
	hm := make([][]float32, height)
	for z := 0; z < height; z++ {
		hm[z] = make([]float32, width)
		for x := 0; x < width; x++ {
			hm[z][x] = defaultH
		}
	}
	return hm
}

func TestSetOcclusionData(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial).(*aoiManager)

	hm := makeHeightMap(50, 50, 0)
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	if mgr.occlusionData == nil {
		t.Fatal("expected occlusion data to be set")
	}
}

func TestIsOccluded_NoOcclusionData(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 50)
	mgr.Add(2, engine.Vector3{X: 20, Y: 0, Z: 5}, 50)

	// No occlusion data — should never be occluded.
	if mgr.IsOccluded(1, 2) {
		t.Error("expected no occlusion without occlusion data")
	}
}

func TestIsOccluded_FlatTerrain(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	hm := makeHeightMap(50, 50, 0) // flat terrain
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 50)
	mgr.Add(2, engine.Vector3{X: 20, Y: 0, Z: 5}, 50)

	// Flat terrain — no occlusion.
	if mgr.IsOccluded(1, 2) {
		t.Error("expected no occlusion on flat terrain")
	}
}

func TestIsOccluded_HighWallBetween(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	hm := makeHeightMap(50, 50, 0)
	// Create a high wall at x=12 across all z.
	for z := 0; z < 50; z++ {
		hm[z][12] = 100
	}
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	// Watcher at (5, 0, 5), target at (20, 0, 5) — wall at x=12 blocks view.
	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 50)
	mgr.Add(2, engine.Vector3{X: 20, Y: 0, Z: 5}, 50)

	if !mgr.IsOccluded(1, 2) {
		t.Error("expected occlusion due to high wall between entities")
	}
}

func TestIsOccluded_WallNotHighEnough(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	hm := makeHeightMap(50, 50, 0)
	// Low wall at x=12.
	for z := 0; z < 50; z++ {
		hm[z][12] = 2
	}
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	// Entities at Y=10 — wall height 2 is below both entities.
	mgr.Add(1, engine.Vector3{X: 5, Y: 10, Z: 5}, 50)
	mgr.Add(2, engine.Vector3{X: 20, Y: 10, Z: 5}, 50)

	if mgr.IsOccluded(1, 2) {
		t.Error("expected no occlusion — wall is below both entities")
	}
}

func TestIsOccluded_DifferentLayers(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	hm := makeHeightMap(50, 50, 0)
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 50)
	mgr.Add(2, engine.Vector3{X: 20, Y: 0, Z: 5}, 50)
	mgr.SetEntityLayer(1, 0)
	mgr.SetEntityLayer(2, 1) // different layer

	// Different layers — always occluded.
	if !mgr.IsOccluded(1, 2) {
		t.Error("expected occlusion for entities on different layers")
	}
}

func TestIsOccluded_SameLayer(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	hm := makeHeightMap(50, 50, 0)
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 50)
	mgr.Add(2, engine.Vector3{X: 20, Y: 0, Z: 5}, 50)
	mgr.SetEntityLayer(1, 0)
	mgr.SetEntityLayer(2, 0)

	// Same layer, flat terrain — no occlusion.
	if mgr.IsOccluded(1, 2) {
		t.Error("expected no occlusion for same layer on flat terrain")
	}
}

func TestIsOccluded_CloseEntities(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	hm := makeHeightMap(50, 50, 100) // all high terrain
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	// Entities very close together — should not be occluded even with high terrain.
	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 50)
	mgr.Add(2, engine.Vector3{X: 6, Y: 0, Z: 5}, 50)

	if mgr.IsOccluded(1, 2) {
		t.Error("expected no occlusion for very close entities")
	}
}

func TestIsOccluded_EntityNotFound(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial)

	hm := makeHeightMap(50, 50, 0)
	mgr.SetOcclusionData(&OcclusionData{HeightMap: hm, LayerIndex: 0})

	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 50)

	if mgr.IsOccluded(1, 999) {
		t.Error("expected false for nonexistent target")
	}
	if mgr.IsOccluded(999, 1) {
		t.Error("expected false for nonexistent watcher")
	}
}

func TestSetEntityLayer(t *testing.T) {
	spatial := NewGridSpatialIndex(DefaultGridConfig())
	mgr := NewAOIManager(spatial).(*aoiManager)

	mgr.Add(1, engine.Vector3{X: 5, Y: 0, Z: 5}, 10)

	mgr.SetEntityLayer(1, 3)
	ent := mgr.entities[1]
	if ent.layerIndex != 3 {
		t.Errorf("expected layer 3, got %d", ent.layerIndex)
	}

	// Setting layer on nonexistent entity should be a no-op.
	mgr.SetEntityLayer(999, 5) // should not panic
}
