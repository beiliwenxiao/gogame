package sync

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gfgame/internal/engine"
	"gfgame/internal/network"
)

// ---------------------------------------------------------------------------
// Mock Session
// ---------------------------------------------------------------------------

type mockSession struct {
	id       string
	messages [][]byte
	mu       sync.Mutex
	closed   atomic.Bool
}

func newMockSession(id string) *mockSession {
	return &mockSession{id: id}
}

func (s *mockSession) ID() string                          { return s.id }
func (s *mockSession) RemoteAddr() string                  { return "127.0.0.1:0" }
func (s *mockSession) Protocol() engine.TransportProtocol  { return engine.ProtocolWebSocket }

func (s *mockSession) Send(msg []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(msg))
	copy(cp, msg)
	s.messages = append(s.messages, cp)
	return nil
}

func (s *mockSession) Close() error {
	s.closed.Store(true)
	return nil
}

func (s *mockSession) Messages() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.messages))
	copy(out, s.messages)
	return out
}

// ---------------------------------------------------------------------------
// Mock AOI Provider
// ---------------------------------------------------------------------------

type mockAOIProvider struct {
	visible map[engine.EntityID][]engine.EntityID
}

func newMockAOIProvider() *mockAOIProvider {
	return &mockAOIProvider{visible: make(map[engine.EntityID][]engine.EntityID)}
}

func (m *mockAOIProvider) SetVisible(entityID engine.EntityID, visibleIDs []engine.EntityID) {
	m.visible[entityID] = visibleIDs
}

func (m *mockAOIProvider) GetVisibleEntities(entityID engine.EntityID) []engine.EntityID {
	return m.visible[entityID]
}

// Verify mockSession implements network.Session
var _ network.Session = (*mockSession)(nil)

// Verify mockAOIProvider implements AOIProvider
var _ AOIProvider = (*mockAOIProvider)(nil)

// ===========================================================================
// LockstepSyncer Tests
// ===========================================================================

func TestLockstepSyncer_Mode(t *testing.T) {
	ls := NewLockstepSyncer(100*time.Millisecond, 300)
	if ls.Mode() != engine.SyncModeLockstep {
		t.Errorf("expected SyncModeLockstep, got %d", ls.Mode())
	}
}

func TestLockstepSyncer_HistoryFrames(t *testing.T) {
	ls := NewLockstepSyncer(100*time.Millisecond, 500)
	if ls.HistoryFrames() != 500 {
		t.Errorf("expected 500 history frames, got %d", ls.HistoryFrames())
	}
}

func TestLockstepSyncer_Defaults(t *testing.T) {
	ls := NewLockstepSyncer(0, 0)
	if ls.HistoryFrames() != 300 {
		t.Errorf("expected default 300 history frames, got %d", ls.HistoryFrames())
	}
}

func TestLockstepSyncer_AddRemoveSession(t *testing.T) {
	ls := NewLockstepSyncer(100*time.Millisecond, 300)
	s1 := newMockSession("s1")
	s2 := newMockSession("s2")

	ls.AddSession(s1)
	ls.AddSession(s2)
	if ls.SessionCount() != 2 {
		t.Fatalf("expected 2 sessions, got %d", ls.SessionCount())
	}

	ls.RemoveSession("s1")
	if ls.SessionCount() != 1 {
		t.Fatalf("expected 1 session, got %d", ls.SessionCount())
	}
}

