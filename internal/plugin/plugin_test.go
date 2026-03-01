package plugin

import (
	"errors"
	"sync/atomic"
	"testing"

	"gfgame/internal/engine"
)

// ---------- mock plugin ----------

type mockPlugin struct {
	name     string
	required bool
	initErr  error
	startErr error
	stopErr  error
	hooks    []*Hook
	initCalled  int32
	startCalled int32
	stopCalled  int32
	bus         EventBus
}

func (p *mockPlugin) Name() string     { return p.name }
func (p *mockPlugin) Required() bool   { return p.required }
func (p *mockPlugin) Hooks() []*Hook   { return p.hooks }

func (p *mockPlugin) Init(bus EventBus) error {
	atomic.AddInt32(&p.initCalled, 1)
	p.bus = bus
	return p.initErr
}

func (p *mockPlugin) Start() error {
	atomic.AddInt32(&p.startCalled, 1)
	return p.startErr
}

func (p *mockPlugin) Stop() error {
	atomic.AddInt32(&p.stopCalled, 1)
	return p.stopErr
}

// ---------- EventBus tests ----------

func TestEventBus_Subscribe_Publish(t *testing.T) {
	bus := NewEventBus()
	received := make([]Event, 0)

	bus.Subscribe("player.join", func(e Event) {
		received = append(received, e)
	})

	bus.Publish(Event{Type: "player.join", Payload: "player-1"})
	bus.Publish(Event{Type: "player.join", Payload: "player-2"})

	if len(received) != 2 {
		t.Errorf("expected 2 events, got %d", len(received))
	}
	if received[0].Payload != "player-1" {
		t.Errorf("expected player-1, got %v", received[0].Payload)
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	var count int32

	bus.Subscribe("tick", func(e Event) { atomic.AddInt32(&count, 1) })
	bus.Subscribe("tick", func(e Event) { atomic.AddInt32(&count, 1) })
	bus.Subscribe("tick", func(e Event) { atomic.AddInt32(&count, 1) })

	bus.Publish(Event{Type: "tick"})

	if count != 3 {
		t.Errorf("expected 3 handlers called, got %d", count)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()
	var count int32

	id := bus.Subscribe("combat.start", func(e Event) { atomic.AddInt32(&count, 1) })
	bus.Publish(Event{Type: "combat.start"})

	bus.Unsubscribe(id)
	bus.Publish(Event{Type: "combat.start"})

	if count != 1 {
		t.Errorf("expected 1 call (before unsubscribe), got %d", count)
	}
}

func TestEventBus_Unsubscribe_Unknown(t *testing.T) {
	bus := NewEventBus()
	// Should not panic.
	bus.Unsubscribe("nonexistent-id")
}

func TestEventBus_NoSubscribers(t *testing.T) {
	bus := NewEventBus()
	// Should not panic.
	bus.Publish(Event{Type: "unknown.event"})
}

func TestEventBus_DifferentTypes_Isolated(t *testing.T) {
	bus := NewEventBus()
	var joinCount, leaveCount int32

	bus.Subscribe("player.join", func(e Event) { atomic.AddInt32(&joinCount, 1) })
	bus.Subscribe("player.leave", func(e Event) { atomic.AddInt32(&leaveCount, 1) })

	bus.Publish(Event{Type: "player.join"})
	bus.Publish(Event{Type: "player.join"})
	bus.Publish(Event{Type: "player.leave"})

	if joinCount != 2 || leaveCount != 1 {
		t.Errorf("expected join=2 leave=1, got join=%d leave=%d", joinCount, leaveCount)
	}
}

// ---------- PluginManager tests ----------

func TestPluginManager_Register(t *testing.T) {
	pm := NewPluginManager()
	p := &mockPlugin{name: "test-plugin", required: true}

	err := pm.Register(p)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if pm.PluginCount() != 1 {
		t.Errorf("expected 1 plugin, got %d", pm.PluginCount())
	}
}

func TestPluginManager_Register_Duplicate(t *testing.T) {
	pm := NewPluginManager()
	p := &mockPlugin{name: "dup"}
	pm.Register(p)

	err := pm.Register(p)
	if err != ErrPluginExists {
		t.Errorf("expected ErrPluginExists, got %v", err)
	}
}

func TestPluginManager_LoadAll_CallsLifecycle(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()
	p := &mockPlugin{name: "p1", required: true}
	pm.Register(p)

	err := pm.LoadAll(bus)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if p.initCalled != 1 {
		t.Errorf("expected Init called once, got %d", p.initCalled)
	}
	if p.startCalled != 1 {
		t.Errorf("expected Start called once, got %d", p.startCalled)
	}
}

func TestPluginManager_LoadAll_RequiredInitFail_Aborts(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()

	p := &mockPlugin{name: "required-fail", required: true, initErr: errors.New("init error")}
	pm.Register(p)

	err := pm.LoadAll(bus)
	if err == nil {
		t.Error("expected error when required plugin fails Init")
	}
	if !errors.Is(err, ErrPluginInitFailed) {
		t.Errorf("expected ErrPluginInitFailed, got %v", err)
	}
}

func TestPluginManager_LoadAll_OptionalInitFail_Continues(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()

	optional := &mockPlugin{name: "optional-fail", required: false, initErr: errors.New("init error")}
	required := &mockPlugin{name: "required-ok", required: true}
	pm.Register(optional)
	pm.Register(required)

	err := pm.LoadAll(bus)
	if err != nil {
		t.Fatalf("expected LoadAll to succeed despite optional failure, got %v", err)
	}
	if required.startCalled != 1 {
		t.Error("expected required plugin to start despite optional failure")
	}
}

func TestPluginManager_StopAll_CallsStop(t *testing.T) {
	bus := NewEventBus()
	pm := NewPluginManager()
	pa := &mockPlugin{name: "a", required: true}
	pb := &mockPlugin{name: "b", required: true}
	pm.Register(pa)
	pm.Register(pb)
	pm.LoadAll(bus)

	pm.StopAll()

	if pa.stopCalled != 1 || pb.stopCalled != 1 {
		t.Errorf("expected both plugins stopped, got a=%d b=%d", pa.stopCalled, pb.stopCalled)
	}
}

func TestPluginManager_GetPlugin(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()
	p := &mockPlugin{name: "my-plugin", required: true}
	pm.Register(p)
	pm.LoadAll(bus)

	got, ok := pm.GetPlugin("my-plugin")
	if !ok || got.Name() != "my-plugin" {
		t.Errorf("expected to find my-plugin, got %v", got)
	}

	_, ok = pm.GetPlugin("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent plugin")
	}
}

// ---------- Hook tests ----------

func TestHooks_RegisteredAndRetrieved(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()

	var called int32
	p := &mockPlugin{
		name:     "hook-plugin",
		required: true,
		hooks: []*Hook{
			{Phase: engine.PhaseUpdate, Priority: 10, Fn: func(_ HookPhase, _ float64) {
				atomic.AddInt32(&called, 1)
			}},
		},
	}
	pm.Register(p)
	pm.LoadAll(bus)

	hooks := pm.GetHooks(engine.PhaseUpdate)
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}

	hooks[0].Fn(engine.PhaseUpdate, 0.016)
	if called != 1 {
		t.Errorf("expected hook called once, got %d", called)
	}
}

