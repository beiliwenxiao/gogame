package gamemap

import (
	"testing"

	"gogame/internal/engine"
)

// helper: create a simple walkable layer.
func simpleLayer(index int, width, height int) *MapLayer {
	walkable := make([][]bool, height)
	heightMap := make([][]float32, height)
	for z := 0; z < height; z++ {
		walkable[z] = make([]bool, width)
		heightMap[z] = make([]float32, width)
		for x := 0; x < width; x++ {
			walkable[z][x] = true
			heightMap[z][x] = float32(z + x)
		}
	}
	return &MapLayer{
		ID:         "layer-" + string(rune('0'+index)),
		LayerIndex: index,
		HeightMap:  heightMap,
		Walkable:   walkable,
	}
}

// helper: create a MapConfig with one layer.
func basicMapConfig(id string) *MapConfig {
	layer := simpleLayer(0, 100, 100)
	return &MapConfig{
		ID:     id,
		Layers: []*MapLayer{layer},
	}
}

// helper: create a MapManager with a simple loader.
func newTestManager(configs map[string]*MapConfig) *mapManager {
	loader := func(mapID string) (*MapConfig, error) {
		cfg, ok := configs[mapID]
		if !ok {
			return nil, ErrMapNotFound
		}
		return cfg, nil
	}
	mm := NewMapManager(WithMapLoader(loader)).(*mapManager)
	return mm
}

func TestLoadMap(t *testing.T) {
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
	}
	mm := newTestManager(configs)

	m, err := mm.LoadMap("map-1")
	if err != nil {
		t.Fatalf("LoadMap failed: %v", err)
	}
	if m.ID != "map-1" {
		t.Errorf("expected map ID 'map-1', got %q", m.ID)
	}
	if len(m.Layers) != 1 {
		t.Errorf("expected 1 layer, got %d", len(m.Layers))
	}
}

func TestLoadMap_Duplicate(t *testing.T) {
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
	}
	mm := newTestManager(configs)

	mm.LoadMap("map-1")
	_, err := mm.LoadMap("map-1")
	if err != ErrMapExists {
		t.Errorf("expected ErrMapExists, got %v", err)
	}
}

func TestLoadMap_NotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	_, err := mm.LoadMap("nonexistent")
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}
}

func TestLoadMap_NoLoader(t *testing.T) {
	mm := NewMapManager().(*mapManager)
	_, err := mm.LoadMap("map-1")
	if err != ErrMapLoaderNotSet {
		t.Errorf("expected ErrMapLoaderNotSet, got %v", err)
	}
}

func TestUnloadMap(t *testing.T) {
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
	}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	err := mm.UnloadMap("map-1")
	if err != nil {
		t.Fatalf("UnloadMap failed: %v", err)
	}

	_, ok := mm.GetMap("map-1")
	if ok {
		t.Error("expected map to be unloaded")
	}
}

func TestUnloadMap_NotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	err := mm.UnloadMap("nonexistent")
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}
}

func TestGetMap(t *testing.T) {
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
	}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	m, ok := mm.GetMap("map-1")
	if !ok || m == nil {
		t.Fatal("expected to find map-1")
	}

	_, ok = mm.GetMap("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent map")
	}
}

func TestIsWalkable(t *testing.T) {
	cfg := basicMapConfig("map-1")
	// Make position (5, 5) not walkable.
	cfg.Layers[0].Walkable[5][5] = false

	configs := map[string]*MapConfig{"map-1": cfg}
	mm := newTestManager(configs)
	m, _ := mm.LoadMap("map-1")

	if !m.IsWalkable(0, 10, 10) {
		t.Error("expected (10,10) to be walkable")
	}
	if m.IsWalkable(0, 5, 5) {
		t.Error("expected (5,5) to NOT be walkable")
	}
	if m.IsWalkable(0, -1, 0) {
		t.Error("expected negative coords to NOT be walkable")
	}
	if m.IsWalkable(0, 200, 0) {
		t.Error("expected out-of-bounds to NOT be walkable")
	}
	if m.IsWalkable(99, 0, 0) {
		t.Error("expected nonexistent layer to NOT be walkable")
	}
}

