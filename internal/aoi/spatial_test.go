package aoi

import (
	"math"
	"sort"
	"testing"

	"gfgame/internal/engine"
)

func v3(x, y, z float32) engine.Vector3 {
	return engine.Vector3{X: x, Y: y, Z: z}
}

func sortIDs(ids []engine.EntityID) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
}

func containsID(ids []engine.EntityID, id engine.EntityID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// ---------- Insert & QueryRange ----------

func TestInsertAndQueryRange_Basic(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 50})

	idx.Insert(1, v3(10, 0, 10))
	idx.Insert(2, v3(20, 0, 20))
	idx.Insert(3, v3(200, 0, 200))

	// Query with radius that covers entities 1 and 2 but not 3.
	result := idx.QueryRange(v3(15, 0, 15), 30)
	sortIDs(result)

	if len(result) != 2 {
		t.Fatalf("expected 2 entities, got %d: %v", len(result), result)
	}
	if result[0] != 1 || result[1] != 2 {
		t.Fatalf("expected [1,2], got %v", result)
	}
}

func TestQueryRange_ZeroRadius(t *testing.T) {
	idx := NewGridSpatialIndex(DefaultGridConfig())
	idx.Insert(1, v3(0, 0, 0))

	result := idx.QueryRange(v3(0, 0, 0), 0)
	if len(result) != 0 {
		t.Fatalf("expected empty result for zero radius, got %v", result)
	}
}

func TestQueryRange_NegativeRadius(t *testing.T) {
	idx := NewGridSpatialIndex(DefaultGridConfig())
	idx.Insert(1, v3(0, 0, 0))

	result := idx.QueryRange(v3(0, 0, 0), -10)
	if len(result) != 0 {
		t.Fatalf("expected empty result for negative radius, got %v", result)
	}
}

func TestQueryRange_IncludesY(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 50})

	// Same X/Z but different Y — should be excluded if 3D distance exceeds radius.
	idx.Insert(1, v3(0, 0, 0))
	idx.Insert(2, v3(0, 100, 0))

	result := idx.QueryRange(v3(0, 0, 0), 50)
	if len(result) != 1 || result[0] != 1 {
		t.Fatalf("expected only entity 1 within Y-aware radius, got %v", result)
	}
}

// ---------- Remove ----------

func TestRemove(t *testing.T) {
	idx := NewGridSpatialIndex(DefaultGridConfig())
	idx.Insert(1, v3(0, 0, 0))
	idx.Insert(2, v3(5, 0, 5))

	idx.Remove(1)

	result := idx.QueryRange(v3(0, 0, 0), 1000)
	if len(result) != 1 || result[0] != 2 {
		t.Fatalf("after remove, expected [2], got %v", result)
	}
}

func TestRemove_NonExistent(t *testing.T) {
	idx := NewGridSpatialIndex(DefaultGridConfig())
	// Should not panic.
	idx.Remove(999)
}

// ---------- Update ----------

func TestUpdate_MoveWithinCell(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 100})
	idx.Insert(1, v3(10, 0, 10))

	idx.Update(1, v3(20, 0, 20))

	// Entity should still be found near the new position.
	result := idx.QueryRange(v3(20, 0, 20), 5)
	if !containsID(result, 1) {
		t.Fatalf("entity 1 should be at updated position")
	}

	// Should NOT be found near the old position with a tight radius.
	result = idx.QueryRange(v3(10, 0, 10), 5)
	if containsID(result, 1) {
		t.Fatalf("entity 1 should have moved away from old position")
	}
}

func TestUpdate_MoveCrossCell(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 50})
	idx.Insert(1, v3(10, 0, 10))

	// Move to a different cell.
	idx.Update(1, v3(200, 0, 200))

	result := idx.QueryRange(v3(200, 0, 200), 10)
	if !containsID(result, 1) {
		t.Fatalf("entity 1 should be in new cell")
	}

	result = idx.QueryRange(v3(10, 0, 10), 10)
	if containsID(result, 1) {
		t.Fatalf("entity 1 should no longer be in old cell")
	}
}

func TestUpdate_NonExistentActsAsInsert(t *testing.T) {
	idx := NewGridSpatialIndex(DefaultGridConfig())
	idx.Update(42, v3(50, 0, 50))

	result := idx.QueryRange(v3(50, 0, 50), 10)
	if !containsID(result, 42) {
		t.Fatalf("update on non-existent entity should insert it")
	}
}

// ---------- Edge cases ----------

func TestInsert_DuplicateOverwrites(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 50})
	idx.Insert(1, v3(0, 0, 0))
	idx.Insert(1, v3(100, 0, 100)) // re-insert at new position

	result := idx.QueryRange(v3(100, 0, 100), 10)
	if !containsID(result, 1) {
		t.Fatalf("re-inserted entity should be at new position")
	}

	result = idx.QueryRange(v3(0, 0, 0), 10)
	if containsID(result, 1) {
		t.Fatalf("re-inserted entity should not remain at old position")
	}
}

func TestNegativeCoordinates(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 50})
	idx.Insert(1, v3(-100, 0, -100))
	idx.Insert(2, v3(-90, 0, -90))

	result := idx.QueryRange(v3(-95, 0, -95), 20)
	sortIDs(result)
	if len(result) != 2 {
		t.Fatalf("expected 2 entities in negative coords, got %d", len(result))
	}
}

func TestQueryRange_ExactBoundary(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 50})
	idx.Insert(1, v3(0, 0, 0))

	// Entity at exactly the radius distance should be included (<=).
	dist := float32(10.0)
	idx.Insert(2, v3(dist, 0, 0))

	result := idx.QueryRange(v3(0, 0, 0), dist)
	if !containsID(result, 2) {
		t.Fatalf("entity at exact radius distance should be included")
	}
}

func TestDefaultGridConfig(t *testing.T) {
	cfg := DefaultGridConfig()
	if cfg.CellSize != 100 {
		t.Fatalf("expected default cell size 100, got %v", cfg.CellSize)
	}
}

func TestNewGridSpatialIndex_InvalidCellSize(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 0})
	if idx.cellSize != 100 {
		t.Fatalf("expected fallback cell size 100, got %v", idx.cellSize)
	}

	idx2 := NewGridSpatialIndex(GridConfig{CellSize: -5})
	if idx2.cellSize != 100 {
		t.Fatalf("expected fallback cell size 100 for negative, got %v", idx2.cellSize)
	}
}

// ---------- Larger scale ----------

func TestQueryRange_ManyEntities(t *testing.T) {
	idx := NewGridSpatialIndex(GridConfig{CellSize: 50})

	// Insert 1000 entities in a 100x100 grid.
	for i := 0; i < 1000; i++ {
		x := float32(i % 100)
		z := float32(i / 100)
		idx.Insert(engine.EntityID(i+1), v3(x, 0, z))
	}

	// Query a small area — should return only nearby entities.
	result := idx.QueryRange(v3(50, 0, 5), 5)
	for _, eid := range result {
		entry := idx.entities[eid]
		dx := float64(entry.pos.X - 50)
		dz := float64(entry.pos.Z - 5)
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 5.0+1e-6 {
			t.Fatalf("entity %d at distance %.2f exceeds radius 5", eid, dist)
		}
	}
}
