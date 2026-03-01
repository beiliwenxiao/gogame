package room

import (
	"testing"
	"time"

	"gfgame/internal/engine"
)

// mockSession implements Session for testing.
type mockSession struct {
	id string
}

func (s *mockSession) ID() string { return s.id }

func newTestManager() *roomManager {
	rm := &roomManager{
		rooms:             make(map[string]*Room),
		migrations:        make(map[engine.EntityID]*EntityMigrationState),
		stopCh:            make(chan struct{}),
		idleCheckInterval: 30 * time.Second,
	}
	return rm
}

func basicRoomConfig(id string) RoomConfig {
	return RoomConfig{
		ID:          id,
		SyncMode:    engine.SyncModeState,
		MaxCapacity: 100,
		IdleTimeout: 30 * time.Second,
		TickRate:    20,
	}
}

func TestCreateRoom(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	r, err := rm.CreateRoom(basicRoomConfig("room-1"))
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if r.ID != "room-1" {
		t.Errorf("expected ID room-1, got %q", r.ID)
	}
	if r.SyncMode != engine.SyncModeState {
		t.Errorf("expected SyncModeState, got %v", r.SyncMode)
	}
	if rm.RoomCount() != 1 {
		t.Errorf("expected 1 room, got %d", rm.RoomCount())
	}
}

func TestCreateRoom_Duplicate(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))
	_, err := rm.CreateRoom(basicRoomConfig("room-1"))
	if err != ErrRoomExists {
		t.Errorf("expected ErrRoomExists, got %v", err)
	}
}

func TestCreateRoom_Defaults(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	r, _ := rm.CreateRoom(RoomConfig{ID: "default", SyncMode: engine.SyncModeState})
	if r.MaxCapacity != DefaultMaxCapacity {
		t.Errorf("expected default capacity %d, got %d", DefaultMaxCapacity, r.MaxCapacity)
	}

	r2, _ := rm.CreateRoom(RoomConfig{ID: "lockstep", SyncMode: engine.SyncModeLockstep})
	if r2.MaxCapacity != AutoSwitchThreshold {
		t.Errorf("expected lockstep capacity %d, got %d", AutoSwitchThreshold, r2.MaxCapacity)
	}
}

func TestDestroyRoom(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))
	err := rm.DestroyRoom("room-1")
	if err != nil {
		t.Fatalf("DestroyRoom failed: %v", err)
	}
	if rm.RoomCount() != 0 {
		t.Errorf("expected 0 rooms, got %d", rm.RoomCount())
	}
}

func TestDestroyRoom_NotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	err := rm.DestroyRoom("nonexistent")
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestGetRoom(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))

	r, ok := rm.GetRoom("room-1")
	if !ok || r == nil {
		t.Fatal("expected to find room-1")
	}

	_, ok = rm.GetRoom("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent room")
	}
}

func TestJoinRoom(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))
	sess := &mockSession{id: "sess-1"}

	err := rm.JoinRoom("room-1", sess)
	if err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}

	r, _ := rm.GetRoom("room-1")
	if r.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", r.ClientCount())
	}
	if !r.HasClient("sess-1") {
		t.Error("expected sess-1 in room")
	}
}

func TestJoinRoom_NotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	err := rm.JoinRoom("nonexistent", &mockSession{id: "s1"})
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestJoinRoom_Full(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	cfg := basicRoomConfig("room-1")
	cfg.MaxCapacity = 2
	rm.CreateRoom(cfg)

	rm.JoinRoom("room-1", &mockSession{id: "s1"})
	rm.JoinRoom("room-1", &mockSession{id: "s2"})

	err := rm.JoinRoom("room-1", &mockSession{id: "s3"})
	if err != ErrRoomFull {
		t.Errorf("expected ErrRoomFull, got %v", err)
	}
}

