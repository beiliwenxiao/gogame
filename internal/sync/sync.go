// Package sync provides synchronization modules for the MMRPG game engine,
// supporting both lockstep (frame) synchronization and authoritative state synchronization.
package sync

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"gfgame/internal/engine"
	"gfgame/internal/network"
)

// ---------------------------------------------------------------------------
// Syncer interface – unified sync abstraction
// ---------------------------------------------------------------------------

// Syncer is the unified synchronization interface. Upper layers interact with
// this interface without knowing whether lockstep or state sync is in use.
type Syncer interface {
	Mode() engine.SyncMode
	OnInput(session network.Session, input engine.PlayerInput)
	OnTick(tick uint64)
	OnReconnect(session network.Session)
	// AddSession registers a client session with the syncer.
	AddSession(session network.Session)
	// RemoveSession unregisters a client session.
	RemoveSession(sessionID string)
	// SessionCount returns the number of active sessions.
	SessionCount() int
}

// LockstepSyncer extends Syncer with lockstep-specific capabilities.
type LockstepSyncer interface {
	Syncer
	HistoryFrames() int
}

// StateSyncer extends Syncer with state-sync-specific capabilities.
type StateSyncer interface {
	Syncer
	SnapshotInterval() int
}

// ---------------------------------------------------------------------------
// SyncManagerConfig
// ---------------------------------------------------------------------------

// SyncManagerConfig holds configuration for the SyncManager.
type SyncManagerConfig struct {
	DefaultMode         engine.SyncMode
	LockstepTimeout     time.Duration // default 100ms
	HistoryFrames       int           // default 300
	SnapshotInterval    int           // default 100
	AutoSwitchThreshold int           // default 50
}

// DefaultSyncManagerConfig returns a SyncManagerConfig with sensible defaults.
func DefaultSyncManagerConfig() SyncManagerConfig {
	return SyncManagerConfig{
		DefaultMode:         engine.SyncModeLockstep,
		LockstepTimeout:     100 * time.Millisecond,
		HistoryFrames:       300,
		SnapshotInterval:    100,
		AutoSwitchThreshold: 50,
	}
}

// ---------------------------------------------------------------------------
// FrameData – a single lockstep frame
// ---------------------------------------------------------------------------

// FrameData represents a single lockstep frame containing all player inputs.
type FrameData struct {
	FrameID uint64               `json:"frame_id"`
	Inputs  []engine.PlayerInput `json:"inputs"`
}

// ---------------------------------------------------------------------------
// lockstepSyncer implementation
// ---------------------------------------------------------------------------

type lockstepSyncer struct {
	mu       sync.RWMutex
	timeout  time.Duration
	maxHist  int
	sessions map[string]network.Session // sessionID → Session

	// Per-tick input collection
	pendingInputs []engine.PlayerInput

	// History ring buffer
	history    []FrameData
	histHead   int // next write position
	histCount  int // number of frames stored
	lastFrame  uint64
}

// NewLockstepSyncer creates a new lockstep synchronizer.
func NewLockstepSyncer(timeout time.Duration, historyFrames int) LockstepSyncer {
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	if historyFrames <= 0 {
		historyFrames = 300
	}
	return &lockstepSyncer{
		timeout:  timeout,
		maxHist:  historyFrames,
		sessions: make(map[string]network.Session),
		history:  make([]FrameData, historyFrames),
	}
}

func (ls *lockstepSyncer) Mode() engine.SyncMode { return engine.SyncModeLockstep }

func (ls *lockstepSyncer) HistoryFrames() int { return ls.maxHist }

func (ls *lockstepSyncer) AddSession(session network.Session) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.sessions[session.ID()] = session
}

func (ls *lockstepSyncer) RemoveSession(sessionID string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	delete(ls.sessions, sessionID)
}

func (ls *lockstepSyncer) SessionCount() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return len(ls.sessions)
}

// OnInput collects a player input for the current tick.
func (ls *lockstepSyncer) OnInput(session network.Session, input engine.PlayerInput) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.pendingInputs = append(ls.pendingInputs, input)
}

