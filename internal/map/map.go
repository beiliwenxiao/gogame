// Package gamemap implements the Map Manager for the MMRPG game engine.
// It manages map lifecycle (load, unload, hot-reload), multi-layer map structure,
// height maps, teleport points, and layer connections.
package gamemap

import (
	"errors"
	"sync"

	"gogame/internal/engine"
)

// Errors returned by MapManager operations.
var (
	ErrMapExists          = errors.New("map already loaded")
	ErrMapNotFound        = errors.New("map not found")
	ErrLayerNotFound      = errors.New("map layer not found")
	ErrConnectionNotFound = errors.New("layer connection not found")
	ErrTeleportNotFound   = errors.New("teleport point not found")
	ErrNotWalkable        = errors.New("target position is not walkable")
	ErrBoundaryNotFound   = errors.New("map boundary not found")
	ErrMapLoaderNotSet    = errors.New("map loader function not set")
)

// Obstacle represents an obstacle on a map layer.
type Obstacle struct {
	ID       string
	Position engine.Vector3
	Size     engine.Vector3
}

// LayerConnection defines a connection point between two map layers.
type LayerConnection struct {
	ID              string
	SourceLayer     int
	SourcePos       engine.Vector3
	TargetLayer     int
	TargetPos       engine.Vector3
	ConnectionType  string  // cave_entrance / stairs / portal
	PreloadDistance float32
}

// TeleportPoint defines a cross-map teleport point.
type TeleportPoint struct {
	ID        string
	Position  engine.Vector3
	TargetMap string
	TargetPos engine.Vector3
}

// MapBoundaryZone defines the boundary region between two adjacent maps.
type MapBoundaryZone struct {
	NeighborMapID   string
	LocalMin        engine.Vector3
	LocalMax        engine.Vector3
	RemoteMin       engine.Vector3
	RemoteMax       engine.Vector3
	PreloadDistance float32
}

// MapLayer represents a single layer within a map.
type MapLayer struct {
	ID          string
	LayerIndex  int
	HeightMap   [][]float32
	Walkable    [][]bool
	Obstacles   []Obstacle
	Connections []LayerConnection
}

// Map represents a game map with multiple layers.
type Map struct {
	ID         string
	Layers     []*MapLayer
	Teleports  []TeleportPoint
	Boundaries []*MapBoundaryZone

	layerIndex    map[int]*MapLayer          // layerIndex → MapLayer
	connectionMap map[string]*LayerConnection // connectionID → LayerConnection
	teleportMap   map[string]*TeleportPoint   // teleportID → TeleportPoint
	boundaryMap   map[string]*MapBoundaryZone // neighborMapID → boundary
}

// newMap creates a Map and builds internal indexes.
func newMap(id string, layers []*MapLayer, teleports []TeleportPoint, boundaries []*MapBoundaryZone) *Map {
	m := &Map{
		ID:            id,
		Layers:        layers,
		Teleports:     teleports,
		Boundaries:    boundaries,
		layerIndex:    make(map[int]*MapLayer),
		connectionMap: make(map[string]*LayerConnection),
		teleportMap:   make(map[string]*TeleportPoint),
		boundaryMap:   make(map[string]*MapBoundaryZone),
	}
	m.buildIndexes()
	return m
}

func (m *Map) buildIndexes() {
	for _, l := range m.Layers {
		m.layerIndex[l.LayerIndex] = l
		for i := range l.Connections {
			m.connectionMap[l.Connections[i].ID] = &l.Connections[i]
		}
	}
	for i := range m.Teleports {
		m.teleportMap[m.Teleports[i].ID] = &m.Teleports[i]
	}
	for _, bz := range m.Boundaries {
		m.boundaryMap[bz.NeighborMapID] = bz
	}
}

// GetLayer returns the layer at the given index.
func (m *Map) GetLayer(layerIndex int) (*MapLayer, bool) {
	l, ok := m.layerIndex[layerIndex]
	return l, ok
}

// GetConnection returns a layer connection by ID.
func (m *Map) GetConnection(connectionID string) (*LayerConnection, bool) {
	c, ok := m.connectionMap[connectionID]
	return c, ok
}

// GetTeleport returns a teleport point by ID.
func (m *Map) GetTeleport(teleportID string) (*TeleportPoint, bool) {
	tp, ok := m.teleportMap[teleportID]
	return tp, ok
}

// IsWalkable checks if a position is walkable on the given layer.
func (m *Map) IsWalkable(layerIndex int, x, z int) bool {
	layer, ok := m.layerIndex[layerIndex]
	if !ok {
		return false
	}
	if len(layer.Walkable) == 0 {
		return true // no walkable data means all walkable
	}
	if z < 0 || z >= len(layer.Walkable) {
		return false
	}
	if x < 0 || x >= len(layer.Walkable[z]) {
		return false
	}
	return layer.Walkable[z][x]
}

