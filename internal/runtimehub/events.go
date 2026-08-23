package runtimehub

import (
	"context"
	"sync"
)

// EventType enumerates domain events that invalidate runtime caches.
// It is a tiny synchronous, in-process dispatcher – no broker, no async.
type EventType string

const (
	EventEntryPublished        EventType = "EntryPublished"
	EventEntryRouteChanged     EventType = "EntryRouteChanged"
	EventMediaUpdated          EventType = "MediaUpdated"
	EventThemeUpdated          EventType = "ThemeUpdated"
	EventNavigationUpdated     EventType = "NavigationUpdated"
	EventSiteSettingsUpdated   EventType = "SiteSettingsUpdated"
	EventLayoutTemplatePublished EventType = "LayoutTemplatePublished"
)

// Event carries a domain event. Payload is optional and type-asserted by handlers.
type Event struct {
	Type    EventType
	Payload any
}

// Handler is a synchronous event handler.
type Handler func(Event)

// Dispatcher holds subscribers and dispatches events synchronously.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

// NewDispatcher creates an empty dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[EventType][]Handler)}
}

// Subscribe registers a handler for the given event type.
func (d *Dispatcher) Subscribe(t EventType, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[t] = append(d.handlers[t], h)
}

// Dispatch calls all handlers for the event type synchronously.
// It is safe to call from any write path; handlers must not block.
func (d *Dispatcher) Dispatch(e Event) {
	d.mu.RLock()
	handlers := append([]Handler(nil), d.handlers[e.Type]...)
	d.mu.RUnlock()
	for _, h := range handlers {
		h(e)
	}
}

// WireRuntime subscribes the Runtime's cache invalidation to domain events.
func (r *Runtime) WireDispatcher(d *Dispatcher) {
	d.Subscribe(EventEntryPublished, func(Event) { r.InvalidateContent() })
	d.Subscribe(EventEntryRouteChanged, func(Event) { r.InvalidateContent() })
	d.Subscribe(EventMediaUpdated, func(Event) { r.InvalidateMediaAll() })
	d.Subscribe(EventThemeUpdated, func(Event) { _ = r.ReloadTheme(context.Background()) })
	d.Subscribe(EventNavigationUpdated, func(Event) { _ = r.ReloadNavigation(context.Background()) })
	d.Subscribe(EventSiteSettingsUpdated, func(Event) { _ = r.ReloadSite(context.Background()) })
	d.Subscribe(EventLayoutTemplatePublished, func(Event) { r.InvalidateLayoutTemplates() })
}