func TestLockstepSyncer_CollectAndBroadcast(t *testing.T) {
	ls := NewLockstepSyncer(100*time.Millisecond, 300)
	s1 := newMockSession("s1")
	s2 := newMockSession("s2")
	ls.AddSession(s1)
	ls.AddSession(s2)

	// Send inputs
	ls.OnInput(s1, engine.PlayerInput{PlayerID: 1, InputType: 1, Tick: 1, Data: []byte("move")})
	ls.OnInput(s2, engine.PlayerInput{PlayerID: 2, InputType: 2, Tick: 1, Data: []byte("attack")})

	// Trigger tick
	ls.OnTick(1)

	// Both sessions should receive the same frame
	msgs1 := s1.Messages()
	msgs2 := s2.Messages()
	if len(msgs1) != 1 || len(msgs2) != 1 {
		t.Fatalf("expected 1 message each, got s1=%d s2=%d", len(msgs1), len(msgs2))
	}

	// Verify content is identical
	if string(msgs1[0]) != string(msgs2[0]) {
		t.Error("frame data mismatch between sessions")
	}

	// Verify frame content
	var frame FrameData
	if err := json.Unmarshal(msgs1[0], &frame); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if frame.FrameID != 1 {
		t.Errorf("expected frame ID 1, got %d", frame.FrameID)
	}
	if len(frame.Inputs) != 2 {
		t.Errorf("expected 2 inputs, got %d", len(frame.Inputs))
	}
}

func TestLockstepSyncer_EmptyTick(t *testing.T) {
	ls := NewLockstepSyncer(100*time.Millisecond, 300)
	s1 := newMockSession("s1")
	ls.AddSession(s1)

	// Tick with no inputs
	ls.OnTick(1)

	msgs := s1.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var frame FrameData
	if err := json.Unmarshal(msgs[0], &frame); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(frame.Inputs) != 0 {
		t.Errorf("expected 0 inputs for empty tick, got %d", len(frame.Inputs))
	}
}

func TestLockstepSyncer_HistoryRingBuffer(t *testing.T) {
	maxHist := 5
	ls := NewLockstepSyncer(100*time.Millisecond, maxHist)
	s1 := newMockSession("s1")
	ls.AddSession(s1)

	// Generate more frames than history capacity
	for i := uint64(1); i <= 8; i++ {
		ls.OnInput(s1, engine.PlayerInput{PlayerID: 1, InputType: 1, Tick: i})
		ls.OnTick(i)
	}

	// Reconnect should only get the last 5 frames
	reconn := newMockSession("reconn")
	ls.OnReconnect(reconn)

	msgs := reconn.Messages()
	if len(msgs) != maxHist {
		t.Fatalf("expected %d history frames, got %d", maxHist, len(msgs))
	}

	// Verify frames are in order (4, 5, 6, 7, 8)
	for i, msg := range msgs {
		var frame FrameData
		if err := json.Unmarshal(msg, &frame); err != nil {
			t.Fatalf("unmarshal error at %d: %v", i, err)
		}
		expectedID := uint64(i + 4)
		if frame.FrameID != expectedID {
			t.Errorf("frame %d: expected ID %d, got %d", i, expectedID, frame.FrameID)
		}
	}
}

func TestLockstepSyncer_GetHistoryFrom(t *testing.T) {
	ls := NewLockstepSyncer(100*time.Millisecond, 10).(*lockstepSyncer)
	s1 := newMockSession("s1")
	ls.AddSession(s1)

	for i := uint64(1); i <= 5; i++ {
		ls.OnTick(i)
	}

	frames := ls.GetHistoryFrom(3)
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames from frame 3, got %d", len(frames))
	}
	if frames[0].FrameID != 3 || frames[2].FrameID != 5 {
		t.Errorf("unexpected frame range: %d to %d", frames[0].FrameID, frames[len(frames)-1].FrameID)
	}
}