// OnTick packages all collected inputs into a frame, stores it in history,
// and broadcasts it to all sessions.
func (ls *lockstepSyncer) OnTick(tick uint64) {
	ls.mu.Lock()

	// Build frame data from collected inputs
	frame := FrameData{
		FrameID: tick,
		Inputs:  ls.pendingInputs,
	}
	if frame.Inputs == nil {
		frame.Inputs = []engine.PlayerInput{}
	}

	// Store in ring buffer
	ls.history[ls.histHead] = frame
	ls.histHead = (ls.histHead + 1) % ls.maxHist
	if ls.histCount < ls.maxHist {
		ls.histCount++
	}
	ls.lastFrame = tick

	// Reset pending inputs for next tick
	ls.pendingInputs = nil

	// Snapshot sessions for broadcast (avoid holding lock during I/O)
	sessions := make([]network.Session, 0, len(ls.sessions))
	for _, s := range ls.sessions {
		sessions = append(sessions, s)
	}
	ls.mu.Unlock()

	// Serialize and broadcast
	data, err := json.Marshal(frame)
	if err != nil {
		log.Printf("[sync] lockstep: failed to marshal frame %d: %v", tick, err)
		return
	}
	for _, s := range sessions {
		if sendErr := s.Send(data); sendErr != nil {
			log.Printf("[sync] lockstep: failed to send frame %d to session %s: %v", tick, s.ID(), sendErr)
		}
	}
}

// OnReconnect sends all missing history frames to the reconnecting session.
func (ls *lockstepSyncer) OnReconnect(session network.Session) {
	ls.mu.RLock()
	frames := ls.getHistoryFramesLocked()
	ls.mu.RUnlock()

	for _, frame := range frames {
		data, err := json.Marshal(frame)
		if err != nil {
			log.Printf("[sync] lockstep: reconnect marshal error: %v", err)
			continue
		}
		if sendErr := session.Send(data); sendErr != nil {
			log.Printf("[sync] lockstep: reconnect send error to %s: %v", session.ID(), sendErr)
			return
		}
	}
}

// getHistoryFramesLocked returns stored history frames in chronological order.
// Caller must hold at least a read lock.
func (ls *lockstepSyncer) getHistoryFramesLocked() []FrameData {
	if ls.histCount == 0 {
		return nil
	}
	result := make([]FrameData, 0, ls.histCount)
	start := (ls.histHead - ls.histCount + ls.maxHist) % ls.maxHist
	for i := 0; i < ls.histCount; i++ {
		idx := (start + i) % ls.maxHist
		result = append(result, ls.history[idx])
	}
	return result
}