func TestGetHeight(t *testing.T) {
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
	}
	mm := newTestManager(configs)
	m, _ := mm.LoadMap("map-1")

	h, ok := m.GetHeight(0, 3, 4)
	if !ok {
		t.Fatal("expected to get height")
	}
	if h != 7 { // z + x = 4 + 3
		t.Errorf("expected height 7, got %v", h)
	}

	_, ok = m.GetHeight(99, 0, 0)
	if ok {
		t.Error("expected false for nonexistent layer")
	}

	_, ok = m.GetHeight(0, -1, 0)
	if ok {
		t.Error("expected false for negative coords")
	}
}

func TestMultiLayerMap(t *testing.T) {
	layer0 := simpleLayer(0, 50, 50)
	layer1 := simpleLayer(1, 50, 50)
	layer0.Connections = []LayerConnection{
		{
			ID:             "cave-entrance",
			SourceLayer:    0,
			SourcePos:      engine.Vector3{X: 25, Y: 0, Z: 25},
			TargetLayer:    1,
			TargetPos:      engine.Vector3{X: 10, Y: 0, Z: 10},
			ConnectionType: "cave_entrance",
			PreloadDistance: 5,
		},
	}

	cfg := &MapConfig{
		ID:     "multi-layer",
		Layers: []*MapLayer{layer0, layer1},
	}
	configs := map[string]*MapConfig{"multi-layer": cfg}
	mm := newTestManager(configs)
	m, _ := mm.LoadMap("multi-layer")

	if len(m.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(m.Layers))
	}

	l0, ok := m.GetLayer(0)
	if !ok {
		t.Fatal("expected layer 0")
	}
	if l0.LayerIndex != 0 {
		t.Errorf("expected layer index 0, got %d", l0.LayerIndex)
	}

	l1, ok := m.GetLayer(1)
	if !ok {
		t.Fatal("expected layer 1")
	}
	if l1.LayerIndex != 1 {
		t.Errorf("expected layer index 1, got %d", l1.LayerIndex)
	}

	conn, ok := m.GetConnection("cave-entrance")
	if !ok {
		t.Fatal("expected to find cave-entrance connection")
	}
	if conn.TargetLayer != 1 {
		t.Errorf("expected target layer 1, got %d", conn.TargetLayer)
	}
}

func TestTeleport(t *testing.T) {
	layer := simpleLayer(0, 100, 100)
	cfg1 := &MapConfig{
		ID:     "map-1",
		Layers: []*MapLayer{layer},
		Teleports: []TeleportPoint{
			{ID: "tp-1", Position: engine.Vector3{X: 50, Y: 0, Z: 50}, TargetMap: "map-2", TargetPos: engine.Vector3{X: 10, Y: 0, Z: 10}},
		},
	}
	cfg2 := &MapConfig{
		ID:     "map-2",
		Layers: []*MapLayer{simpleLayer(0, 100, 100)},
	}

	var updatedMapID string
	var updatedPos engine.Vector3
	updater := func(entityID engine.EntityID, mapID string, layerIndex int, pos engine.Vector3) error {
		updatedMapID = mapID
		updatedPos = pos
		return nil
	}

	configs := map[string]*MapConfig{"map-1": cfg1, "map-2": cfg2}
	mm := NewMapManager(
		WithMapLoader(func(id string) (*MapConfig, error) {
			c, ok := configs[id]
			if !ok {
				return nil, ErrMapNotFound
			}
			return c, nil
		}),
		WithPositionUpdater(updater),
	).(*mapManager)

	mm.LoadMap("map-1")
	mm.LoadMap("map-2")

	err := mm.Teleport(engine.EntityID(1), "map-1", "map-2", engine.Vector3{X: 10, Y: 0, Z: 10})
	if err != nil {
		t.Fatalf("Teleport failed: %v", err)
	}
	if updatedMapID != "map-2" {
		t.Errorf("expected updated map 'map-2', got %q", updatedMapID)
	}
	if updatedPos.X != 10 || updatedPos.Z != 10 {
		t.Errorf("expected pos (10,0,10), got %v", updatedPos)
	}
}

func TestTeleport_MapNotFound(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	err := mm.Teleport(engine.EntityID(1), "map-1", "nonexistent", engine.Vector3{})
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}

	err = mm.Teleport(engine.EntityID(1), "nonexistent", "map-1", engine.Vector3{})
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound for source, got %v", err)
	}
}