func TestJoinRoom_AdmissionDenied(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	cfg := basicRoomConfig("room-1")
	cfg.AdmissionCheck = func(s Session) bool { return s.ID() != "banned" }
	rm.CreateRoom(cfg)

	err := rm.JoinRoom("room-1", &mockSession{id: "banned"})
	if err != ErrAdmissionDenied {
		t.Errorf("expected ErrAdmissionDenied, got %v", err)
	}

	err = rm.JoinRoom("room-1", &mockSession{id: "allowed"})
	if err != nil {
		t.Fatalf("expected allowed session to join, got %v", err)
	}
}

func TestJoinRoom_AlreadyIn(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))
	sess := &mockSession{id: "s1"}
	rm.JoinRoom("room-1", sess)

	err := rm.JoinRoom("room-1", sess)
	if err != ErrClientAlreadyIn {
		t.Errorf("expected ErrClientAlreadyIn, got %v", err)
	}
}

func TestLeaveRoom(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))
	sess := &mockSession{id: "s1"}
	rm.JoinRoom("room-1", sess)

	err := rm.LeaveRoom("room-1", sess)
	if err != nil {
		t.Fatalf("LeaveRoom failed: %v", err)
	}

	r, _ := rm.GetRoom("room-1")
	if r.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", r.ClientCount())
	}
}

func TestLeaveRoom_NotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	err := rm.LeaveRoom("nonexistent", &mockSession{id: "s1"})
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestLeaveRoom_NotInRoom(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))
	err := rm.LeaveRoom("room-1", &mockSession{id: "s1"})
	if err != ErrClientNotInRoom {
		t.Errorf("expected ErrClientNotInRoom, got %v", err)
	}
}


func TestIdleRoomAutoDestroy(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	cfg := basicRoomConfig("idle-room")
	cfg.IdleTimeout = 50 * time.Millisecond
	rm.CreateRoom(cfg)

	if rm.RoomCount() != 1 {
		t.Fatalf("expected 1 room, got %d", rm.RoomCount())
	}

	time.Sleep(100 * time.Millisecond)
	rm.checkIdleRooms()

	if rm.RoomCount() != 0 {
		t.Errorf("expected idle room to be destroyed, got %d rooms", rm.RoomCount())
	}
}

func TestIdleRoom_NotDestroyedWithClients(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	cfg := basicRoomConfig("busy-room")
	cfg.IdleTimeout = 50 * time.Millisecond
	rm.CreateRoom(cfg)
	rm.JoinRoom("busy-room", &mockSession{id: "s1"})

	time.Sleep(100 * time.Millisecond)
	rm.checkIdleRooms()

	if rm.RoomCount() != 1 {
		t.Errorf("expected room with clients to remain, got %d rooms", rm.RoomCount())
	}
}

func TestRoomCount(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	if rm.RoomCount() != 0 {
		t.Errorf("expected 0, got %d", rm.RoomCount())
	}

	rm.CreateRoom(basicRoomConfig("r1"))
	rm.CreateRoom(basicRoomConfig("r2"))
	rm.CreateRoom(basicRoomConfig("r3"))

	if rm.RoomCount() != 3 {
		t.Errorf("expected 3, got %d", rm.RoomCount())
	}

	rm.DestroyRoom("r2")
	if rm.RoomCount() != 2 {
		t.Errorf("expected 2, got %d", rm.RoomCount())
	}
}

func TestGetClients(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-1"))
	rm.JoinRoom("room-1", &mockSession{id: "s1"})
	rm.JoinRoom("room-1", &mockSession{id: "s2"})

	r, _ := rm.GetRoom("room-1")
	clients := r.GetClients()
	if len(clients) != 2 {
		t.Errorf("expected 2 clients, got %d", len(clients))
	}
}

