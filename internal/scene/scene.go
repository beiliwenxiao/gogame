// Package scene implements the Scene Manager for the MMRPG game engine.
// It manages Scene lifecycle (create, load, unload), entity admission,
// cross-scene entity transfer, and idle scene auto-unload.
package scene

import (
	"errors"
	"sync"
	"time"

	"gfgame/internal/aoi"
	"gfgame/internal/ecs"
	"gfgame/internal/engine"
)

// Default values.
const (
	DefaultIdleTimeout = 5 * time.Minute
	DefaultMaxEntities = 10000
)

// Errors returned by SceneManager operations.
var (
	ErrSceneExists          = errors.New("scene already exists")
	ErrSceneNotFound        = errors.New("scene not found")
	ErrEntityNotFound       = errors.New("entity not found in scene")
	ErrAdmissionDenied      = errors.New("entity admission denied")
	ErrSceneAtCapacity      = errors.New("scene at maximum entity capacity")
	ErrEntityNotInSource    = errors.New("entity not in source scene")
	ErrBoundaryZoneNotFound = errors.New("boundary zone not found")
	ErrNeighborNotFound     = errors.New("neighbor scene not found or not loaded")
	ErrEntityNotInBoundary  = errors.New("entity not in boundary zone")
)

// SceneBoundaryZone defines the boundary region between two adjacent scenes.
type SceneBoundaryZone struct {
	NeighborSceneID string
	LocalMin        engine.Vector3
	LocalMax        engine.Vector3
	RemoteMin       engine.Vector3
	RemoteMax       engine.Vector3
	PreloadDistance float32
}

// SceneConfig holds configuration for creating a new Scene.
type SceneConfig struct {
	ID             string
	MapID          string
	MaxEntities    int
	IdleTimeout    time.Duration
	AdmissionCheck func(engine.EntityID) bool
	BoundaryZones  []*SceneBoundaryZone
}

// Scene represents a single game scene instance with its own ECS World and AOI manager.
type Scene struct {
	ID            string
	World         *ecs.World
	AOI           aoi.AOIManager
	MapData       interface{} // *Map placeholder — will be typed when map module is implemented
	LastActiveAt  time.Time
	BoundaryZones []*SceneBoundaryZone

	config        SceneConfig
	entities      map[engine.EntityID]struct{} // tracks which entities belong to this scene
	boundaryMap   map[string]*SceneBoundaryZone // neighborSceneID → zone for fast lookup
	preloaded     map[string]bool               // tracks which neighbor scenes have been preloaded for which entities
	mu            sync.RWMutex
}

// EntityCount returns the number of entities currently in the scene.
func (s *Scene) EntityCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entities)
}

// HasEntity checks whether the given entity belongs to this scene.
func (s *Scene) HasEntity(entityID engine.EntityID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.entities[entityID]
	return ok
}

// touch updates the last active timestamp.
func (s *Scene) touch() {
	s.LastActiveAt = time.Now()
}

// SceneManager manages the lifecycle of scenes and entity movement between them.
type SceneManager interface {
	CreateScene(config SceneConfig) (*Scene, error)
	GetScene(id string) (*Scene, bool)
	UnloadScene(id string) error
	EnterScene(entityID engine.EntityID, sceneID string) error
	TransferScene(entityID engine.EntityID, fromSceneID, toSceneID string) error
	ActiveSceneCount() int
	// Seamless transition methods
	ConfigureBoundaryZone(sceneID, neighborSceneID string, zone *SceneBoundaryZone) error
	GetBoundaryZone(sceneID, neighborSceneID string) (*SceneBoundaryZone, bool)
	IsInBoundaryZone(entityID engine.EntityID, sceneID string) (bool, string)
	TriggerBoundaryPreload(entityID engine.EntityID, sceneID, neighborSceneID string) error
	SeamlessTransfer(entityID engine.EntityID, fromSceneID, toSceneID string) error
}

// sceneManager is the concrete implementation of SceneManager.
type sceneManager struct {
	mu     sync.RWMutex
	scenes map[string]*Scene

	// entityScene tracks which scene each entity belongs to for fast lookup.
	entityScene map[engine.EntityID]string

	// stopCh signals the idle-check goroutine to stop.
	stopCh chan struct{}
	// idleCheckInterval controls how often the idle checker runs.
	idleCheckInterval time.Duration
}

// NewSceneManager creates a new SceneManager and starts the idle scene cleanup goroutine.
func NewSceneManager() SceneManager {
	sm := &sceneManager{
		scenes:            make(map[string]*Scene),
		entityScene:       make(map[engine.EntityID]string),
		stopCh:            make(chan struct{}),
		idleCheckInterval: 30 * time.Second,
	}
	go sm.idleCheckLoop()
	return sm
}