func TestLockstepSyncer_BroadcastConsistency(t *testing.T) {
	ls := NewLockstepSyncer(100*time.Millisecond, 300)
	sessions := make([]*mockSession, 10)
	for i := range sessions {
		sessions[i] = newMockSession(fmt.Sprintf("s%d", i))
		ls.AddSession(sessions[i])
	}

	// Multiple ticks with various inputs
	for tick := uint64(1); tick <= 5; tick++ {
		for i := 0; i < 3; i++ {
			ls.OnInput(sessions[i], engine.PlayerInput{
				PlayerID:  engine.EntityID(i),
				InputType: uint32(tick),
				Tick:      tick,
			})
		}
		ls.OnTick(tick)
	}

	// All sessions should have received 5 frames
	for _, s := range sessions {
		msgs := s.Messages()
		if len(msgs) != 5 {
			t.Errorf("session %s: expected 5 messages, got %d", s.ID(), len(msgs))
		}
	}

	// All sessions should have identical frame data for each tick
	for frameIdx := 0; frameIdx < 5; frameIdx++ {
		ref := string(sessions[0].Messages()[frameIdx])
		for _, s := range sessions[1:] {
			if string(s.Messages()[frameIdx]) != ref {
				t.Errorf("frame %d: data mismatch between session %s and s0", frameIdx, s.ID())
			}
		}
	}
}

// ===========================================================================
// StateSyncer Tests
// ===========================================================================

func TestStateSyncer_Mode(t *testing.T) {
	ss := NewStateSyncer(100, nil)
	if ss.Mode() != engine.SyncModeState {
		t.Errorf("expected SyncModeState, got %d", ss.Mode())
	}
}

func TestStateSyncer_SnapshotInterval(t *testing.T) {
	ss := NewStateSyncer(50, nil)
	if ss.SnapshotInterval() != 50 {
		t.Errorf("expected 50, got %d", ss.SnapshotInterval())
	}
}

func TestStateSyncer_Defaults(t *testing.T) {
	ss := NewStateSyncer(0, nil)
	if ss.SnapshotInterval() != 100 {
		t.Errorf("expected default 100, got %d", ss.SnapshotInterval())
	}
}

func TestStateSyncer_AddRemoveSession(t *testing.T) {
	ss := NewStateSyncer(100, nil)
	s1 := newMockSession("s1")
	ss.AddSession(s1)
	if ss.SessionCount() != 1 {
		t.Fatalf("expected 1 session, got %d", ss.SessionCount())
	}
	ss.RemoveSession("s1")
	if ss.SessionCount() != 0 {
		t.Fatalf("expected 0 sessions, got %d", ss.SessionCount())
	}
}

func TestStateSyncer_DeltaComputation(t *testing.T) {
	ss := NewStateSyncer(100, nil).(*stateSyncer)
	s1 := newMockSession("s1")
	ss.AddSession(s1)

	// Tick 1: add entity with initial state
	ss.SetEntityState(1, map[string]float64{"hp": 100, "mp": 50})
	ss.OnTick(1)

	// Tick 2: modify entity state
	ss.SetEntityState(1, map[string]float64{"hp": 80, "mp": 50})
	ss.OnTick(2)

	msgs := s1.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	// First tick: entity is new, all properties are "changed"
	var delta1 StateDelta
	if err := json.Unmarshal(msgs[0], &delta1); err != nil {
		t.Fatalf("unmarshal delta1: %v", err)
	}
	if len(delta1.EntityDeltas) != 1 {
		t.Fatalf("expected 1 entity delta in tick 1, got %d", len(delta1.EntityDeltas))
	}

	// Second tick: only hp changed
	var delta2 StateDelta
	if err := json.Unmarshal(msgs[1], &delta2); err != nil {
		t.Fatalf("unmarshal delta2: %v", err)
	}
	if len(delta2.EntityDeltas) != 1 {
		t.Fatalf("expected 1 entity delta in tick 2, got %d", len(delta2.EntityDeltas))
	}
	if delta2.EntityDeltas[0].Changed["hp"] != 80 {
		t.Errorf("expected hp=80 in delta, got %v", delta2.EntityDeltas[0].Changed["hp"])
	}
	if _, hasMp := delta2.EntityDeltas[0].Changed["mp"]; hasMp {
		t.Error("mp should not be in delta since it didn't change")
	}
}