func TestTeleport_NotWalkable(t *testing.T) {
	cfg := basicMapConfig("map-1")
	cfg.Layers[0].Walkable[10][10] = false
	cfg2 := &MapConfig{
		ID:     "map-2",
		Layers: []*MapLayer{cfg.Layers[0]},
	}

	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1"), "map-2": cfg2}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")
	mm.LoadMap("map-2")

	err := mm.Teleport(engine.EntityID(1), "map-1", "map-2", engine.Vector3{X: 10, Y: 0, Z: 10})
	if err != ErrNotWalkable {
		t.Errorf("expected ErrNotWalkable, got %v", err)
	}
}

func TestSwitchLayer(t *testing.T) {
	layer0 := simpleLayer(0, 50, 50)
	layer1 := simpleLayer(1, 50, 50)
	layer0.Connections = []LayerConnection{
		{
			ID:          "stairs-1",
			SourceLayer: 0,
			SourcePos:   engine.Vector3{X: 20, Y: 0, Z: 20},
			TargetLayer: 1,
			TargetPos:   engine.Vector3{X: 5, Y: 0, Z: 5},
		},
	}

	var updatedLayer int
	var updatedPos engine.Vector3
	updater := func(entityID engine.EntityID, mapID string, layerIndex int, pos engine.Vector3) error {
		updatedLayer = layerIndex
		updatedPos = pos
		return nil
	}

	cfg := &MapConfig{ID: "map-1", Layers: []*MapLayer{layer0, layer1}}
	configs := map[string]*MapConfig{"map-1": cfg}
	mm := NewMapManager(
		WithMapLoader(func(id string) (*MapConfig, error) {
			c, ok := configs[id]
			if !ok {
				return nil, ErrMapNotFound
			}
			return c, nil
		}),
		WithPositionUpdater(updater),
	).(*mapManager)
	mm.LoadMap("map-1")

	err := mm.SwitchLayer(engine.EntityID(1), "map-1", "stairs-1")
	if err != nil {
		t.Fatalf("SwitchLayer failed: %v", err)
	}
	if updatedLayer != 1 {
		t.Errorf("expected layer 1, got %d", updatedLayer)
	}
	if updatedPos.X != 5 || updatedPos.Z != 5 {
		t.Errorf("expected pos (5,0,5), got %v", updatedPos)
	}
}

func TestSwitchLayer_MapNotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	err := mm.SwitchLayer(engine.EntityID(1), "nonexistent", "conn-1")
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}
}

func TestSwitchLayer_ConnectionNotFound(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	err := mm.SwitchLayer(engine.EntityID(1), "map-1", "nonexistent")
	if err != ErrConnectionNotFound {
		t.Errorf("expected ErrConnectionNotFound, got %v", err)
	}
}

func TestHotReload(t *testing.T) {
	version := 1
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
	}

	loader := func(mapID string) (*MapConfig, error) {
		if version == 2 {
			// Return updated config with 2 layers.
			return &MapConfig{
				ID:     "map-1",
				Layers: []*MapLayer{simpleLayer(0, 50, 50), simpleLayer(1, 50, 50)},
			}, nil
		}
		return configs[mapID], nil
	}

	mm := NewMapManager(WithMapLoader(loader)).(*mapManager)
	mm.LoadMap("map-1")

	m, _ := mm.GetMap("map-1")
	if len(m.Layers) != 1 {
		t.Fatalf("expected 1 layer before reload, got %d", len(m.Layers))
	}

	version = 2
	err := mm.HotReload("map-1")
	if err != nil {
		t.Fatalf("HotReload failed: %v", err)
	}

	m, _ = mm.GetMap("map-1")
	if len(m.Layers) != 2 {
		t.Errorf("expected 2 layers after reload, got %d", len(m.Layers))
	}
}

func TestHotReload_MapNotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	err := mm.HotReload("nonexistent")
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}
}

func TestConfigureMapBoundary(t *testing.T) {
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
	}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	zone := &MapBoundaryZone{
		LocalMin:        engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:        engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin:       engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax:       engine.Vector3{X: 10, Y: 100, Z: 100},
		PreloadDistance: 15,
	}

	err := mm.ConfigureMapBoundary("map-1", "map-2", zone)
	if err != nil {
		t.Fatalf("ConfigureMapBoundary failed: %v", err)
	}

	bz, ok := mm.GetMapBoundary("map-1", "map-2")
	if !ok {
		t.Fatal("expected to find boundary")
	}
	if bz.PreloadDistance != 15 {
		t.Errorf("expected PreloadDistance 15, got %v", bz.PreloadDistance)
	}
	if bz.NeighborMapID != "map-2" {
		t.Errorf("expected neighbor map-2, got %q", bz.NeighborMapID)
	}
}