// GetHistoryFrom returns history frames starting from the given frame ID (inclusive).
// This is used for reconnection to send only the missing frames.
func (ls *lockstepSyncer) GetHistoryFrom(fromFrame uint64) []FrameData {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	all := ls.getHistoryFramesLocked()
	for i, f := range all {
		if f.FrameID >= fromFrame {
			return all[i:]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// EntityState – tracked entity state for delta computation
// ---------------------------------------------------------------------------

// EntityState represents the state of a single entity at a point in time.
type EntityState struct {
	EntityID   engine.EntityID    `json:"entity_id"`
	Properties map[string]float64 `json:"properties"`
}

// Clone returns a deep copy of the EntityState.
func (es EntityState) Clone() EntityState {
	props := make(map[string]float64, len(es.Properties))
	for k, v := range es.Properties {
		props[k] = v
	}
	return EntityState{
		EntityID:   es.EntityID,
		Properties: props,
	}
}

// EntityDelta represents the changes to a single entity between two ticks.
type EntityDelta struct {
	EntityID engine.EntityID    `json:"entity_id"`
	Changed  map[string]float64 `json:"changed,omitempty"`
	Removed  []string           `json:"removed,omitempty"`
}

// StateDelta represents all changes in a single tick.
type StateDelta struct {
	Tick           uint64        `json:"tick"`
	EntityDeltas   []EntityDelta `json:"entity_deltas,omitempty"`
	RemovedEntities []engine.EntityID `json:"removed_entities,omitempty"`
}

// StateSnapshot represents a complete state snapshot at a given tick.
type StateSnapshot struct {
	Tick     uint64        `json:"tick"`
	Entities []EntityState `json:"entities"`
}

// ---------------------------------------------------------------------------
// AOIProvider – interface for AOI filtering
// ---------------------------------------------------------------------------

// AOIProvider is an optional interface that the StateSyncer uses to filter
// which entities are visible to a given session.
type AOIProvider interface {
	// GetVisibleEntities returns the entity IDs visible to the given entity.
	GetVisibleEntities(entityID engine.EntityID) []engine.EntityID
}

// ---------------------------------------------------------------------------
// stateSyncer implementation
// ---------------------------------------------------------------------------

type stateSyncer struct {
	mu               sync.RWMutex
	snapshotInterval int
	sessions         map[string]network.Session
	sessionEntities  map[string]engine.EntityID // sessionID → entity that session controls

	// Entity state tracking
	currentState map[engine.EntityID]EntityState
	prevState    map[engine.EntityID]EntityState

	// Snapshot storage
	lastSnapshot   *StateSnapshot
	lastSnapshotTick uint64

	// Delta history (for reconnection)
	deltaHistory []StateDelta
	maxDeltas    int // keep deltas since last snapshot + buffer

	// AOI provider (optional)
	aoiProvider AOIProvider
}

// NewStateSyncer creates a new state synchronizer.
func NewStateSyncer(snapshotInterval int, aoiProvider AOIProvider) StateSyncer {
	if snapshotInterval <= 0 {
		snapshotInterval = 100
	}
	return &stateSyncer{
		snapshotInterval: snapshotInterval,
		sessions:         make(map[string]network.Session),
		sessionEntities:  make(map[string]engine.EntityID),
		currentState:     make(map[engine.EntityID]EntityState),
		prevState:        make(map[engine.EntityID]EntityState),
		maxDeltas:        snapshotInterval * 2,
		aoiProvider:      aoiProvider,
	}
}

func (ss *stateSyncer) Mode() engine.SyncMode { return engine.SyncModeState }

func (ss *stateSyncer) SnapshotInterval() int { return ss.snapshotInterval }

func (ss *stateSyncer) AddSession(session network.Session) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.sessions[session.ID()] = session
}

func (ss *stateSyncer) RemoveSession(sessionID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.sessions, sessionID)
	delete(ss.sessionEntities, sessionID)
}

func (ss *stateSyncer) SessionCount() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.sessions)
}

// SetSessionEntity associates a session with the entity it controls (for AOI filtering).
func (ss *stateSyncer) SetSessionEntity(sessionID string, entityID engine.EntityID) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.sessionEntities[sessionID] = entityID
}

// SetEntityState updates the authoritative state for an entity.
// This should be called by the game logic each tick.
func (ss *stateSyncer) SetEntityState(entityID engine.EntityID, props map[string]float64) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.currentState[entityID] = EntityState{
		EntityID:   entityID,
		Properties: props,
	}
}

// RemoveEntity removes an entity from state tracking.
func (ss *stateSyncer) RemoveEntity(entityID engine.EntityID) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.currentState, entityID)
}

// OnInput processes a player input (state sync runs authoritative logic server-side,
// so inputs are queued for the game logic to process).
func (ss *stateSyncer) OnInput(session network.Session, input engine.PlayerInput) {
	// In state sync, inputs are processed by the authoritative game logic.
	// The syncer just acknowledges receipt; actual processing happens in the game loop.
}