func TestStateSyncer_EntityRemoval(t *testing.T) {
	ss := NewStateSyncer(100, nil).(*stateSyncer)
	s1 := newMockSession("s1")
	ss.AddSession(s1)

	// Tick 1: add entity
	ss.SetEntityState(1, map[string]float64{"hp": 100})
	ss.OnTick(1)

	// Tick 2: remove entity
	ss.RemoveEntity(1)
	ss.OnTick(2)

	msgs := s1.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	var delta StateDelta
	if err := json.Unmarshal(msgs[1], &delta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(delta.RemovedEntities) != 1 || delta.RemovedEntities[0] != 1 {
		t.Errorf("expected entity 1 in removed list, got %v", delta.RemovedEntities)
	}
}

func TestStateSyncer_PeriodicSnapshot(t *testing.T) {
	interval := 3
	ss := NewStateSyncer(interval, nil).(*stateSyncer)
	s1 := newMockSession("s1")
	ss.AddSession(s1)

	ss.SetEntityState(1, map[string]float64{"hp": 100})

	// Run ticks; snapshot should happen at tick 3
	for tick := uint64(1); tick <= 4; tick++ {
		ss.OnTick(tick)
	}

	// Verify a snapshot was taken
	ss.mu.RLock()
	snap := ss.lastSnapshot
	ss.mu.RUnlock()

	if snap == nil {
		t.Fatal("expected a snapshot to be taken at tick 3")
	}
	if snap.Tick != 3 {
		t.Errorf("expected snapshot at tick 3, got tick %d", snap.Tick)
	}
}

func TestStateSyncer_Reconnect(t *testing.T) {
	ss := NewStateSyncer(5, nil).(*stateSyncer)
	s1 := newMockSession("s1")
	ss.AddSession(s1)

	ss.SetEntityState(1, map[string]float64{"hp": 100})

	// Run enough ticks to trigger a snapshot
	for tick := uint64(1); tick <= 7; tick++ {
		if tick == 4 {
			ss.SetEntityState(1, map[string]float64{"hp": 80})
		}
		ss.OnTick(tick)
	}

	// Reconnect should send snapshot + deltas
	reconn := newMockSession("reconn")
	ss.OnReconnect(reconn)

	msgs := reconn.Messages()
	if len(msgs) == 0 {
		t.Fatal("expected reconnect to send at least a snapshot")
	}

	// First message should be the snapshot
	var snap StateSnapshot
	if err := json.Unmarshal(msgs[0], &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Tick != 5 {
		t.Errorf("expected snapshot at tick 5, got %d", snap.Tick)
	}
}

func TestStateSyncer_AOIFiltering(t *testing.T) {
	aoi := newMockAOIProvider()
	ss := NewStateSyncer(100, aoi).(*stateSyncer)

	s1 := newMockSession("s1")
	ss.AddSession(s1)
	ss.SetSessionEntity("s1", 1)

	// Entity 1 can see entity 2 but not entity 3
	aoi.SetVisible(1, []engine.EntityID{2})

	ss.SetEntityState(1, map[string]float64{"hp": 100})
	ss.SetEntityState(2, map[string]float64{"hp": 200})
	ss.SetEntityState(3, map[string]float64{"hp": 300})

	ss.OnTick(1)

	msgs := s1.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var delta StateDelta
	if err := json.Unmarshal(msgs[0], &delta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should only contain entities 1 (self) and 2 (visible), not 3
	entityIDs := make(map[engine.EntityID]bool)
	for _, ed := range delta.EntityDeltas {
		entityIDs[ed.EntityID] = true
	}
	if !entityIDs[1] {
		t.Error("expected self entity 1 in delta")
	}
	if !entityIDs[2] {
		t.Error("expected visible entity 2 in delta")
	}
	if entityIDs[3] {
		t.Error("entity 3 should be filtered out by AOI")
	}
}

// ===========================================================================
// ApplyDelta Tests
// ===========================================================================

func TestApplyDelta_NewEntity(t *testing.T) {
	state := map[engine.EntityID]EntityState{}
	delta := StateDelta{
		Tick: 1,
		EntityDeltas: []EntityDelta{
			{EntityID: 1, Changed: map[string]float64{"hp": 100, "mp": 50}},
		},
	}

	result := ApplyDelta(state, delta)
	if len(result) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result))
	}
	if result[1].Properties["hp"] != 100 || result[1].Properties["mp"] != 50 {
		t.Errorf("unexpected properties: %v", result[1].Properties)
	}
}