func TestConfigureMapBoundary_MapNotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	err := mm.ConfigureMapBoundary("nonexistent", "map-2", &MapBoundaryZone{})
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}
}

func TestGetMapBoundary_NotFound(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	_, ok := mm.GetMapBoundary("map-1", "nonexistent")
	if ok {
		t.Error("expected no boundary for nonexistent neighbor")
	}

	_, ok = mm.GetMapBoundary("nonexistent", "map-1")
	if ok {
		t.Error("expected no boundary for nonexistent map")
	}
}

func TestLoadMapFromConfig(t *testing.T) {
	mm := NewMapManager().(*mapManager)

	cfg := basicMapConfig("direct-map")
	m, err := mm.LoadMapFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadMapFromConfig failed: %v", err)
	}
	if m.ID != "direct-map" {
		t.Errorf("expected ID 'direct-map', got %q", m.ID)
	}

	// Duplicate.
	_, err = mm.LoadMapFromConfig(cfg)
	if err != ErrMapExists {
		t.Errorf("expected ErrMapExists, got %v", err)
	}
}

func TestTeleportHook(t *testing.T) {
	var hookCalled bool
	var hookFrom, hookTo string

	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
		"map-2": basicMapConfig("map-2"),
	}
	mm := NewMapManager(
		WithMapLoader(func(id string) (*MapConfig, error) {
			c, ok := configs[id]
			if !ok {
				return nil, ErrMapNotFound
			}
			return c, nil
		}),
		WithTeleportHook(func(entityID engine.EntityID, from, to string) {
			hookCalled = true
			hookFrom = from
			hookTo = to
		}),
	).(*mapManager)

	mm.LoadMap("map-1")
	mm.LoadMap("map-2")

	mm.Teleport(engine.EntityID(1), "map-1", "map-2", engine.Vector3{X: 5, Y: 0, Z: 5})

	if !hookCalled {
		t.Error("expected teleport hook to be called")
	}
	if hookFrom != "map-1" || hookTo != "map-2" {
		t.Errorf("expected hook from=map-1 to=map-2, got from=%q to=%q", hookFrom, hookTo)
	}
}

func TestMapGetTeleport(t *testing.T) {
	cfg := &MapConfig{
		ID:     "map-1",
		Layers: []*MapLayer{simpleLayer(0, 50, 50)},
		Teleports: []TeleportPoint{
			{ID: "tp-1", Position: engine.Vector3{X: 10, Y: 0, Z: 10}, TargetMap: "map-2", TargetPos: engine.Vector3{X: 5, Y: 0, Z: 5}},
		},
	}
	configs := map[string]*MapConfig{"map-1": cfg}
	mm := newTestManager(configs)
	m, _ := mm.LoadMap("map-1")

	tp, ok := m.GetTeleport("tp-1")
	if !ok {
		t.Fatal("expected to find teleport tp-1")
	}
	if tp.TargetMap != "map-2" {
		t.Errorf("expected target map-2, got %q", tp.TargetMap)
	}

	_, ok = m.GetTeleport("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent teleport")
	}
}

func TestEmptyHeightMapAndWalkable(t *testing.T) {
	// Layer with no height map or walkable data.
	layer := &MapLayer{ID: "empty", LayerIndex: 0}
	cfg := &MapConfig{ID: "empty-map", Layers: []*MapLayer{layer}}
	mm := NewMapManager().(*mapManager)
	m, _ := mm.LoadMapFromConfig(cfg)

	// No walkable data means all walkable.
	if !m.IsWalkable(0, 5, 5) {
		t.Error("expected walkable when no walkable data")
	}

	// No height map returns 0.
	h, ok := m.GetHeight(0, 5, 5)
	if !ok {
		t.Error("expected ok for empty height map")
	}
	if h != 0 {
		t.Errorf("expected height 0, got %v", h)
	}
}


// ============================================================
// Task 10.5: Seamless Map Transition & Layer Switch Tests
// ============================================================

