// Package room implements the Room Manager for the MMRPG game engine.
// It manages Room lifecycle, client join/leave, per-room GameLoop and Syncer,
// and idle room auto-destruction.
package room

import (
	"errors"
	"sync"
	"time"

	"gfgame/internal/engine"
)

// Default values.
const (
	DefaultMaxCapacity     = 1000
	DefaultIdleTimeout     = 30 * time.Second
	DefaultTickRate        = 20
	AutoSwitchThreshold    = 50
)

// Errors returned by RoomManager operations.
var (
	ErrRoomExists       = errors.New("room already exists")
	ErrRoomNotFound     = errors.New("room not found")
	ErrRoomFull         = errors.New("room is full")
	ErrAdmissionDenied  = errors.New("admission denied")
	ErrClientNotInRoom  = errors.New("client not in room")
	ErrClientAlreadyIn  = errors.New("client already in room")
)

// Session is a minimal interface for network sessions used by RoomManager.
// This avoids a direct dependency on the network package.
type Session interface {
	ID() string
}

// RoomBoundaryZone defines the boundary region between two adjacent rooms.
type RoomBoundaryZone struct {
	NeighborRoomID string
	LocalMin       engine.Vector3
	LocalMax       engine.Vector3
	RemoteMin      engine.Vector3
	RemoteMax      engine.Vector3
}

// RoomConfig holds configuration for creating a new Room.
type RoomConfig struct {
	ID             string
	SyncMode       engine.SyncMode
	MaxCapacity    int
	IdleTimeout    time.Duration
	TickRate       int
	AdmissionCheck func(Session) bool
	BoundaryZones  []*RoomBoundaryZone
}

// Room represents a game room with its own GameLoop and Syncer.
type Room struct {
	ID            string
	SyncMode      engine.SyncMode
	MaxCapacity   int
	CreatedAt     time.Time
	LastActiveAt  time.Time
	BoundaryZones []*RoomBoundaryZone

	config      RoomConfig
	clients     map[string]Session // sessionID → Session
	boundaryMap map[string]*RoomBoundaryZone // neighborRoomID → zone
	entities    map[engine.EntityID]struct{} // entities in this room
	mu          sync.RWMutex
}

// EntityMigrationState tracks the progress of an entity migration between rooms.
type EntityMigrationState struct {
	EntityID     engine.EntityID
	SourceRoomID string
	TargetRoomID string
	Phase        engine.MigrationPhase
	StartTime    time.Time
}

// ClientCount returns the number of clients in the room.
func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// HasClient checks if a session is in the room.
func (r *Room) HasClient(sessionID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.clients[sessionID]
	return ok
}

// GetClients returns all sessions in the room.
func (r *Room) GetClients() []Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Session, 0, len(r.clients))
	for _, s := range r.clients {
		result = append(result, s)
	}
	return result
}

func (r *Room) touch() {
	r.LastActiveAt = time.Now()
}

// RoomManager manages the lifecycle of game rooms.
type RoomManager interface {
	CreateRoom(config RoomConfig) (*Room, error)
	DestroyRoom(id string) error
	GetRoom(id string) (*Room, bool)
	JoinRoom(roomID string, session Session) error
	LeaveRoom(roomID string, session Session) error
	RoomCount() int
	// Seamless room switch
	ConfigureRoomBoundary(roomID, neighborRoomID string, zone *RoomBoundaryZone) error
	GetRoomBoundary(roomID, neighborRoomID string) (*RoomBoundaryZone, bool)
	SeamlessTransfer(session Session, fromRoomID, toRoomID string) error
	MigrateEntity(entityID engine.EntityID, fromRoomID, toRoomID string) (*EntityMigrationState, error)
	GetMigrationState(entityID engine.EntityID) (*EntityMigrationState, bool)
	GetBoundaryEntities(roomID, neighborRoomID string) []engine.EntityID
}

// roomManager is the concrete implementation of RoomManager.
type roomManager struct {
	mu                sync.RWMutex
	rooms             map[string]*Room
	migrations        map[engine.EntityID]*EntityMigrationState
	stopCh            chan struct{}
	idleCheckInterval time.Duration
}

// NewRoomManager creates a new RoomManager and starts the idle room cleanup goroutine.
func NewRoomManager() RoomManager {
	rm := &roomManager{
		rooms:             make(map[string]*Room),
		migrations:        make(map[engine.EntityID]*EntityMigrationState),
		stopCh:            make(chan struct{}),
		idleCheckInterval: 10 * time.Second,
	}
	go rm.idleCheckLoop()
	return rm
}