// GetHeight returns the height at a position on the given layer.
func (m *Map) GetHeight(layerIndex int, x, z int) (float32, bool) {
	layer, ok := m.layerIndex[layerIndex]
	if !ok {
		return 0, false
	}
	if len(layer.HeightMap) == 0 {
		return 0, true
	}
	if z < 0 || z >= len(layer.HeightMap) {
		return 0, false
	}
	if x < 0 || x >= len(layer.HeightMap[z]) {
		return 0, false
	}
	return layer.HeightMap[z][x], true
}


// MapConfig holds configuration for loading a map.
type MapConfig struct {
	ID         string
	Layers     []*MapLayer
	Teleports  []TeleportPoint
	Boundaries []*MapBoundaryZone
}

// MapLoader is a function that loads map data by ID.
// In production this would read from files; for testing it can be a simple lookup.
type MapLoader func(mapID string) (*MapConfig, error)

// MapManager manages the lifecycle of game maps.
type MapManager interface {
	LoadMap(mapID string) (*Map, error)
	UnloadMap(mapID string) error
	GetMap(mapID string) (*Map, bool)
	Teleport(entityID engine.EntityID, fromMapID, toMapID string, targetPos engine.Vector3) error
	SwitchLayer(entityID engine.EntityID, mapID string, connectionID string) error
	HotReload(mapID string) error
	// Boundary methods
	ConfigureMapBoundary(mapID, neighborMapID string, zone *MapBoundaryZone) error
	GetMapBoundary(mapID, neighborMapID string) (*MapBoundaryZone, bool)
	// Seamless transition methods
	CheckBoundaryProximity(entityID engine.EntityID, mapID string, entityPos engine.Vector3) (bool, string)
	TriggerMapBoundaryPreload(entityID engine.EntityID, mapID, neighborMapID string) error
	SeamlessMapTransfer(entityID engine.EntityID, fromMapID, toMapID string) error
	SeamlessLayerSwitch(entityID engine.EntityID, mapID string, connectionID string) error
	TriggerLayerPreload(entityID engine.EntityID, mapID string, connectionID string) error
}

// PositionUpdater is a callback to update an entity's position/layer in the ECS world.
// The MapManager doesn't directly depend on ECS; instead the caller provides this callback.
type PositionUpdater func(entityID engine.EntityID, mapID string, layerIndex int, pos engine.Vector3) error

// mapManager is the concrete implementation of MapManager.
type mapManager struct {
	mu              sync.RWMutex
	maps            map[string]*Map
	loader          MapLoader
	positionUpdater PositionUpdater
	onTeleport      func(entityID engine.EntityID, fromMapID, toMapID string) // optional hook
}

// MapManagerOption configures the MapManager.
type MapManagerOption func(*mapManager)

// WithMapLoader sets the map loader function.
func WithMapLoader(loader MapLoader) MapManagerOption {
	return func(m *mapManager) { m.loader = loader }
}

// WithPositionUpdater sets the position updater callback.
func WithPositionUpdater(updater PositionUpdater) MapManagerOption {
	return func(m *mapManager) { m.positionUpdater = updater }
}

// WithTeleportHook sets a callback invoked after a teleport.
func WithTeleportHook(hook func(entityID engine.EntityID, fromMapID, toMapID string)) MapManagerOption {
	return func(m *mapManager) { m.onTeleport = hook }
}

// NewMapManager creates a new MapManager.
func NewMapManager(opts ...MapManagerOption) MapManager {
	mm := &mapManager{
		maps: make(map[string]*Map),
	}
	for _, opt := range opts {
		opt(mm)
	}
	return mm
}

// LoadMap loads a map by ID using the configured loader, or creates from config if loader is nil.
func (mm *mapManager) LoadMap(mapID string) (*Map, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.maps[mapID]; exists {
		return nil, ErrMapExists
	}

	if mm.loader == nil {
		return nil, ErrMapLoaderNotSet
	}

	cfg, err := mm.loader(mapID)
	if err != nil {
		return nil, err
	}

	m := newMap(cfg.ID, cfg.Layers, cfg.Teleports, cfg.Boundaries)
	mm.maps[mapID] = m
	return m, nil
}

// LoadMapFromConfig loads a map directly from a MapConfig (useful for testing).
func (mm *mapManager) LoadMapFromConfig(cfg *MapConfig) (*Map, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.maps[cfg.ID]; exists {
		return nil, ErrMapExists
	}

	m := newMap(cfg.ID, cfg.Layers, cfg.Teleports, cfg.Boundaries)
	mm.maps[cfg.ID] = m
	return m, nil
}

