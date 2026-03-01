package engine

import (
	"sync"
	"time"
)

// Resettable 由可在归还对象池前重置自身状态的对象实现。
type Resettable interface {
	Reset()
}

// ObjectPool 是对 sync.Pool 的泛型类型安全封装。
// T 必须是实现了 Resettable 的结构体指针。
type ObjectPool[T Resettable] struct {
	pool sync.Pool
}

// NewObjectPool 创建一个 ObjectPool，其 New 函数调用 newFn。
func NewObjectPool[T Resettable](newFn func() T) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() any { return newFn() },
		},
	}
}

// Get 从对象池获取一个对象（或创建新对象）。
func (p *ObjectPool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put 重置对象并将其归还到对象池。
func (p *ObjectPool[T]) Put(obj T) {
	obj.Reset()
	p.pool.Put(obj)
}

// ---------------------------------------------------------------------------
// 高频对象类型及其预建对象池
// ---------------------------------------------------------------------------

// Message 表示客户端与服务端之间交换的网络消息。
type Message struct {
	ID        uint16 // 消息类型标识符
	SessionID string // 来源/目标 Session
	Payload   []byte // 序列化后的消息体
}

// Reset 清空 Message 字段以便复用。
func (m *Message) Reset() {
	m.ID = 0
	m.SessionID = ""
	m.Payload = m.Payload[:0] // 保留底层数组
}

// Event 表示通过事件总线分发的事件。
type Event struct {
	Type      string
	Payload   any
	Timestamp time.Time
}

// Reset 清空 Event 字段以便复用。
func (e *Event) Reset() {
	e.Type = ""
	e.Payload = nil
	e.Timestamp = time.Time{}
}

// PlayerInput 表示从客户端收到的单次输入操作。
type PlayerInput struct {
	PlayerID  EntityID
	InputType uint32
	Tick      uint64
	Data      []byte
}

// Reset 清空 PlayerInput 字段以便复用。
func (pi *PlayerInput) Reset() {
	pi.PlayerID = 0
	pi.InputType = 0
	pi.Tick = 0
	pi.Data = pi.Data[:0]
}

// ---------------------------------------------------------------------------
// 全局单例对象池
// ---------------------------------------------------------------------------

// MessagePool 是 Message 对象的全局对象池。
var MessagePool = NewObjectPool(func() *Message {
	return &Message{Payload: make([]byte, 0, 256)}
})

// EventPool 是 Event 对象的全局对象池。
var EventPool = NewObjectPool(func() *Event {
	return &Event{}
})

// InputPool 是 PlayerInput 对象的全局对象池。
var InputPool = NewObjectPool(func() *PlayerInput {
	return &PlayerInput{Data: make([]byte, 0, 64)}
})