func TestCheckBoundaryProximity_InZone(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	mm.ConfigureMapBoundary("map-1", "map-2", &MapBoundaryZone{
		LocalMin:        engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:        engine.Vector3{X: 100, Y: 100, Z: 100},
		PreloadDistance: 10,
	})

	// Entity inside boundary zone.
	need, neighbor := mm.CheckBoundaryProximity(engine.EntityID(1), "map-1", engine.Vector3{X: 95, Y: 0, Z: 50})
	if !need {
		t.Error("expected preload needed")
	}
	if neighbor != "map-2" {
		t.Errorf("expected neighbor map-2, got %q", neighbor)
	}
}

func TestCheckBoundaryProximity_NearZone(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	mm.ConfigureMapBoundary("map-1", "map-2", &MapBoundaryZone{
		LocalMin:        engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:        engine.Vector3{X: 100, Y: 100, Z: 100},
		PreloadDistance: 10,
	})

	// Entity within preload distance but outside zone.
	need, neighbor := mm.CheckBoundaryProximity(engine.EntityID(1), "map-1", engine.Vector3{X: 85, Y: 0, Z: 50})
	if !need {
		t.Error("expected preload needed within preload distance")
	}
	if neighbor != "map-2" {
		t.Errorf("expected neighbor map-2, got %q", neighbor)
	}
}

func TestCheckBoundaryProximity_FarAway(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	mm.ConfigureMapBoundary("map-1", "map-2", &MapBoundaryZone{
		LocalMin:        engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:        engine.Vector3{X: 100, Y: 100, Z: 100},
		PreloadDistance: 5,
	})

	// Entity far from boundary.
	need, _ := mm.CheckBoundaryProximity(engine.EntityID(1), "map-1", engine.Vector3{X: 10, Y: 0, Z: 50})
	if need {
		t.Error("expected no preload needed for far entity")
	}
}

func TestCheckBoundaryProximity_MapNotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	need, _ := mm.CheckBoundaryProximity(engine.EntityID(1), "nonexistent", engine.Vector3{})
	if need {
		t.Error("expected false for nonexistent map")
	}
}

func TestTriggerMapBoundaryPreload(t *testing.T) {
	cfg1 := basicMapConfig("map-1")
	cfg2 := basicMapConfig("map-2")
	configs := map[string]*MapConfig{"map-1": cfg1, "map-2": cfg2}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	mm.ConfigureMapBoundary("map-1", "map-2", &MapBoundaryZone{
		LocalMin: engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax: engine.Vector3{X: 100, Y: 100, Z: 100},
	})

	// map-2 not loaded yet.
	_, ok := mm.GetMap("map-2")
	if ok {
		t.Fatal("map-2 should not be loaded yet")
	}

	err := mm.TriggerMapBoundaryPreload(engine.EntityID(1), "map-1", "map-2")
	if err != nil {
		t.Fatalf("TriggerMapBoundaryPreload failed: %v", err)
	}

	// map-2 should now be loaded.
	_, ok = mm.GetMap("map-2")
	if !ok {
		t.Error("expected map-2 to be loaded after preload")
	}
}

func TestTriggerMapBoundaryPreload_AlreadyLoaded(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1"), "map-2": basicMapConfig("map-2")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")
	mm.LoadMap("map-2")

	mm.ConfigureMapBoundary("map-1", "map-2", &MapBoundaryZone{})

	// Should succeed without error since map-2 is already loaded.
	err := mm.TriggerMapBoundaryPreload(engine.EntityID(1), "map-1", "map-2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestTriggerMapBoundaryPreload_NoBoundary(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	err := mm.TriggerMapBoundaryPreload(engine.EntityID(1), "map-1", "map-3")
	if err != ErrBoundaryNotFound {
		t.Errorf("expected ErrBoundaryNotFound, got %v", err)
	}
}

