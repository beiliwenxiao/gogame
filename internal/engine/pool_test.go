package engine

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ObjectPool generic tests
// ---------------------------------------------------------------------------

func TestObjectPool_GetReturnsNewObject(t *testing.T) {
	pool := NewObjectPool(func() *Message {
		return &Message{Payload: make([]byte, 0, 64)}
	})
	msg := pool.Get()
	if msg == nil {
		t.Fatal("Get() returned nil")
	}
}

func TestObjectPool_PutResetsObject(t *testing.T) {
	pool := NewObjectPool(func() *Message {
		return &Message{Payload: make([]byte, 0, 64)}
	})

	msg := pool.Get()
	msg.ID = 42
	msg.SessionID = "sess-1"
	msg.Payload = append(msg.Payload, 0xAA, 0xBB)

	pool.Put(msg)

	// After Put the object should have been reset.
	// We cannot guarantee we get the same pointer back from sync.Pool,
	// but we can verify the contract by getting and checking a fresh one.
	msg2 := pool.Get()
	// If we got the recycled object, it must be reset.
	if msg2.ID != 0 || msg2.SessionID != "" || len(msg2.Payload) != 0 {
		// It's possible sync.Pool gave us a brand-new object, which is also fine.
		// Only fail if the object clearly was NOT reset.
		if msg2 == msg {
			t.Fatalf("recycled Message was not reset: ID=%d SessionID=%q Payload=%v",
				msg2.ID, msg2.SessionID, msg2.Payload)
		}
	}
}

// ---------------------------------------------------------------------------
// Message pool tests
// ---------------------------------------------------------------------------

func TestMessagePool_GetPut(t *testing.T) {
	msg := MessagePool.Get()
	if msg == nil {
		t.Fatal("MessagePool.Get() returned nil")
	}
	msg.ID = 100
	msg.SessionID = "abc"
	msg.Payload = append(msg.Payload, 1, 2, 3)

	MessagePool.Put(msg)
}

func TestMessage_Reset(t *testing.T) {
	m := &Message{
		ID:        10,
		SessionID: "s",
		Payload:   []byte{1, 2, 3},
	}
	m.Reset()
	if m.ID != 0 {
		t.Errorf("expected ID=0, got %d", m.ID)
	}
	if m.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", m.SessionID)
	}
	if len(m.Payload) != 0 {
		t.Errorf("expected empty Payload, got len=%d", len(m.Payload))
	}
	// Underlying capacity should be preserved.
	if cap(m.Payload) < 3 {
		t.Errorf("expected Payload cap >= 3, got %d", cap(m.Payload))
	}
}

// ---------------------------------------------------------------------------
// Event pool tests
// ---------------------------------------------------------------------------

func TestEventPool_GetPut(t *testing.T) {
	ev := EventPool.Get()
	if ev == nil {
		t.Fatal("EventPool.Get() returned nil")
	}
	ev.Type = "combat.hit"
	ev.Payload = map[string]int{"damage": 50}
	ev.Timestamp = time.Now()

	EventPool.Put(ev)
}

func TestEvent_Reset(t *testing.T) {
	e := &Event{
		Type:      "test",
		Payload:   42,
		Timestamp: time.Now(),
	}
	e.Reset()
	if e.Type != "" {
		t.Errorf("expected empty Type, got %q", e.Type)
	}
	if e.Payload != nil {
		t.Errorf("expected nil Payload, got %v", e.Payload)
	}
	if !e.Timestamp.IsZero() {
		t.Errorf("expected zero Timestamp, got %v", e.Timestamp)
	}
}

// ---------------------------------------------------------------------------
// PlayerInput pool tests
// ---------------------------------------------------------------------------

func TestInputPool_GetPut(t *testing.T) {
	inp := InputPool.Get()
	if inp == nil {
		t.Fatal("InputPool.Get() returned nil")
	}
	inp.PlayerID = EntityID(1)
	inp.InputType = 3
	inp.Tick = 999
	inp.Data = append(inp.Data, 0xFF)

	InputPool.Put(inp)
}

func TestPlayerInput_Reset(t *testing.T) {
	pi := &PlayerInput{
		PlayerID:  EntityID(7),
		InputType: 2,
		Tick:      100,
		Data:      []byte{10, 20},
	}
	pi.Reset()
	if pi.PlayerID != 0 {
		t.Errorf("expected PlayerID=0, got %d", pi.PlayerID)
	}
	if pi.InputType != 0 {
		t.Errorf("expected InputType=0, got %d", pi.InputType)
	}
	if pi.Tick != 0 {
		t.Errorf("expected Tick=0, got %d", pi.Tick)
	}
	if len(pi.Data) != 0 {
		t.Errorf("expected empty Data, got len=%d", len(pi.Data))
	}
	if cap(pi.Data) < 2 {
		t.Errorf("expected Data cap >= 2, got %d", cap(pi.Data))
	}
}

// ---------------------------------------------------------------------------
// Concurrent access tests
// ---------------------------------------------------------------------------

func TestMessagePool_ConcurrentAccess(t *testing.T) {
	const goroutines = 100
	const opsPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				msg := MessagePool.Get()
				msg.ID = uint16(id)
				msg.SessionID = "concurrent"
				msg.Payload = append(msg.Payload, byte(i))
				MessagePool.Put(msg)
			}
		}(g)
	}

	wg.Wait()
}

func TestEventPool_ConcurrentAccess(t *testing.T) {
	const goroutines = 100
	const opsPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				ev := EventPool.Get()
				ev.Type = "tick"
				ev.Timestamp = time.Now()
				EventPool.Put(ev)
			}
		}()
	}

	wg.Wait()
}

func TestInputPool_ConcurrentAccess(t *testing.T) {
	const goroutines = 100
	const opsPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				inp := InputPool.Get()
				inp.PlayerID = EntityID(id)
				inp.InputType = uint32(i)
				inp.Tick = uint64(i)
				inp.Data = append(inp.Data, byte(i))
				InputPool.Put(inp)
			}
		}(g)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Custom pool type test (verifies generic ObjectPool works with any Resettable)
// ---------------------------------------------------------------------------

type testResettable struct {
	Value int
}

func (tr *testResettable) Reset() {
	tr.Value = 0
}

func TestObjectPool_CustomType(t *testing.T) {
	pool := NewObjectPool(func() *testResettable {
		return &testResettable{}
	})

	obj := pool.Get()
	obj.Value = 42
	pool.Put(obj)

	obj2 := pool.Get()
	// Either a recycled (reset) object or a brand-new one — both should have Value == 0.
	if obj2.Value != 0 {
		if obj2 == obj {
			t.Fatalf("recycled testResettable was not reset: Value=%d", obj2.Value)
		}
	}
}