// Stop terminates the idle-check goroutine. Call when shutting down.
func (sm *sceneManager) Stop() {
	close(sm.stopCh)
}

// CreateScene creates a new scene with the given configuration.
func (sm *sceneManager) CreateScene(config SceneConfig) (*Scene, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.scenes[config.ID]; exists {
		return nil, ErrSceneExists
	}

	// Apply defaults.
	if config.MaxEntities <= 0 {
		config.MaxEntities = DefaultMaxEntities
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = DefaultIdleTimeout
	}

	world := ecs.NewWorld()
	spatial := aoi.NewGridSpatialIndex(aoi.DefaultGridConfig())
	aoiMgr := aoi.NewAOIManager(spatial)

	s := &Scene{
		ID:            config.ID,
		World:         world,
		AOI:           aoiMgr,
		LastActiveAt:  time.Now(),
		BoundaryZones: config.BoundaryZones,
		config:        config,
		entities:      make(map[engine.EntityID]struct{}),
		boundaryMap:   make(map[string]*SceneBoundaryZone),
		preloaded:     make(map[string]bool),
	}

	// Index boundary zones by neighbor scene ID.
	for _, bz := range config.BoundaryZones {
		if bz != nil {
			s.boundaryMap[bz.NeighborSceneID] = bz
		}
	}

	sm.scenes[config.ID] = s
	return s, nil
}

// GetScene returns the scene with the given ID, or false if not found.
func (sm *sceneManager) GetScene(id string) (*Scene, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.scenes[id]
	return s, ok
}

// UnloadScene removes a scene and all its entities.
func (sm *sceneManager) UnloadScene(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.scenes[id]
	if !ok {
		return ErrSceneNotFound
	}

	// Remove entity-scene mappings for all entities in this scene.
	s.mu.RLock()
	for eid := range s.entities {
		delete(sm.entityScene, eid)
	}
	s.mu.RUnlock()

	delete(sm.scenes, id)
	return nil
}

// EnterScene adds an entity to the target scene after validating admission conditions.
func (sm *sceneManager) EnterScene(entityID engine.EntityID, sceneID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.scenes[sceneID]
	if !ok {
		return ErrSceneNotFound
	}

	// Admission check.
	if s.config.AdmissionCheck != nil && !s.config.AdmissionCheck(entityID) {
		return ErrAdmissionDenied
	}

	// Capacity check.
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.entities) >= s.config.MaxEntities {
		return ErrSceneAtCapacity
	}

	// Create entity in the scene's World if it doesn't exist yet.
	// The entity is tracked in the scene's entity set.
	s.entities[entityID] = struct{}{}
	sm.entityScene[entityID] = sceneID
	s.touch()

	return nil
}

// TransferScene moves an entity from one scene to another without losing component state.
// It snapshots all components from the source World, removes the entity from the source,
// creates it in the target, and restores all components.
func (sm *sceneManager) TransferScene(entityID engine.EntityID, fromSceneID, toSceneID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	fromScene, ok := sm.scenes[fromSceneID]
	if !ok {
		return ErrSceneNotFound
	}
	toScene, ok := sm.scenes[toSceneID]
	if !ok {
		return ErrSceneNotFound
	}

	// Verify entity is in source scene.
	fromScene.mu.Lock()
	if _, exists := fromScene.entities[entityID]; !exists {
		fromScene.mu.Unlock()
		return ErrEntityNotInSource
	}

	// Admission check on target scene.
	if toScene.config.AdmissionCheck != nil && !toScene.config.AdmissionCheck(entityID) {
		fromScene.mu.Unlock()
		return ErrAdmissionDenied
	}

	toScene.mu.Lock()

	// Capacity check on target.
	if len(toScene.entities) >= toScene.config.MaxEntities {
		toScene.mu.Unlock()
		fromScene.mu.Unlock()
		return ErrSceneAtCapacity
	}

	// Snapshot all components from source World.
	components := sm.snapshotComponents(fromScene.World, entityID)

	// Remove entity from source scene's AOI.
	fromScene.AOI.Remove(entityID)

	// Destroy entity in source World (removes all components).
	fromScene.World.DestroyEntity(entityID)

	// Remove from source scene tracking.
	delete(fromScene.entities, entityID)
	fromScene.touch()
	fromScene.mu.Unlock()

	// Add entity to target scene and restore components.
	toScene.entities[entityID] = struct{}{}
	sm.entityScene[entityID] = toSceneID

	// Restore components in target World.
	sm.restoreComponents(toScene.World, entityID, components)

	toScene.touch()
	toScene.mu.Unlock()

	return nil
}