func TestApplyDelta_ModifyEntity(t *testing.T) {
	state := map[engine.EntityID]EntityState{
		1: {EntityID: 1, Properties: map[string]float64{"hp": 100, "mp": 50}},
	}
	delta := StateDelta{
		Tick: 2,
		EntityDeltas: []EntityDelta{
			{EntityID: 1, Changed: map[string]float64{"hp": 80}},
		},
	}

	result := ApplyDelta(state, delta)
	if result[1].Properties["hp"] != 80 {
		t.Errorf("expected hp=80, got %v", result[1].Properties["hp"])
	}
	if result[1].Properties["mp"] != 50 {
		t.Errorf("expected mp=50 unchanged, got %v", result[1].Properties["mp"])
	}
}

func TestApplyDelta_RemoveProperty(t *testing.T) {
	state := map[engine.EntityID]EntityState{
		1: {EntityID: 1, Properties: map[string]float64{"hp": 100, "buff": 10}},
	}
	delta := StateDelta{
		Tick: 2,
		EntityDeltas: []EntityDelta{
			{EntityID: 1, Removed: []string{"buff"}},
		},
	}

	result := ApplyDelta(state, delta)
	if _, has := result[1].Properties["buff"]; has {
		t.Error("buff property should have been removed")
	}
	if result[1].Properties["hp"] != 100 {
		t.Error("hp should be unchanged")
	}
}

func TestApplyDelta_RemoveEntity(t *testing.T) {
	state := map[engine.EntityID]EntityState{
		1: {EntityID: 1, Properties: map[string]float64{"hp": 100}},
		2: {EntityID: 2, Properties: map[string]float64{"hp": 200}},
	}
	delta := StateDelta{
		Tick:            2,
		RemovedEntities: []engine.EntityID{1},
	}

	result := ApplyDelta(state, delta)
	if len(result) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result))
	}
	if _, has := result[1]; has {
		t.Error("entity 1 should have been removed")
	}
}

func TestApplyDelta_DoesNotMutateOriginal(t *testing.T) {
	state := map[engine.EntityID]EntityState{
		1: {EntityID: 1, Properties: map[string]float64{"hp": 100}},
	}
	delta := StateDelta{
		Tick: 2,
		EntityDeltas: []EntityDelta{
			{EntityID: 1, Changed: map[string]float64{"hp": 50}},
		},
	}

	_ = ApplyDelta(state, delta)
	// Original state should be unchanged
	if state[1].Properties["hp"] != 100 {
		t.Error("ApplyDelta mutated the original state")
	}
}

// ===========================================================================
// SyncManager Tests
// ===========================================================================

func TestSyncManager_CreateLockstepSyncer(t *testing.T) {
	sm := NewSyncManager(DefaultSyncManagerConfig())
	syncer, err := sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if syncer.Mode() != engine.SyncModeLockstep {
		t.Errorf("expected lockstep mode, got %d", syncer.Mode())
	}
}

func TestSyncManager_CreateStateSyncer(t *testing.T) {
	sm := NewSyncManager(DefaultSyncManagerConfig())
	syncer, err := sm.CreateSyncer("room1", engine.SyncModeState, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if syncer.Mode() != engine.SyncModeState {
		t.Errorf("expected state mode, got %d", syncer.Mode())
	}
}

func TestSyncManager_DuplicateRoom(t *testing.T) {
	sm := NewSyncManager(DefaultSyncManagerConfig())
	_, err := sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)
	if err == nil {
		t.Error("expected error for duplicate room")
	}
}