func TestSeamlessMapTransfer(t *testing.T) {
	configs := map[string]*MapConfig{
		"map-1": basicMapConfig("map-1"),
		"map-2": basicMapConfig("map-2"),
	}

	var updatedMapID string
	var updatedPos engine.Vector3
	mm := NewMapManager(
		WithMapLoader(func(id string) (*MapConfig, error) {
			c, ok := configs[id]
			if !ok {
				return nil, ErrMapNotFound
			}
			return c, nil
		}),
		WithPositionUpdater(func(eid engine.EntityID, mapID string, layer int, pos engine.Vector3) error {
			updatedMapID = mapID
			updatedPos = pos
			return nil
		}),
	).(*mapManager)

	mm.LoadMap("map-1")
	mm.LoadMap("map-2")

	mm.ConfigureMapBoundary("map-1", "map-2", &MapBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	})

	err := mm.SeamlessMapTransfer(engine.EntityID(1), "map-1", "map-2")
	if err != nil {
		t.Fatalf("SeamlessMapTransfer failed: %v", err)
	}
	if updatedMapID != "map-2" {
		t.Errorf("expected map-2, got %q", updatedMapID)
	}
	// Target should be center of remote zone: (5, 50, 50).
	if updatedPos.X != 5 || updatedPos.Z != 50 {
		t.Errorf("expected pos (5,50,50), got %v", updatedPos)
	}
}

func TestSeamlessMapTransfer_NoBoundary(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1"), "map-2": basicMapConfig("map-2")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")
	mm.LoadMap("map-2")

	err := mm.SeamlessMapTransfer(engine.EntityID(1), "map-1", "map-2")
	if err != ErrBoundaryNotFound {
		t.Errorf("expected ErrBoundaryNotFound, got %v", err)
	}
}

func TestSeamlessMapTransfer_MapNotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	err := mm.SeamlessMapTransfer(engine.EntityID(1), "nonexistent", "map-2")
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}
}

func TestSeamlessLayerSwitch(t *testing.T) {
	layer0 := simpleLayer(0, 50, 50)
	layer1 := simpleLayer(1, 50, 50)
	layer0.Connections = []LayerConnection{
		{ID: "portal-1", SourceLayer: 0, SourcePos: engine.Vector3{X: 20, Y: 0, Z: 20}, TargetLayer: 1, TargetPos: engine.Vector3{X: 10, Y: 0, Z: 10}},
	}

	var updatedLayer int
	mm := NewMapManager(
		WithMapLoader(func(id string) (*MapConfig, error) {
			return &MapConfig{ID: id, Layers: []*MapLayer{layer0, layer1}}, nil
		}),
		WithPositionUpdater(func(eid engine.EntityID, mapID string, layer int, pos engine.Vector3) error {
			updatedLayer = layer
			return nil
		}),
	).(*mapManager)
	mm.LoadMap("map-1")

	err := mm.SeamlessLayerSwitch(engine.EntityID(1), "map-1", "portal-1")
	if err != nil {
		t.Fatalf("SeamlessLayerSwitch failed: %v", err)
	}
	if updatedLayer != 1 {
		t.Errorf("expected layer 1, got %d", updatedLayer)
	}
}

func TestTriggerLayerPreload(t *testing.T) {
	layer0 := simpleLayer(0, 50, 50)
	layer1 := simpleLayer(1, 50, 50)
	layer0.Connections = []LayerConnection{
		{ID: "stairs-1", SourceLayer: 0, TargetLayer: 1, TargetPos: engine.Vector3{X: 5, Y: 0, Z: 5}},
	}

	configs := map[string]*MapConfig{"map-1": {ID: "map-1", Layers: []*MapLayer{layer0, layer1}}}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	err := mm.TriggerLayerPreload(engine.EntityID(1), "map-1", "stairs-1")
	if err != nil {
		t.Fatalf("TriggerLayerPreload failed: %v", err)
	}
}

func TestTriggerLayerPreload_MapNotFound(t *testing.T) {
	mm := newTestManager(map[string]*MapConfig{})
	err := mm.TriggerLayerPreload(engine.EntityID(1), "nonexistent", "conn-1")
	if err != ErrMapNotFound {
		t.Errorf("expected ErrMapNotFound, got %v", err)
	}
}

func TestTriggerLayerPreload_ConnectionNotFound(t *testing.T) {
	configs := map[string]*MapConfig{"map-1": basicMapConfig("map-1")}
	mm := newTestManager(configs)
	mm.LoadMap("map-1")

	err := mm.TriggerLayerPreload(engine.EntityID(1), "map-1", "nonexistent")
	if err != ErrConnectionNotFound {
		t.Errorf("expected ErrConnectionNotFound, got %v", err)
	}
}
