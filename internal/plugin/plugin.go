// Package plugin implements the Plugin system and EventBus for the MMRPG game engine.
// Plugins can subscribe to events, inject logic into GameLoop phases via Hooks,
// and are managed through a lifecycle: Init → Start → Stop.
package plugin

import (
	"errors"
	"fmt"
	"sync"

	"gfgame/internal/engine"
)

// ---------- Errors ----------

var (
	ErrPluginNotFound    = errors.New("plugin not found")
	ErrPluginExists      = errors.New("plugin already registered")
	ErrPluginInitFailed  = errors.New("plugin init failed")
	ErrEventNotFound     = errors.New("event type not registered")
)

// ---------- Event ----------

// Event carries data published on the EventBus.
type Event struct {
	Type    string
	Payload interface{}
}

// Handler is a function that processes an Event.
type Handler func(event Event)

// ---------- EventBus ----------

// EventBus allows plugins and engine systems to communicate via pub/sub.
type EventBus interface {
	// Subscribe registers a handler for the given event type.
	Subscribe(eventType string, handler Handler) (subscriptionID string)
	// Unsubscribe removes a handler by its subscription ID.
	Unsubscribe(subscriptionID string)
	// Publish dispatches an event to all registered handlers synchronously.
	Publish(event Event)
}

type subscription struct {
	id        string
	eventType string
	handler   Handler
}

type eventBus struct {
	mu      sync.RWMutex
	subs    map[string]*subscription // subscriptionID → subscription
	byType  map[string][]string      // eventType → []subscriptionID
	counter uint64
}

// NewEventBus creates a new EventBus.
func NewEventBus() EventBus {
	return &eventBus{
		subs:   make(map[string]*subscription),
		byType: make(map[string][]string),
	}
}

func (eb *eventBus) Subscribe(eventType string, handler Handler) string {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.counter++
	id := fmt.Sprintf("sub-%d", eb.counter)
	sub := &subscription{id: id, eventType: eventType, handler: handler}
	eb.subs[id] = sub
	eb.byType[eventType] = append(eb.byType[eventType], id)
	return id
}

func (eb *eventBus) Unsubscribe(subscriptionID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	sub, ok := eb.subs[subscriptionID]
	if !ok {
		return
	}
	delete(eb.subs, subscriptionID)

	ids := eb.byType[sub.eventType]
	filtered := ids[:0]
	for _, id := range ids {
		if id != subscriptionID {
			filtered = append(filtered, id)
		}
	}
	eb.byType[sub.eventType] = filtered
}

func (eb *eventBus) Publish(event Event) {
	eb.mu.RLock()
	ids := make([]string, len(eb.byType[event.Type]))
	copy(ids, eb.byType[event.Type])
	eb.mu.RUnlock()

	for _, id := range ids {
		eb.mu.RLock()
		sub, ok := eb.subs[id]
		eb.mu.RUnlock()
		if ok {
			sub.handler(event)
		}
	}
}

// ---------- Hook ----------

// HookPhase identifies the GameLoop phase a Hook is attached to.
type HookPhase = engine.TickPhase

// HookFunc is the function injected into a GameLoop phase.
type HookFunc func(phase HookPhase, delta float64)

// Hook allows a Plugin to inject logic into a specific GameLoop phase.
type Hook struct {
	Phase    HookPhase
	Priority int // lower value = earlier execution
	Fn       HookFunc
}

// ---------- Plugin ----------

// Plugin defines the lifecycle interface for engine plugins.
type Plugin interface {
	// Name returns the unique plugin identifier.
	Name() string
	// Required returns true if the engine must abort startup on init failure.
	Required() bool
	// Init is called once during engine startup. Returns error on failure.
	Init(bus EventBus) error
	// Start is called after all plugins are initialised.
	Start() error
	// Stop is called during engine shutdown.
	Stop() error
	// Hooks returns the GameLoop hooks this plugin wants to register.
	Hooks() []*Hook
}

// ---------- PluginManager ----------

// PluginManager manages plugin lifecycle and hook registration.
type PluginManager interface {
	// Register adds a plugin. Must be called before LoadAll.
	Register(p Plugin) error
	// LoadAll initialises and starts all registered plugins in order.
	// Required plugins that fail Init cause LoadAll to return an error.
	// Optional plugins that fail Init are skipped with a warning.
	LoadAll(bus EventBus) error
	// StopAll stops all running plugins in reverse registration order.
	StopAll() error
	// GetPlugin returns a registered plugin by name.
	GetPlugin(name string) (Plugin, bool)
	// GetHooks returns all hooks for a given phase, sorted by priority.
	GetHooks(phase HookPhase) []*Hook
	// PluginCount returns the number of registered plugins.
	PluginCount() int
}

type pluginManager struct {
	mu      sync.RWMutex
	plugins []Plugin            // ordered by registration
	byName  map[string]Plugin
	hooks   map[HookPhase][]*Hook
	running []Plugin // plugins that started successfully (for StopAll)
}

// NewPluginManager creates a new PluginManager.
func NewPluginManager() PluginManager {
	return &pluginManager{
		byName: make(map[string]Plugin),
		hooks:  make(map[HookPhase][]*Hook),
	}
}

func (pm *pluginManager) Register(p Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.byName[p.Name()]; exists {
		return ErrPluginExists
	}
	pm.plugins = append(pm.plugins, p)
	pm.byName[p.Name()] = p
	return nil
}

func (pm *pluginManager) LoadAll(bus EventBus) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, p := range pm.plugins {
		if err := p.Init(bus); err != nil {
			if p.Required() {
				return fmt.Errorf("%w: %s: %v", ErrPluginInitFailed, p.Name(), err)
			}
			// Optional plugin — skip silently.
			continue
		}

		if err := p.Start(); err != nil {
			if p.Required() {
				return fmt.Errorf("plugin %s failed to start: %v", p.Name(), err)
			}
			continue
		}

		pm.running = append(pm.running, p)

		// Register hooks.
		for _, h := range p.Hooks() {
			pm.hooks[h.Phase] = insertHook(pm.hooks[h.Phase], h)
		}
	}
	return nil
}

// insertHook inserts h into the slice maintaining priority order (ascending).
func insertHook(hooks []*Hook, h *Hook) []*Hook {
	hooks = append(hooks, h)
	// Simple insertion sort — hook lists are small.
	for i := len(hooks) - 1; i > 0 && hooks[i].Priority < hooks[i-1].Priority; i-- {
		hooks[i], hooks[i-1] = hooks[i-1], hooks[i]
	}
	return hooks
}

func (pm *pluginManager) StopAll() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Stop in reverse order.
	var firstErr error
	for i := len(pm.running) - 1; i >= 0; i-- {
		if err := pm.running[i].Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	pm.running = nil
	return firstErr
}

func (pm *pluginManager) GetPlugin(name string) (Plugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.byName[name]
	return p, ok
}

func (pm *pluginManager) GetHooks(phase HookPhase) []*Hook {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	hooks := pm.hooks[phase]
	result := make([]*Hook, len(hooks))
	copy(result, hooks)
	return result
}

func (pm *pluginManager) PluginCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.plugins)
}