func TestSyncManager_GetSyncer(t *testing.T) {
	sm := NewSyncManager(DefaultSyncManagerConfig())
	sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)

	syncer, ok := sm.GetSyncer("room1")
	if !ok || syncer == nil {
		t.Fatal("expected to find syncer for room1")
	}

	_, ok = sm.GetSyncer("nonexistent")
	if ok {
		t.Error("expected not to find syncer for nonexistent room")
	}
}

func TestSyncManager_RemoveSyncer(t *testing.T) {
	sm := NewSyncManager(DefaultSyncManagerConfig())
	sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)
	sm.RemoveSyncer("room1")

	_, ok := sm.GetSyncer("room1")
	if ok {
		t.Error("expected syncer to be removed")
	}
	if sm.RoomCount() != 0 {
		t.Errorf("expected 0 rooms, got %d", sm.RoomCount())
	}
}

func TestSyncManager_RoomCount(t *testing.T) {
	sm := NewSyncManager(DefaultSyncManagerConfig())
	sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)
	sm.CreateSyncer("room2", engine.SyncModeState, nil)

	if sm.RoomCount() != 2 {
		t.Errorf("expected 2 rooms, got %d", sm.RoomCount())
	}
}

func TestSyncManager_ThresholdWarning(t *testing.T) {
	cfg := DefaultSyncManagerConfig()
	cfg.AutoSwitchThreshold = 3
	sm := NewSyncManager(cfg)

	syncer, _ := sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)

	// Add sessions exceeding threshold
	for i := 0; i < 5; i++ {
		syncer.AddSession(newMockSession(fmt.Sprintf("s%d", i)))
	}

	// This should log a warning (we can't easily capture log output,
	// but we verify it doesn't panic)
	sm.CheckThreshold("room1")
}

func TestSyncManager_ThresholdNoWarningForStateSync(t *testing.T) {
	cfg := DefaultSyncManagerConfig()
	cfg.AutoSwitchThreshold = 3
	sm := NewSyncManager(cfg)

	syncer, _ := sm.CreateSyncer("room1", engine.SyncModeState, nil)

	for i := 0; i < 5; i++ {
		syncer.AddSession(newMockSession(fmt.Sprintf("s%d", i)))
	}

	// State sync rooms should not trigger the warning
	sm.CheckThreshold("room1")
}

func TestSyncManager_DefaultConfig(t *testing.T) {
	cfg := DefaultSyncManagerConfig()
	if cfg.LockstepTimeout != 100*time.Millisecond {
		t.Errorf("expected 100ms timeout, got %v", cfg.LockstepTimeout)
	}
	if cfg.HistoryFrames != 300 {
		t.Errorf("expected 300 history frames, got %d", cfg.HistoryFrames)
	}
	if cfg.SnapshotInterval != 100 {
		t.Errorf("expected 100 snapshot interval, got %d", cfg.SnapshotInterval)
	}
	if cfg.AutoSwitchThreshold != 50 {
		t.Errorf("expected 50 threshold, got %d", cfg.AutoSwitchThreshold)
	}
}

func TestSyncManager_InvalidMode(t *testing.T) {
	sm := NewSyncManager(DefaultSyncManagerConfig())
	_, err := sm.CreateSyncer("room1", engine.SyncMode(99), nil)
	if err == nil {
		t.Error("expected error for invalid sync mode")
	}
}

func TestSyncManager_Config(t *testing.T) {
	cfg := SyncManagerConfig{
		DefaultMode:         engine.SyncModeState,
		LockstepTimeout:     200 * time.Millisecond,
		HistoryFrames:       500,
		SnapshotInterval:    50,
		AutoSwitchThreshold: 100,
	}
	sm := NewSyncManager(cfg)
	got := sm.Config()
	if got.DefaultMode != engine.SyncModeState {
		t.Error("config mismatch: DefaultMode")
	}
	if got.LockstepTimeout != 200*time.Millisecond {
		t.Error("config mismatch: LockstepTimeout")
	}
}