func TestRoomBoundaryZones(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	cfg := basicRoomConfig("room-1")
	cfg.BoundaryZones = []*RoomBoundaryZone{
		{
			NeighborRoomID: "room-2",
			LocalMin:       engine.Vector3{X: 90, Y: 0, Z: 0},
			LocalMax:       engine.Vector3{X: 100, Y: 100, Z: 100},
		},
	}
	r, _ := rm.CreateRoom(cfg)

	if len(r.BoundaryZones) != 1 {
		t.Fatalf("expected 1 boundary zone, got %d", len(r.BoundaryZones))
	}
	if r.BoundaryZones[0].NeighborRoomID != "room-2" {
		t.Errorf("expected neighbor room-2, got %q", r.BoundaryZones[0].NeighborRoomID)
	}
}

// ---------- Task 12.3: Seamless Room Switch & Entity Migration Tests ----------

func TestConfigureRoomBoundary(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	rm.CreateRoom(basicRoomConfig("room-B"))

	zone := &RoomBoundaryZone{
		LocalMin:  engine.Vector3{X: 90, Y: 0, Z: 0},
		LocalMax:  engine.Vector3{X: 100, Y: 100, Z: 100},
		RemoteMin: engine.Vector3{X: 0, Y: 0, Z: 0},
		RemoteMax: engine.Vector3{X: 10, Y: 100, Z: 100},
	}

	err := rm.ConfigureRoomBoundary("room-A", "room-B", zone)
	if err != nil {
		t.Fatalf("ConfigureRoomBoundary failed: %v", err)
	}

	got, ok := rm.GetRoomBoundary("room-A", "room-B")
	if !ok {
		t.Fatal("expected boundary zone to exist")
	}
	if got.NeighborRoomID != "room-B" {
		t.Errorf("expected NeighborRoomID room-B, got %q", got.NeighborRoomID)
	}
}

func TestConfigureRoomBoundary_RoomNotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	err := rm.ConfigureRoomBoundary("nonexistent", "room-B", &RoomBoundaryZone{})
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestGetRoomBoundary_NotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))

	_, ok := rm.GetRoomBoundary("room-A", "room-B")
	if ok {
		t.Error("expected no boundary zone for unconfigured neighbor")
	}

	_, ok = rm.GetRoomBoundary("nonexistent", "room-B")
	if ok {
		t.Error("expected no boundary zone for nonexistent room")
	}
}

func TestSeamlessTransfer(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	rm.CreateRoom(basicRoomConfig("room-B"))

	sess := &mockSession{id: "player-1"}
	rm.JoinRoom("room-A", sess)

	err := rm.SeamlessTransfer(sess, "room-A", "room-B")
	if err != nil {
		t.Fatalf("SeamlessTransfer failed: %v", err)
	}

	rA, _ := rm.GetRoom("room-A")
	rB, _ := rm.GetRoom("room-B")

	if rA.HasClient("player-1") {
		t.Error("player-1 should have left room-A")
	}
	if !rB.HasClient("player-1") {
		t.Error("player-1 should be in room-B")
	}
}

func TestSeamlessTransfer_RoomNotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	sess := &mockSession{id: "p1"}

	err := rm.SeamlessTransfer(sess, "room-A", "nonexistent")
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestSeamlessTransfer_TargetFull(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	cfg := basicRoomConfig("room-B")
	cfg.MaxCapacity = 1
	rm.CreateRoom(cfg)

	rm.JoinRoom("room-A", &mockSession{id: "p1"})
	rm.JoinRoom("room-B", &mockSession{id: "p2"}) // fills room-B

	err := rm.SeamlessTransfer(&mockSession{id: "p1"}, "room-A", "room-B")
	if err != ErrRoomFull {
		t.Errorf("expected ErrRoomFull, got %v", err)
	}
}

func TestSeamlessTransfer_AdmissionDenied(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	cfg := basicRoomConfig("room-B")
	cfg.AdmissionCheck = func(s Session) bool { return false }
	rm.CreateRoom(cfg)

	sess := &mockSession{id: "p1"}
	rm.JoinRoom("room-A", sess)

	err := rm.SeamlessTransfer(sess, "room-A", "room-B")
	if err != ErrAdmissionDenied {
		t.Errorf("expected ErrAdmissionDenied, got %v", err)
	}
}