// Stop terminates the idle-check goroutine.
func (rm *roomManager) Stop() {
	close(rm.stopCh)
}

// CreateRoom creates a new room with the given configuration.
func (rm *roomManager) CreateRoom(config RoomConfig) (*Room, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.rooms[config.ID]; exists {
		return nil, ErrRoomExists
	}

	// Apply defaults.
	if config.MaxCapacity <= 0 {
		if config.SyncMode == engine.SyncModeState {
			config.MaxCapacity = DefaultMaxCapacity // 1000 for state sync
		} else {
			config.MaxCapacity = AutoSwitchThreshold // smaller for lockstep
		}
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = DefaultIdleTimeout
	}
	if config.TickRate <= 0 {
		config.TickRate = DefaultTickRate
	}

	now := time.Now()
	r := &Room{
		ID:            config.ID,
		SyncMode:      config.SyncMode,
		MaxCapacity:   config.MaxCapacity,
		CreatedAt:     now,
		LastActiveAt:  now,
		BoundaryZones: config.BoundaryZones,
		config:        config,
		clients:       make(map[string]Session),
		boundaryMap:   make(map[string]*RoomBoundaryZone),
		entities:      make(map[engine.EntityID]struct{}),
	}

	// Index boundary zones.
	for _, bz := range config.BoundaryZones {
		if bz != nil {
			r.boundaryMap[bz.NeighborRoomID] = bz
		}
	}

	rm.rooms[config.ID] = r
	return r, nil
}

// DestroyRoom removes a room.
func (rm *roomManager) DestroyRoom(id string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.rooms[id]; !exists {
		return ErrRoomNotFound
	}
	delete(rm.rooms, id)
	return nil
}

// GetRoom returns a room by ID.
func (rm *roomManager) GetRoom(id string) (*Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	r, ok := rm.rooms[id]
	return r, ok
}

// JoinRoom adds a client session to a room after validating capacity and admission.
func (rm *roomManager) JoinRoom(roomID string, session Session) error {
	rm.mu.RLock()
	r, ok := rm.rooms[roomID]
	rm.mu.RUnlock()

	if !ok {
		return ErrRoomNotFound
	}

	// Admission check.
	if r.config.AdmissionCheck != nil && !r.config.AdmissionCheck(session) {
		return ErrAdmissionDenied
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[session.ID()]; exists {
		return ErrClientAlreadyIn
	}

	if len(r.clients) >= r.MaxCapacity {
		return ErrRoomFull
	}

	r.clients[session.ID()] = session
	r.touch()
	return nil
}

// LeaveRoom removes a client session from a room.
func (rm *roomManager) LeaveRoom(roomID string, session Session) error {
	rm.mu.RLock()
	r, ok := rm.rooms[roomID]
	rm.mu.RUnlock()

	if !ok {
		return ErrRoomNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[session.ID()]; !exists {
		return ErrClientNotInRoom
	}

	delete(r.clients, session.ID())
	r.touch()
	return nil
}

// RoomCount returns the number of active rooms.
func (rm *roomManager) RoomCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.rooms)
}

// idleCheckLoop periodically checks for idle rooms and destroys them.
func (rm *roomManager) idleCheckLoop() {
	ticker := time.NewTicker(rm.idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.stopCh:
			return
		case <-ticker.C:
			rm.checkIdleRooms()
		}
	}
}

// checkIdleRooms destroys rooms with no clients that have exceeded idle timeout.
func (rm *roomManager) checkIdleRooms() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	var toDestroy []string

	for id, r := range rm.rooms {
		r.mu.RLock()
		clientCount := len(r.clients)
		lastActive := r.LastActiveAt
		idleTimeout := r.config.IdleTimeout
		r.mu.RUnlock()

		if clientCount == 0 && now.Sub(lastActive) > idleTimeout {
			toDestroy = append(toDestroy, id)
		}
	}

	for _, id := range toDestroy {
		delete(rm.rooms, id)
	}
}


// ---------- Seamless Room Switch & Entity Migration ----------

