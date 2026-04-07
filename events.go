package goddgs

import "time"

type EventType string

const (
	EventSearchStart   EventType = "search_start"
	EventSearchFinish  EventType = "search_finish"
	EventProviderStart EventType = "provider_start"
	EventProviderEnd   EventType = "provider_end"
	EventBlocked       EventType = "blocked"
	EventFallback      EventType = "fallback"
)

// Event is emitted by the engine for hooks/metrics.
type Event struct {
	Type      EventType
	Provider  string
	Duration  time.Duration
	ErrKind   ErrorKind
	Success   bool
	Block     *BlockInfo
	QueryHash string
}

type EventHook func(Event)
