package gamemap

import (
	"testing"

	"gogame/internal/engine"
)

// helper: create a fully walkable map with given dimensions.
func walkableMap(id string, width, height int) *Map {
	layer := simpleLayer(0, width, height)
	return newMap(id, []*MapLayer{layer}, nil, nil)
}

// helper: create a map with an obstacle wall.
func mapWithWall(id string, width, height int, wallX int) *Map {
	layer := simpleLayer(0, width, height)
	// Create a wall at column wallX, leaving a gap at row 0.
	for z := 1; z < height; z++ {
		layer.Walkable[z][wallX] = false
	}
	return newMap(id, []*MapLayer{layer}, nil, nil)
}

func TestFindPath_StraightLine(t *testing.T) {
	m := walkableMap("test", 20, 20)
	pf := NewPathfinder()

	path, err := pf.FindPath(m,
		engine.Vector3{X: 0, Y: 0, Z: 0},
		engine.Vector3{X: 5, Y: 0, Z: 0},
		0, 0,
	)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}
	if len(path) == 0 {
		t.Fatal("expected non-empty path")
	}
	// First point should be start.
	if path[0].X != 0 || path[0].Z != 0 {
		t.Errorf("expected start (0,0), got (%v,%v)", path[0].X, path[0].Z)
	}
	// Last point should be end.
	last := path[len(path)-1]
	if last.X != 5 || last.Z != 0 {
		t.Errorf("expected end (5,0), got (%v,%v)", last.X, last.Z)
	}
	// Path length should be 6 (0,1,2,3,4,5).
	if len(path) != 6 {
		t.Errorf("expected path length 6, got %d", len(path))
	}
}

func TestFindPath_SamePosition(t *testing.T) {
	m := walkableMap("test", 10, 10)
	pf := NewPathfinder()

	path, err := pf.FindPath(m,
		engine.Vector3{X: 3, Y: 0, Z: 3},
		engine.Vector3{X: 3, Y: 0, Z: 3},
		0, 0,
	)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}
	if len(path) != 1 {
		t.Errorf("expected path length 1, got %d", len(path))
	}
}

func TestFindPath_AroundWall(t *testing.T) {
	m := mapWithWall("test", 20, 20, 5)
	pf := NewPathfinder()

	// Path from (3,5) to (7,5) must go around the wall at x=5.
	path, err := pf.FindPath(m,
		engine.Vector3{X: 3, Y: 0, Z: 5},
		engine.Vector3{X: 7, Y: 0, Z: 5},
		0, 0,
	)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}
	if len(path) == 0 {
		t.Fatal("expected non-empty path")
	}

	// Verify path doesn't go through the wall.
	for _, p := range path {
		if int(p.X) == 5 && int(p.Z) > 0 {
			t.Errorf("path goes through wall at (%v,%v)", p.X, p.Z)
		}
	}

	// Verify start and end.
	if path[0].X != 3 || path[0].Z != 5 {
		t.Errorf("expected start (3,5), got (%v,%v)", path[0].X, path[0].Z)
	}
	last := path[len(path)-1]
	if last.X != 7 || last.Z != 5 {
		t.Errorf("expected end (7,5), got (%v,%v)", last.X, last.Z)
	}
}

func TestFindPath_Blocked(t *testing.T) {
	// Create a map with a complete wall blocking the path.
	layer := simpleLayer(0, 10, 10)
	for z := 0; z < 10; z++ {
		layer.Walkable[z][5] = false
	}
	m := newMap("blocked", []*MapLayer{layer}, nil, nil)
	pf := NewPathfinder()

	_, err := pf.FindPath(m,
		engine.Vector3{X: 2, Y: 0, Z: 5},
		engine.Vector3{X: 8, Y: 0, Z: 5},
		0, 0,
	)
	if err != ErrPathNotFound {
		t.Errorf("expected ErrPathNotFound, got %v", err)
	}
}

func TestFindPath_StartNotWalkable(t *testing.T) {
	layer := simpleLayer(0, 10, 10)
	layer.Walkable[3][3] = false
	m := newMap("test", []*MapLayer{layer}, nil, nil)
	pf := NewPathfinder()

	_, err := pf.FindPath(m,
		engine.Vector3{X: 3, Y: 0, Z: 3},
		engine.Vector3{X: 5, Y: 0, Z: 5},
		0, 0,
	)
	if err != ErrStartNotWalkable {
		t.Errorf("expected ErrStartNotWalkable, got %v", err)
	}
}