// ===========================================================================
// Integration-style test: full lockstep flow
// ===========================================================================

func TestLockstepSyncer_FullFlow(t *testing.T) {
	sm := NewSyncManager(SyncManagerConfig{
		LockstepTimeout: 100 * time.Millisecond,
		HistoryFrames:   10,
	})

	syncer, err := sm.CreateSyncer("room1", engine.SyncModeLockstep, nil)
	if err != nil {
		t.Fatalf("create syncer: %v", err)
	}

	s1 := newMockSession("player1")
	s2 := newMockSession("player2")
	syncer.AddSession(s1)
	syncer.AddSession(s2)

	// Simulate 5 ticks
	for tick := uint64(1); tick <= 5; tick++ {
		syncer.OnInput(s1, engine.PlayerInput{PlayerID: 1, InputType: 1, Tick: tick})
		syncer.OnInput(s2, engine.PlayerInput{PlayerID: 2, InputType: 2, Tick: tick})
		syncer.OnTick(tick)
	}

	// Verify both players got all 5 frames
	if len(s1.Messages()) != 5 {
		t.Errorf("player1: expected 5 frames, got %d", len(s1.Messages()))
	}
	if len(s2.Messages()) != 5 {
		t.Errorf("player2: expected 5 frames, got %d", len(s2.Messages()))
	}

	// Player 3 reconnects and should get all 5 history frames
	s3 := newMockSession("player3")
	syncer.AddSession(s3)
	syncer.OnReconnect(s3)

	if len(s3.Messages()) != 5 {
		t.Errorf("player3 reconnect: expected 5 frames, got %d", len(s3.Messages()))
	}

	sm.CheckThreshold("room1")
}

// ===========================================================================
// Integration-style test: full state sync flow
// ===========================================================================

func TestStateSyncer_FullFlow(t *testing.T) {
	aoi := newMockAOIProvider()
	sm := NewSyncManager(SyncManagerConfig{
		SnapshotInterval: 5,
	})

	syncer, err := sm.CreateSyncer("room1", engine.SyncModeState, aoi)
	if err != nil {
		t.Fatalf("create syncer: %v", err)
	}

	ss := syncer.(*stateSyncer)
	s1 := newMockSession("player1")
	syncer.AddSession(s1)
	ss.SetSessionEntity("player1", 1)

	// Entity 1 can see entity 2
	aoi.SetVisible(1, []engine.EntityID{2})

	// Tick 1: add entities
	ss.SetEntityState(1, map[string]float64{"hp": 100, "x": 0})
	ss.SetEntityState(2, map[string]float64{"hp": 200, "x": 10})
	ss.SetEntityState(3, map[string]float64{"hp": 300, "x": 20}) // not visible to player1
	syncer.OnTick(1)

	// Tick 2-4: modify states
	for tick := uint64(2); tick <= 4; tick++ {
		ss.SetEntityState(1, map[string]float64{"hp": 100, "x": float64(tick)})
		ss.SetEntityState(2, map[string]float64{"hp": 200, "x": float64(tick * 10)})
		syncer.OnTick(tick)
	}

	// Tick 5: should trigger snapshot
	ss.SetEntityState(1, map[string]float64{"hp": 90, "x": 5})
	syncer.OnTick(5)

	// Verify snapshot was taken
	ss.mu.RLock()
	snap := ss.lastSnapshot
	ss.mu.RUnlock()
	if snap == nil || snap.Tick != 5 {
		t.Error("expected snapshot at tick 5")
	}

	// Reconnect test
	s2 := newMockSession("player2")
	syncer.AddSession(s2)
	syncer.OnReconnect(s2)

	msgs := s2.Messages()
	if len(msgs) == 0 {
		t.Error("expected reconnect to send data")
	}
}