func TestHooks_PriorityOrder(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()

	order := make([]int, 0)
	p := &mockPlugin{
		name:     "multi-hook",
		required: true,
		hooks: []*Hook{
			{Phase: engine.PhaseUpdate, Priority: 30, Fn: func(_ HookPhase, _ float64) { order = append(order, 30) }},
			{Phase: engine.PhaseUpdate, Priority: 10, Fn: func(_ HookPhase, _ float64) { order = append(order, 10) }},
			{Phase: engine.PhaseUpdate, Priority: 20, Fn: func(_ HookPhase, _ float64) { order = append(order, 20) }},
		},
	}
	pm.Register(p)
	pm.LoadAll(bus)

	for _, h := range pm.GetHooks(engine.PhaseUpdate) {
		h.Fn(engine.PhaseUpdate, 0)
	}

	if len(order) != 3 || order[0] != 10 || order[1] != 20 || order[2] != 30 {
		t.Errorf("expected hooks in priority order [10,20,30], got %v", order)
	}
}

func TestHooks_DifferentPhases_Isolated(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()

	p := &mockPlugin{
		name:     "phase-plugin",
		required: true,
		hooks: []*Hook{
			{Phase: engine.PhaseInput, Priority: 1, Fn: func(_ HookPhase, _ float64) {}},
			{Phase: engine.PhaseSync, Priority: 1, Fn: func(_ HookPhase, _ float64) {}},
		},
	}
	pm.Register(p)
	pm.LoadAll(bus)

	if len(pm.GetHooks(engine.PhaseInput)) != 1 {
		t.Errorf("expected 1 input hook")
	}
	if len(pm.GetHooks(engine.PhaseSync)) != 1 {
		t.Errorf("expected 1 sync hook")
	}
	if len(pm.GetHooks(engine.PhaseUpdate)) != 0 {
		t.Errorf("expected 0 update hooks")
	}
}

func TestPlugin_ReceivesEventBus(t *testing.T) {
	pm := NewPluginManager()
	bus := NewEventBus()
	p := &mockPlugin{name: "bus-plugin", required: true}
	pm.Register(p)
	pm.LoadAll(bus)

	if p.bus == nil {
		t.Error("expected plugin to receive EventBus during Init")
	}
}