// OnTick computes deltas, optionally takes a snapshot, and sends updates to sessions.
func (ss *stateSyncer) OnTick(tick uint64) {
	ss.mu.Lock()

	// Compute delta between previous and current state
	delta := ss.computeDeltaLocked(tick)

	// Store delta in history
	ss.deltaHistory = append(ss.deltaHistory, delta)
	if len(ss.deltaHistory) > ss.maxDeltas {
		ss.deltaHistory = ss.deltaHistory[1:]
	}

	// Check if we need a full snapshot
	needSnapshot := tick > 0 && tick%uint64(ss.snapshotInterval) == 0
	var snapshot *StateSnapshot
	if needSnapshot {
		snapshot = ss.takeSnapshotLocked(tick)
		ss.lastSnapshot = snapshot
		ss.lastSnapshotTick = tick
		// Trim delta history: only keep deltas after the snapshot
		ss.trimDeltaHistoryLocked(tick)
	}

	// Update prevState to current for next tick's delta computation
	ss.prevState = make(map[engine.EntityID]EntityState, len(ss.currentState))
	for id, state := range ss.currentState {
		ss.prevState[id] = state.Clone()
	}

	// Snapshot sessions and their entity mappings for sending
	infos := make([]syncSessionInfo, 0, len(ss.sessions))
	for sid, s := range ss.sessions {
		eid, has := ss.sessionEntities[sid]
		infos = append(infos, syncSessionInfo{session: s, entityID: eid, hasEntity: has})
	}

	ss.mu.Unlock()

	// Send snapshot or delta to each session
	if needSnapshot && snapshot != nil {
		ss.broadcastSnapshot(snapshot, infos)
	} else if len(delta.EntityDeltas) > 0 || len(delta.RemovedEntities) > 0 {
		ss.broadcastDelta(&delta, infos)
	}
}

// computeDeltaLocked computes the state delta. Caller must hold the lock.
func (ss *stateSyncer) computeDeltaLocked(tick uint64) StateDelta {
	delta := StateDelta{Tick: tick}

	// Check for changed/new entities
	for id, cur := range ss.currentState {
		prev, existed := ss.prevState[id]
		if !existed {
			// New entity – all properties are "changed"
			ed := EntityDelta{
				EntityID: id,
				Changed:  make(map[string]float64, len(cur.Properties)),
			}
			for k, v := range cur.Properties {
				ed.Changed[k] = v
			}
			delta.EntityDeltas = append(delta.EntityDeltas, ed)
			continue
		}

		// Existing entity – find property changes
		ed := EntityDelta{EntityID: id}
		for k, v := range cur.Properties {
			if oldV, ok := prev.Properties[k]; !ok || oldV != v {
				if ed.Changed == nil {
					ed.Changed = make(map[string]float64)
				}
				ed.Changed[k] = v
			}
		}
		// Find removed properties
		for k := range prev.Properties {
			if _, ok := cur.Properties[k]; !ok {
				ed.Removed = append(ed.Removed, k)
			}
		}
		if len(ed.Changed) > 0 || len(ed.Removed) > 0 {
			delta.EntityDeltas = append(delta.EntityDeltas, ed)
		}
	}

	// Check for removed entities
	for id := range ss.prevState {
		if _, exists := ss.currentState[id]; !exists {
			delta.RemovedEntities = append(delta.RemovedEntities, id)
		}
	}

	return delta
}

// takeSnapshotLocked creates a full state snapshot. Caller must hold the lock.
func (ss *stateSyncer) takeSnapshotLocked(tick uint64) *StateSnapshot {
	snap := &StateSnapshot{
		Tick:     tick,
		Entities: make([]EntityState, 0, len(ss.currentState)),
	}
	for _, state := range ss.currentState {
		snap.Entities = append(snap.Entities, state.Clone())
	}
	return snap
}

// trimDeltaHistoryLocked removes deltas older than the snapshot tick.
func (ss *stateSyncer) trimDeltaHistoryLocked(snapshotTick uint64) {
	trimIdx := 0
	for i, d := range ss.deltaHistory {
		if d.Tick > snapshotTick {
			trimIdx = i
			break
		}
		trimIdx = i + 1
	}
	if trimIdx > 0 && trimIdx <= len(ss.deltaHistory) {
		ss.deltaHistory = ss.deltaHistory[trimIdx:]
	}
}

// syncSessionInfo holds session info for broadcast operations.
type syncSessionInfo struct {
	session   network.Session
	entityID  engine.EntityID
	hasEntity bool
}