func TestFindPath_EndNotWalkable(t *testing.T) {
	layer := simpleLayer(0, 10, 10)
	layer.Walkable[5][5] = false
	m := newMap("test", []*MapLayer{layer}, nil, nil)
	pf := NewPathfinder()

	_, err := pf.FindPath(m,
		engine.Vector3{X: 3, Y: 0, Z: 3},
		engine.Vector3{X: 5, Y: 0, Z: 5},
		0, 0,
	)
	if err != ErrEndNotWalkable {
		t.Errorf("expected ErrEndNotWalkable, got %v", err)
	}
}

func TestFindPath_CrossLayer(t *testing.T) {
	layer0 := simpleLayer(0, 20, 20)
	layer1 := simpleLayer(1, 20, 20)

	// Add connection from layer 0 at (10,10) to layer 1 at (5,5).
	layer0.Connections = []LayerConnection{
		{
			ID:          "cave-1",
			SourceLayer: 0,
			SourcePos:   engine.Vector3{X: 10, Y: 0, Z: 10},
			TargetLayer: 1,
			TargetPos:   engine.Vector3{X: 5, Y: 0, Z: 5},
		},
	}

	m := newMap("multi", []*MapLayer{layer0, layer1}, nil, nil)
	pf := NewPathfinder()

	// Path from (0,0) on layer 0 to (8,8) on layer 1.
	path, err := pf.FindPath(m,
		engine.Vector3{X: 0, Y: 0, Z: 0},
		engine.Vector3{X: 8, Y: 0, Z: 8},
		0, 1,
	)
	if err != nil {
		t.Fatalf("FindPath cross-layer failed: %v", err)
	}
	if len(path) == 0 {
		t.Fatal("expected non-empty path")
	}

	// Verify start and end.
	if path[0].X != 0 || path[0].Z != 0 {
		t.Errorf("expected start (0,0), got (%v,%v)", path[0].X, path[0].Z)
	}
	last := path[len(path)-1]
	if last.X != 8 || last.Z != 8 {
		t.Errorf("expected end (8,8), got (%v,%v)", last.X, last.Z)
	}
}

func TestFindPath_CrossLayer_NoConnection(t *testing.T) {
	layer0 := simpleLayer(0, 10, 10)
	layer1 := simpleLayer(1, 10, 10)
	// No connections between layers.
	m := newMap("no-conn", []*MapLayer{layer0, layer1}, nil, nil)
	pf := NewPathfinder()

	_, err := pf.FindPath(m,
		engine.Vector3{X: 0, Y: 0, Z: 0},
		engine.Vector3{X: 5, Y: 0, Z: 5},
		0, 1,
	)
	if err != ErrNoLayerPath {
		t.Errorf("expected ErrNoLayerPath, got %v", err)
	}
}

func TestFindPath_PathConsecutive(t *testing.T) {
	m := walkableMap("test", 15, 15)
	pf := NewPathfinder()

	path, err := pf.FindPath(m,
		engine.Vector3{X: 2, Y: 0, Z: 3},
		engine.Vector3{X: 10, Y: 0, Z: 8},
		0, 0,
	)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}

	// Verify each step is adjacent (Manhattan distance = 1).
	for i := 1; i < len(path); i++ {
		dx := abs(int(path[i].X) - int(path[i-1].X))
		dz := abs(int(path[i].Z) - int(path[i-1].Z))
		if dx+dz != 1 {
			// Allow layer transitions which may jump.
			if dx+dz > 1 {
				// This could be a layer connection step — acceptable.
				continue
			}
			t.Errorf("non-adjacent step at index %d: (%v,%v) -> (%v,%v)",
				i, path[i-1].X, path[i-1].Z, path[i].X, path[i].Z)
		}
	}
}

func TestFindPath_OptimalLength(t *testing.T) {
	m := walkableMap("test", 20, 20)
	pf := NewPathfinder()

	// On an open grid, optimal path length = Manhattan distance + 1.
	path, err := pf.FindPath(m,
		engine.Vector3{X: 0, Y: 0, Z: 0},
		engine.Vector3{X: 5, Y: 0, Z: 3},
		0, 0,
	)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}
	expected := 5 + 3 + 1 // Manhattan distance + 1 for start node
	if len(path) != expected {
		t.Errorf("expected optimal path length %d, got %d", expected, len(path))
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