// UnloadMap removes a map from the manager.
func (mm *mapManager) UnloadMap(mapID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.maps[mapID]; !exists {
		return ErrMapNotFound
	}
	delete(mm.maps, mapID)
	return nil
}

// GetMap returns a loaded map by ID.
func (mm *mapManager) GetMap(mapID string) (*Map, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	m, ok := mm.maps[mapID]
	return m, ok
}

// Teleport moves an entity from one map to another via a teleport point.
func (mm *mapManager) Teleport(entityID engine.EntityID, fromMapID, toMapID string, targetPos engine.Vector3) error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	_, ok := mm.maps[fromMapID]
	if !ok {
		return ErrMapNotFound
	}

	toMap, ok := mm.maps[toMapID]
	if !ok {
		return ErrMapNotFound
	}

	// Validate target position is walkable on layer 0 (default layer).
	posX, posZ := int(targetPos.X), int(targetPos.Z)
	if len(toMap.Layers) > 0 {
		if !toMap.IsWalkable(0, posX, posZ) {
			return ErrNotWalkable
		}
	}

	// Update entity position via callback.
	if mm.positionUpdater != nil {
		if err := mm.positionUpdater(entityID, toMapID, 0, targetPos); err != nil {
			return err
		}
	}

	if mm.onTeleport != nil {
		mm.onTeleport(entityID, fromMapID, toMapID)
	}

	return nil
}

// SwitchLayer moves an entity between layers using a LayerConnection.
func (mm *mapManager) SwitchLayer(entityID engine.EntityID, mapID string, connectionID string) error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	m, ok := mm.maps[mapID]
	if !ok {
		return ErrMapNotFound
	}

	conn, ok := m.GetConnection(connectionID)
	if !ok {
		return ErrConnectionNotFound
	}

	// Validate target position is walkable on target layer.
	posX, posZ := int(conn.TargetPos.X), int(conn.TargetPos.Z)
	if !m.IsWalkable(conn.TargetLayer, posX, posZ) {
		return ErrNotWalkable
	}

	// Update entity position via callback.
	if mm.positionUpdater != nil {
		if err := mm.positionUpdater(entityID, mapID, conn.TargetLayer, conn.TargetPos); err != nil {
			return err
		}
	}

	return nil
}

// HotReload reloads a map's data without unloading it.
func (mm *mapManager) HotReload(mapID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	existing, ok := mm.maps[mapID]
	if !ok {
		return ErrMapNotFound
	}

	if mm.loader == nil {
		return ErrMapLoaderNotSet
	}

	cfg, err := mm.loader(mapID)
	if err != nil {
		return err
	}

	// Update the map in-place: replace layers, teleports, boundaries and rebuild indexes.
	existing.Layers = cfg.Layers
	existing.Teleports = cfg.Teleports
	existing.Boundaries = cfg.Boundaries
	existing.layerIndex = make(map[int]*MapLayer)
	existing.connectionMap = make(map[string]*LayerConnection)
	existing.teleportMap = make(map[string]*TeleportPoint)
	existing.boundaryMap = make(map[string]*MapBoundaryZone)
	existing.buildIndexes()

	return nil
}

// ConfigureMapBoundary configures a boundary zone between two maps.
func (mm *mapManager) ConfigureMapBoundary(mapID, neighborMapID string, zone *MapBoundaryZone) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	m, ok := mm.maps[mapID]
	if !ok {
		return ErrMapNotFound
	}

	zone.NeighborMapID = neighborMapID
	m.boundaryMap[neighborMapID] = zone
	m.Boundaries = append(m.Boundaries, zone)
	return nil
}

// GetMapBoundary returns the boundary zone between two maps.
func (mm *mapManager) GetMapBoundary(mapID, neighborMapID string) (*MapBoundaryZone, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	m, ok := mm.maps[mapID]
	if !ok {
		return nil, false
	}

	bz, found := m.boundaryMap[neighborMapID]
	return bz, found
}

// inBounds checks if a position is within an axis-aligned bounding box.
func inBounds(pos, min, max engine.Vector3) bool {
	return pos.X >= min.X && pos.X <= max.X &&
		pos.Y >= min.Y && pos.Y <= max.Y &&
		pos.Z >= min.Z && pos.Z <= max.Z
}

// distToBox returns the minimum distance from a point to an AABB.
func distToBox(pos, min, max engine.Vector3) float32 {
	dx := maxF(min.X-pos.X, 0, pos.X-max.X)
	dz := maxF(min.Z-pos.Z, 0, pos.Z-max.Z)
	return dx + dz // Manhattan distance to box
}