func (ss *stateSyncer) broadcastSnapshot(snapshot *StateSnapshot, infos []syncSessionInfo) {
	for _, info := range infos {
		filtered := ss.filterForSession(snapshot, info.entityID, info.hasEntity)
		data, err := json.Marshal(filtered)
		if err != nil {
			log.Printf("[sync] state: snapshot marshal error: %v", err)
			continue
		}
		if sendErr := info.session.Send(data); sendErr != nil {
			log.Printf("[sync] state: snapshot send error to %s: %v", info.session.ID(), sendErr)
		}
	}
}

func (ss *stateSyncer) broadcastDelta(delta *StateDelta, infos []syncSessionInfo) {
	for _, info := range infos {
		filtered := ss.filterDeltaForSession(delta, info.entityID, info.hasEntity)
		if len(filtered.EntityDeltas) == 0 && len(filtered.RemovedEntities) == 0 {
			continue
		}
		data, err := json.Marshal(filtered)
		if err != nil {
			log.Printf("[sync] state: delta marshal error: %v", err)
			continue
		}
		if sendErr := info.session.Send(data); sendErr != nil {
			log.Printf("[sync] state: delta send error to %s: %v", info.session.ID(), sendErr)
		}
	}
}

// filterForSession filters a snapshot to only include entities visible to the session's entity.
func (ss *stateSyncer) filterForSession(snapshot *StateSnapshot, entityID engine.EntityID, hasEntity bool) *StateSnapshot {
	if !hasEntity || ss.aoiProvider == nil {
		return snapshot // no AOI filtering
	}
	visible := ss.getVisibleSet(entityID)
	filtered := &StateSnapshot{
		Tick:     snapshot.Tick,
		Entities: make([]EntityState, 0),
	}
	for _, es := range snapshot.Entities {
		if visible[es.EntityID] || es.EntityID == entityID {
			filtered.Entities = append(filtered.Entities, es)
		}
	}
	return filtered
}

// filterDeltaForSession filters a delta to only include entities visible to the session's entity.
func (ss *stateSyncer) filterDeltaForSession(delta *StateDelta, entityID engine.EntityID, hasEntity bool) StateDelta {
	if !hasEntity || ss.aoiProvider == nil {
		return *delta // no AOI filtering
	}
	visible := ss.getVisibleSet(entityID)
	filtered := StateDelta{Tick: delta.Tick}
	for _, ed := range delta.EntityDeltas {
		if visible[ed.EntityID] || ed.EntityID == entityID {
			filtered.EntityDeltas = append(filtered.EntityDeltas, ed)
		}
	}
	for _, rid := range delta.RemovedEntities {
		if visible[rid] || rid == entityID {
			filtered.RemovedEntities = append(filtered.RemovedEntities, rid)
		}
	}
	return filtered
}

func (ss *stateSyncer) getVisibleSet(entityID engine.EntityID) map[engine.EntityID]bool {
	visible := make(map[engine.EntityID]bool)
	if ss.aoiProvider != nil {
		for _, eid := range ss.aoiProvider.GetVisibleEntities(entityID) {
			visible[eid] = true
		}
	}
	return visible
}

// OnReconnect sends the latest snapshot plus subsequent deltas to restore state.
func (ss *stateSyncer) OnReconnect(session network.Session) {
	ss.mu.RLock()
	snapshot := ss.lastSnapshot
	deltas := make([]StateDelta, len(ss.deltaHistory))
	copy(deltas, ss.deltaHistory)
	ss.mu.RUnlock()

	// Send snapshot first
	if snapshot != nil {
		data, err := json.Marshal(snapshot)
		if err != nil {
			log.Printf("[sync] state: reconnect snapshot marshal error: %v", err)
			return
		}
		if sendErr := session.Send(data); sendErr != nil {
			log.Printf("[sync] state: reconnect snapshot send error: %v", sendErr)
			return
		}
	}

	// Send subsequent deltas
	for _, d := range deltas {
		data, err := json.Marshal(d)
		if err != nil {
			log.Printf("[sync] state: reconnect delta marshal error: %v", err)
			continue
		}
		if sendErr := session.Send(data); sendErr != nil {
			log.Printf("[sync] state: reconnect delta send error: %v", sendErr)
			return
		}
	}
}