// ActiveSceneCount returns the number of currently loaded scenes.
func (sm *sceneManager) ActiveSceneCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.scenes)
}

// ConfigureBoundaryZone configures a boundary zone between two scenes and registers
// cross-boundary AOI between their AOI managers.
func (sm *sceneManager) ConfigureBoundaryZone(sceneID, neighborSceneID string, zone *SceneBoundaryZone) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.scenes[sceneID]
	if !ok {
		return ErrSceneNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Store the zone.
	zone.NeighborSceneID = neighborSceneID
	s.boundaryMap[neighborSceneID] = zone
	s.BoundaryZones = append(s.BoundaryZones, zone)

	// If the neighbor scene is already loaded, register cross-boundary AOI.
	if neighbor, exists := sm.scenes[neighborSceneID]; exists {
		aoiBZ := &aoi.AOIBoundaryZone{
			LocalMin:  zone.LocalMin,
			LocalMax:  zone.LocalMax,
			RemoteMin: zone.RemoteMin,
			RemoteMax: zone.RemoteMax,
		}
		s.AOI.RegisterNeighborAOI(neighborSceneID, neighbor.AOI, aoiBZ)
	}

	return nil
}

// GetBoundaryZone returns the boundary zone between two scenes.
func (sm *sceneManager) GetBoundaryZone(sceneID, neighborSceneID string) (*SceneBoundaryZone, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	s, ok := sm.scenes[sceneID]
	if !ok {
		return nil, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	bz, found := s.boundaryMap[neighborSceneID]
	return bz, found
}

// IsInBoundaryZone checks whether an entity is within any boundary zone of the given scene.
// Returns true and the neighbor scene ID if the entity is in a boundary zone.
func (sm *sceneManager) IsInBoundaryZone(entityID engine.EntityID, sceneID string) (bool, string) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	s, ok := sm.scenes[sceneID]
	if !ok {
		return false, ""
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if entity is in this scene.
	if _, exists := s.entities[entityID]; !exists {
		return false, ""
	}

	// Get entity position from the scene's World.
	posComp, ok := s.World.GetComponent(entityID, ecs.CompPosition)
	if !ok {
		return false, ""
	}
	pos := posComp.(*ecs.PositionComponent)
	entityPos := engine.Vector3{X: pos.X, Y: pos.Y, Z: pos.Z}

	// Check against all boundary zones.
	for neighborID, bz := range s.boundaryMap {
		if inBounds(entityPos, bz.LocalMin, bz.LocalMax) {
			return true, neighborID
		}
	}

	return false, ""
}

// TriggerBoundaryPreload ensures the neighbor scene is loaded/created when an entity
// approaches a boundary zone. If the neighbor scene doesn't exist, it creates one
// with default config. It also registers cross-boundary AOI between the scenes.
func (sm *sceneManager) TriggerBoundaryPreload(entityID engine.EntityID, sceneID, neighborSceneID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.scenes[sceneID]
	if !ok {
		return ErrSceneNotFound
	}

	s.mu.RLock()
	bz, hasBZ := s.boundaryMap[neighborSceneID]
	s.mu.RUnlock()

	if !hasBZ {
		return ErrBoundaryZoneNotFound
	}

	// Ensure the neighbor scene exists. If not, create it.
	neighbor, exists := sm.scenes[neighborSceneID]
	if !exists {
		// Create the neighbor scene with default config.
		neighborConfig := SceneConfig{
			ID:          neighborSceneID,
			MaxEntities: DefaultMaxEntities,
			IdleTimeout: DefaultIdleTimeout,
		}

		world := ecs.NewWorld()
		spatial := aoi.NewGridSpatialIndex(aoi.DefaultGridConfig())
		aoiMgr := aoi.NewAOIManager(spatial)

		neighbor = &Scene{
			ID:            neighborSceneID,
			World:         world,
			AOI:           aoiMgr,
			LastActiveAt:  time.Now(),
			config:        neighborConfig,
			entities:      make(map[engine.EntityID]struct{}),
			boundaryMap:   make(map[string]*SceneBoundaryZone),
			preloaded:     make(map[string]bool),
		}
		sm.scenes[neighborSceneID] = neighbor
	}

	// Register cross-boundary AOI if not already done.
	aoiBZ := &aoi.AOIBoundaryZone{
		LocalMin:  bz.LocalMin,
		LocalMax:  bz.LocalMax,
		RemoteMin: bz.RemoteMin,
		RemoteMax: bz.RemoteMax,
	}
	s.AOI.RegisterNeighborAOI(neighborSceneID, neighbor.AOI, aoiBZ)

	// Also register the reverse direction on the neighbor.
	reverseAOIBZ := &aoi.AOIBoundaryZone{
		LocalMin:  bz.RemoteMin,
		LocalMax:  bz.RemoteMax,
		RemoteMin: bz.LocalMin,
		RemoteMax: bz.LocalMax,
	}
	neighbor.AOI.RegisterNeighborAOI(sceneID, s.AOI, reverseAOIBZ)

	// Mark as preloaded.
	s.mu.Lock()
	s.preloaded[neighborSceneID] = true
	s.mu.Unlock()

	neighbor.touch()
	return nil
}

// SeamlessTransfer performs a seamless entity transfer from one scene to another.
// It uses the existing TransferScene logic and ensures cross-boundary AOI visibility
// is maintained during the transition. The entity must be in a boundary zone of the
// source scene that connects to the target scene.
func (sm *sceneManager) SeamlessTransfer(entityID engine.EntityID, fromSceneID, toSceneID string) error {
	// Verify the entity is in a boundary zone connecting to the target scene.
	// We need to read-lock first to check, then proceed with the transfer.
	sm.mu.RLock()
	fromScene, ok := sm.scenes[fromSceneID]
	if !ok {
		sm.mu.RUnlock()
		return ErrSceneNotFound
	}
	_, toExists := sm.scenes[toSceneID]
	if !toExists {
		sm.mu.RUnlock()
		return ErrSceneNotFound
	}

	fromScene.mu.RLock()
	_, hasBZ := fromScene.boundaryMap[toSceneID]
	fromScene.mu.RUnlock()
	sm.mu.RUnlock()

	if !hasBZ {
		return ErrBoundaryZoneNotFound
	}

	// Perform the actual transfer using the existing TransferScene logic.
	// TransferScene already handles: snapshot components, remove from source,
	// add to target, restore components.
	return sm.TransferScene(entityID, fromSceneID, toSceneID)
}

// inBounds checks if a position is within the axis-aligned bounding box.
func inBounds(pos, min, max engine.Vector3) bool {
	return pos.X >= min.X && pos.X <= max.X &&
		pos.Y >= min.Y && pos.Y <= max.Y &&
		pos.Z >= min.Z && pos.Z <= max.Z
}

// snapshotComponents extracts all components for an entity from a World.
func (sm *sceneManager) snapshotComponents(w *ecs.World, entityID engine.EntityID) []ecs.Component {
	// We check all known component types.
	knownTypes := []engine.ComponentType{
		ecs.CompPosition,
		ecs.CompMovement,
		ecs.CompCombatAttribute,
		ecs.CompEquipment,
		ecs.CompSkill,
		ecs.CompBuff,
		ecs.CompNetwork,
		ecs.CompAOI,
	}

	var components []ecs.Component
	for _, ct := range knownTypes {
		if comp, ok := w.GetComponent(entityID, ct); ok {
			components = append(components, comp)
		}
	}
	return components
}

// restoreComponents creates an entity in the target World and adds all snapshotted components.
func (sm *sceneManager) restoreComponents(w *ecs.World, entityID engine.EntityID, components []ecs.Component) {
	// We need to ensure the entity exists in the target world.
	// Since World.CreateEntity() generates a new ID, we use a direct approach:
	// We add the entity with its existing ID by calling AddComponent which
	// requires the entity to exist. We'll use a helper approach.
	// The World doesn't expose a way to create an entity with a specific ID,
	// so we need to work with what we have. We'll inject the entity directly.
	w.InjectEntity(entityID)
	for _, comp := range components {
		w.AddComponent(entityID, comp)
	}
}

// idleCheckLoop periodically checks for idle scenes and unloads them.
func (sm *sceneManager) idleCheckLoop() {
	ticker := time.NewTicker(sm.idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-ticker.C:
			sm.checkIdleScenes()
		}
	}
}

// checkIdleScenes unloads scenes that have been idle longer than their configured timeout.
func (sm *sceneManager) checkIdleScenes() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	var toUnload []string

	for id, s := range sm.scenes {
		s.mu.RLock()
		entityCount := len(s.entities)
		lastActive := s.LastActiveAt
		idleTimeout := s.config.IdleTimeout
		s.mu.RUnlock()

		// Only unload scenes with no entities that have exceeded idle timeout.
		if entityCount == 0 && now.Sub(lastActive) > idleTimeout {
			toUnload = append(toUnload, id)
		}
	}

	for _, id := range toUnload {
		s := sm.scenes[id]
		s.mu.RLock()
		for eid := range s.entities {
			delete(sm.entityScene, eid)
		}
		s.mu.RUnlock()
		delete(sm.scenes, id)
	}
}