// ConfigureRoomBoundary configures a boundary zone between two rooms.
func (rm *roomManager) ConfigureRoomBoundary(roomID, neighborRoomID string, zone *RoomBoundaryZone) error {
	rm.mu.RLock()
	r, ok := rm.rooms[roomID]
	rm.mu.RUnlock()

	if !ok {
		return ErrRoomNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	zone.NeighborRoomID = neighborRoomID
	r.boundaryMap[neighborRoomID] = zone
	r.BoundaryZones = append(r.BoundaryZones, zone)
	return nil
}

// GetRoomBoundary returns the boundary zone between two rooms.
func (rm *roomManager) GetRoomBoundary(roomID, neighborRoomID string) (*RoomBoundaryZone, bool) {
	rm.mu.RLock()
	r, ok := rm.rooms[roomID]
	rm.mu.RUnlock()

	if !ok {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	bz, found := r.boundaryMap[neighborRoomID]
	return bz, found
}

// SeamlessTransfer moves a client session from one room to another without
// disconnecting. The session is added to the target room first, then removed
// from the source room ("add-before-remove" strategy).
func (rm *roomManager) SeamlessTransfer(session Session, fromRoomID, toRoomID string) error {
	rm.mu.RLock()
	fromRoom, fromOK := rm.rooms[fromRoomID]
	toRoom, toOK := rm.rooms[toRoomID]
	rm.mu.RUnlock()

	if !fromOK || !toOK {
		return ErrRoomNotFound
	}

	// Admission check on target.
	if toRoom.config.AdmissionCheck != nil && !toRoom.config.AdmissionCheck(session) {
		return ErrAdmissionDenied
	}

	// Add to target first.
	toRoom.mu.Lock()
	if len(toRoom.clients) >= toRoom.MaxCapacity {
		toRoom.mu.Unlock()
		return ErrRoomFull
	}
	toRoom.clients[session.ID()] = session
	toRoom.touch()
	toRoom.mu.Unlock()

	// Remove from source.
	fromRoom.mu.Lock()
	delete(fromRoom.clients, session.ID())
	fromRoom.touch()
	fromRoom.mu.Unlock()

	return nil
}

// MigrateEntity migrates an entity from one room to another using the
// "add-before-remove" strategy with a state machine tracking progress.
func (rm *roomManager) MigrateEntity(entityID engine.EntityID, fromRoomID, toRoomID string) (*EntityMigrationState, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	fromRoom, fromOK := rm.rooms[fromRoomID]
	toRoom, toOK := rm.rooms[toRoomID]
	if !fromOK || !toOK {
		return nil, ErrRoomNotFound
	}

	// Phase 1: Prepare — verify entity is in source room.
	fromRoom.mu.Lock()
	if _, exists := fromRoom.entities[entityID]; !exists {
		fromRoom.mu.Unlock()
		return nil, ErrClientNotInRoom
	}
	fromRoom.mu.Unlock()

	state := &EntityMigrationState{
		EntityID:     entityID,
		SourceRoomID: fromRoomID,
		TargetRoomID: toRoomID,
		Phase:        engine.MigrationPrepare,
		StartTime:    time.Now(),
	}
	rm.migrations[entityID] = state

	// Phase 2: Transfer — add entity to target room first.
	state.Phase = engine.MigrationTransfer
	toRoom.mu.Lock()
	toRoom.entities[entityID] = struct{}{}
	toRoom.touch()
	toRoom.mu.Unlock()

	// Phase 3: Confirm — target has the entity.
	state.Phase = engine.MigrationConfirm

	// Phase 4: Cleanup — remove from source room.
	state.Phase = engine.MigrationCleanup
	fromRoom.mu.Lock()
	delete(fromRoom.entities, entityID)
	fromRoom.touch()
	fromRoom.mu.Unlock()

	// Phase 5: Complete.
	state.Phase = engine.MigrationComplete
	return state, nil
}

// GetMigrationState returns the current migration state for an entity.
func (rm *roomManager) GetMigrationState(entityID engine.EntityID) (*EntityMigrationState, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	state, ok := rm.migrations[entityID]
	return state, ok
}

// GetBoundaryEntities returns entity IDs in the boundary zone of a room
// that faces the given neighbor room.
func (rm *roomManager) GetBoundaryEntities(roomID, neighborRoomID string) []engine.EntityID {
	rm.mu.RLock()
	r, ok := rm.rooms[roomID]
	rm.mu.RUnlock()

	if !ok {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, hasBZ := r.boundaryMap[neighborRoomID]
	if !hasBZ {
		return nil
	}

	// Return all entities in the room (in a full implementation, we'd filter
	// by position within the boundary zone, but entity positions are managed
	// by the Scene/ECS layer, not the Room layer directly).
	result := make([]engine.EntityID, 0, len(r.entities))
	for eid := range r.entities {
		result = append(result, eid)
	}
	return result
}

// AddEntity registers an entity in a room (used for migration tracking).
func (rm *roomManager) AddEntity(roomID string, entityID engine.EntityID) error {
	rm.mu.RLock()
	r, ok := rm.rooms[roomID]
	rm.mu.RUnlock()

	if !ok {
		return ErrRoomNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.entities[entityID] = struct{}{}
	return nil
}