// ApplyDelta applies a StateDelta to a map of EntityStates, producing the next state.
// This is a pure function useful for testing delta correctness.
func ApplyDelta(state map[engine.EntityID]EntityState, delta StateDelta) map[engine.EntityID]EntityState {
	result := make(map[engine.EntityID]EntityState, len(state))
	for id, es := range state {
		result[id] = es.Clone()
	}

	// Apply entity deltas
	for _, ed := range delta.EntityDeltas {
		es, exists := result[ed.EntityID]
		if !exists {
			es = EntityState{
				EntityID:   ed.EntityID,
				Properties: make(map[string]float64),
			}
		}
		for k, v := range ed.Changed {
			es.Properties[k] = v
		}
		for _, k := range ed.Removed {
			delete(es.Properties, k)
		}
		result[ed.EntityID] = es
	}

	// Remove entities
	for _, rid := range delta.RemovedEntities {
		delete(result, rid)
	}

	return result
}

// ---------------------------------------------------------------------------
// SyncManager – factory and manager for per-room syncers
// ---------------------------------------------------------------------------

// SyncManager manages synchronization instances per room.
type SyncManager struct {
	mu      sync.RWMutex
	config  SyncManagerConfig
	syncers map[string]Syncer // roomID → Syncer
}

// NewSyncManager creates a new SyncManager with the given configuration.
func NewSyncManager(config SyncManagerConfig) *SyncManager {
	if config.LockstepTimeout <= 0 {
		config.LockstepTimeout = 100 * time.Millisecond
	}
	if config.HistoryFrames <= 0 {
		config.HistoryFrames = 300
	}
	if config.SnapshotInterval <= 0 {
		config.SnapshotInterval = 100
	}
	if config.AutoSwitchThreshold <= 0 {
		config.AutoSwitchThreshold = 50
	}
	return &SyncManager{
		config:  config,
		syncers: make(map[string]Syncer),
	}
}

// CreateSyncer creates and registers a syncer for the given room.
// If mode is not specified (negative), the default mode from config is used.
func (sm *SyncManager) CreateSyncer(roomID string, mode engine.SyncMode, aoiProvider AOIProvider) (Syncer, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.syncers[roomID]; exists {
		return nil, fmt.Errorf("syncer already exists for room %s", roomID)
	}

	var syncer Syncer
	switch mode {
	case engine.SyncModeLockstep:
		syncer = NewLockstepSyncer(sm.config.LockstepTimeout, sm.config.HistoryFrames)
	case engine.SyncModeState:
		syncer = NewStateSyncer(sm.config.SnapshotInterval, aoiProvider)
	default:
		return nil, fmt.Errorf("unknown sync mode: %d", mode)
	}

	sm.syncers[roomID] = syncer
	return syncer, nil
}

// GetSyncer returns the syncer for the given room.
func (sm *SyncManager) GetSyncer(roomID string) (Syncer, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.syncers[roomID]
	return s, ok
}

// RemoveSyncer removes and returns the syncer for the given room.
func (sm *SyncManager) RemoveSyncer(roomID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.syncers, roomID)
}

// CheckThreshold checks if the session count for a room exceeds the auto-switch
// threshold and logs a warning if so.
func (sm *SyncManager) CheckThreshold(roomID string) {
	sm.mu.RLock()
	syncer, ok := sm.syncers[roomID]
	sm.mu.RUnlock()
	if !ok {
		return
	}

	count := syncer.SessionCount()
	if count > sm.config.AutoSwitchThreshold && syncer.Mode() == engine.SyncModeLockstep {
		log.Printf("[WARN] sync: room %s has %d clients (threshold: %d), consider switching to State_Sync mode",
			roomID, count, sm.config.AutoSwitchThreshold)
	}
}

// RoomCount returns the number of rooms managed by the SyncManager.
func (sm *SyncManager) RoomCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.syncers)
}

// Config returns the current SyncManager configuration.
func (sm *SyncManager) Config() SyncManagerConfig {
	return sm.config
}