func maxF(vals ...float32) float32 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// CheckBoundaryProximity checks if an entity is within any map boundary's preload distance.
// Returns true and the neighbor map ID if preloading should be triggered.
func (mm *mapManager) CheckBoundaryProximity(entityID engine.EntityID, mapID string, entityPos engine.Vector3) (bool, string) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	m, ok := mm.maps[mapID]
	if !ok {
		return false, ""
	}

	for neighborID, bz := range m.boundaryMap {
		// Check if entity is inside the boundary zone.
		if inBounds(entityPos, bz.LocalMin, bz.LocalMax) {
			return true, neighborID
		}
		// Check if entity is within preload distance of the boundary zone.
		if bz.PreloadDistance > 0 {
			dist := distToBox(entityPos, bz.LocalMin, bz.LocalMax)
			if dist <= bz.PreloadDistance {
				return true, neighborID
			}
		}
	}
	return false, ""
}

// TriggerMapBoundaryPreload ensures the neighbor map is loaded when an entity
// approaches a map boundary. If the neighbor map isn't loaded, it loads it.
func (mm *mapManager) TriggerMapBoundaryPreload(entityID engine.EntityID, mapID, neighborMapID string) error {
	mm.mu.RLock()
	m, ok := mm.maps[mapID]
	if !ok {
		mm.mu.RUnlock()
		return ErrMapNotFound
	}
	_, hasBZ := m.boundaryMap[neighborMapID]
	mm.mu.RUnlock()

	if !hasBZ {
		return ErrBoundaryNotFound
	}

	// Check if neighbor is already loaded.
	mm.mu.RLock()
	_, loaded := mm.maps[neighborMapID]
	mm.mu.RUnlock()

	if loaded {
		return nil // already loaded
	}

	// Load the neighbor map.
	_, err := mm.LoadMap(neighborMapID)
	return err
}

// SeamlessMapTransfer transfers an entity from one map to another via a boundary zone.
// The boundary zone must be configured between the two maps.
func (mm *mapManager) SeamlessMapTransfer(entityID engine.EntityID, fromMapID, toMapID string) error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	fromMap, ok := mm.maps[fromMapID]
	if !ok {
		return ErrMapNotFound
	}

	toMap, ok := mm.maps[toMapID]
	if !ok {
		return ErrMapNotFound
	}

	bz, hasBZ := fromMap.boundaryMap[toMapID]
	if !hasBZ {
		return ErrBoundaryNotFound
	}

	// Calculate target position: map from local boundary zone to remote boundary zone.
	// Use the center of the remote boundary zone as the default target.
	targetPos := engine.Vector3{
		X: (bz.RemoteMin.X + bz.RemoteMax.X) / 2,
		Y: (bz.RemoteMin.Y + bz.RemoteMax.Y) / 2,
		Z: (bz.RemoteMin.Z + bz.RemoteMax.Z) / 2,
	}

	// Validate target is walkable on the target map's default layer.
	if len(toMap.Layers) > 0 {
		if !toMap.IsWalkable(0, int(targetPos.X), int(targetPos.Z)) {
			return ErrNotWalkable
		}
	}

	// Update entity position via callback.
	if mm.positionUpdater != nil {
		if err := mm.positionUpdater(entityID, toMapID, 0, targetPos); err != nil {
			return err
		}
	}

	if mm.onTeleport != nil {
		mm.onTeleport(entityID, fromMapID, toMapID)
	}

	return nil
}

// SeamlessLayerSwitch switches an entity between layers via a LayerConnection
// without requiring a loading screen. Same as SwitchLayer but semantically
// indicates a seamless transition.
func (mm *mapManager) SeamlessLayerSwitch(entityID engine.EntityID, mapID string, connectionID string) error {
	return mm.SwitchLayer(entityID, mapID, connectionID)
}

// TriggerLayerPreload prepares the target layer data when an entity approaches
// a LayerConnection. This is a notification hook — the actual data loading
// depends on the map loader implementation.
func (mm *mapManager) TriggerLayerPreload(entityID engine.EntityID, mapID string, connectionID string) error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	m, ok := mm.maps[mapID]
	if !ok {
		return ErrMapNotFound
	}

	conn, ok := m.GetConnection(connectionID)
	if !ok {
		return ErrConnectionNotFound
	}

	// Verify the target layer exists in the map.
	_, ok = m.GetLayer(conn.TargetLayer)
	if !ok {
		return ErrLayerNotFound
	}

	// In a real implementation, this would trigger async loading of the target
	// layer's detailed data (textures, entities, etc.) and push it to the client.
	// For now, the layer data is already in memory as part of the Map structure.
	return nil
}
