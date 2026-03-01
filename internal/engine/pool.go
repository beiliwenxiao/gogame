package engine

import (
	"sync"
	"time"
)

// Resettable is implemented by objects that can reset their state before
// being returned to a pool.
type Resettable interface {
	Reset()
}

// ObjectPool is a generic, type-safe wrapper around sync.Pool.
// T must be a pointer to a struct that implements Resettable.
type ObjectPool[T Resettable] struct {
	pool sync.Pool
}

// NewObjectPool creates an ObjectPool whose New function calls newFn.
func NewObjectPool[T Resettable](newFn func() T) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() any { return newFn() },
		},
	}
}

// Get retrieves an object from the pool (or creates a new one).
func (p *ObjectPool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put resets the object and returns it to the pool.
func (p *ObjectPool[T]) Put(obj T) {
	obj.Reset()
	p.pool.Put(obj)
}

// ---------------------------------------------------------------------------
// High-frequency object types and their pre-built pools
// ---------------------------------------------------------------------------

// Message represents a network message exchanged between client and server.
type Message struct {
	ID        uint16 // message type identifier
	SessionID string // source / destination session
	Payload   []byte // serialised message body
}

// Reset clears the Message fields so it can be reused.
func (m *Message) Reset() {
	m.ID = 0
	m.SessionID = ""
	m.Payload = m.Payload[:0] // keep underlying array
}

// Event represents an event dispatched through the event bus.
type Event struct {
	Type      string
	Payload   any
	Timestamp time.Time
}

// Reset clears the Event fields so it can be reused.
func (e *Event) Reset() {
	e.Type = ""
	e.Payload = nil
	e.Timestamp = time.Time{}
}

// PlayerInput represents a single input action received from a client.
type PlayerInput struct {
	PlayerID  EntityID
	InputType uint32
	Tick      uint64
	Data      []byte
}

// Reset clears the PlayerInput fields so it can be reused.
func (pi *PlayerInput) Reset() {
	pi.PlayerID = 0
	pi.InputType = 0
	pi.Tick = 0
	pi.Data = pi.Data[:0]
}

// ---------------------------------------------------------------------------
// Pre-built singleton pools
// ---------------------------------------------------------------------------

// MessagePool is the global pool for Message objects.
var MessagePool = NewObjectPool(func() *Message {
	return &Message{Payload: make([]byte, 0, 256)}
})

// EventPool is the global pool for Event objects.
var EventPool = NewObjectPool(func() *Event {
	return &Event{}
})

// InputPool is the global pool for PlayerInput objects.
var InputPool = NewObjectPool(func() *PlayerInput {
	return &PlayerInput{Data: make([]byte, 0, 64)}
})