func TestMigrateEntity(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	rm.CreateRoom(basicRoomConfig("room-B"))

	var eid engine.EntityID = 42
	rm.AddEntity("room-A", eid)

	state, err := rm.MigrateEntity(eid, "room-A", "room-B")
	if err != nil {
		t.Fatalf("MigrateEntity failed: %v", err)
	}
	if state.Phase != engine.MigrationComplete {
		t.Errorf("expected MigrationComplete, got %v", state.Phase)
	}
	if state.EntityID != eid {
		t.Errorf("expected entity %d, got %d", eid, state.EntityID)
	}

	// Entity should be in room-B, not room-A.
	rA, _ := rm.GetRoom("room-A")
	rB, _ := rm.GetRoom("room-B")

	rA.mu.RLock()
	_, inA := rA.entities[eid]
	rA.mu.RUnlock()

	rB.mu.RLock()
	_, inB := rB.entities[eid]
	rB.mu.RUnlock()

	if inA {
		t.Error("entity should not be in room-A after migration")
	}
	if !inB {
		t.Error("entity should be in room-B after migration")
	}
}

func TestMigrateEntity_RoomNotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	var eid engine.EntityID = 1
	rm.AddEntity("room-A", eid)

	_, err := rm.MigrateEntity(eid, "room-A", "nonexistent")
	if err != ErrRoomNotFound {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestMigrateEntity_NotInRoom(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	rm.CreateRoom(basicRoomConfig("room-B"))

	var eid engine.EntityID = 99
	// Entity not added to room-A.
	_, err := rm.MigrateEntity(eid, "room-A", "room-B")
	if err != ErrClientNotInRoom {
		t.Errorf("expected ErrClientNotInRoom, got %v", err)
	}
}

func TestGetMigrationState(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	rm.CreateRoom(basicRoomConfig("room-B"))

	var eid engine.EntityID = 7
	rm.AddEntity("room-A", eid)
	rm.MigrateEntity(eid, "room-A", "room-B")

	state, ok := rm.GetMigrationState(eid)
	if !ok {
		t.Fatal("expected migration state to exist")
	}
	if state.Phase != engine.MigrationComplete {
		t.Errorf("expected MigrationComplete, got %v", state.Phase)
	}
	if state.SourceRoomID != "room-A" || state.TargetRoomID != "room-B" {
		t.Errorf("unexpected room IDs: %q → %q", state.SourceRoomID, state.TargetRoomID)
	}
}

func TestGetMigrationState_NotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	_, ok := rm.GetMigrationState(engine.EntityID(999))
	if ok {
		t.Error("expected no migration state for unknown entity")
	}
}

func TestGetBoundaryEntities(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	rm.CreateRoom(basicRoomConfig("room-B"))

	zone := &RoomBoundaryZone{
		LocalMin: engine.Vector3{X: 90},
		LocalMax: engine.Vector3{X: 100},
	}
	rm.ConfigureRoomBoundary("room-A", "room-B", zone)

	rm.AddEntity("room-A", engine.EntityID(1))
	rm.AddEntity("room-A", engine.EntityID(2))

	entities := rm.GetBoundaryEntities("room-A", "room-B")
	if len(entities) != 2 {
		t.Errorf("expected 2 boundary entities, got %d", len(entities))
	}
}

func TestGetBoundaryEntities_NoBoundary(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	rm.CreateRoom(basicRoomConfig("room-A"))
	rm.AddEntity("room-A", engine.EntityID(1))

	entities := rm.GetBoundaryEntities("room-A", "room-B")
	if entities != nil {
		t.Errorf("expected nil for unconfigured boundary, got %v", entities)
	}
}

func TestGetBoundaryEntities_RoomNotFound(t *testing.T) {
	rm := newTestManager()
	defer rm.Stop()

	entities := rm.GetBoundaryEntities("nonexistent", "room-B")
	if entities != nil {
		t.Errorf("expected nil for nonexistent room, got %v", entities)
	}
}
