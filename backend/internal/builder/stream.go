package builder

import (
	"sync"
	"time"
)

const defaultSubscriberBuffer = 16

// Event is a bounded lifecycle record suitable for a future SSE/WebSocket
// bridge. It deliberately carries no raw command output, source filename, or
// credential-bearing URL.
type Event struct {
	Sequence uint64
	At       time.Time
	State    State
	Message  string
}

// Stream fans out ordered lifecycle records to local subscribers. It is not a
// durable log store: a future adapter must persist redacted records before
// exposing them across processes or reconnecting clients.
type Stream struct {
	mu          sync.Mutex
	next        uint64
	closed      bool
	subscribers map[chan Event]struct{}
}

// NewStream creates a stream for one build request.
func NewStream() *Stream {
	return &Stream{subscribers: make(map[chan Event]struct{})}
}

// Subscribe starts receiving subsequent events. Slow subscribers are detached
// instead of being allowed to block or exhaust the isolated builder.
func (s *Stream) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = defaultSubscriberBuffer
	}
	ch := make(chan Event, buffer)
	s.mu.Lock()
	if s.closed {
		close(ch)
		s.mu.Unlock()
		return ch, func() {}
	}
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.mu.Lock()
			if _, ok := s.subscribers[ch]; ok {
				delete(s.subscribers, ch)
				close(ch)
			}
			s.mu.Unlock()
		})
	}
}

// Publish delivers a lifecycle record. A slow in-process observer is detached;
// a later durable log store is the source of truth for browser reconnection.
func (s *Stream) Publish(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.next++
	event.Sequence = s.next
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			delete(s.subscribers, ch)
			close(ch)
		}
	}
}

// Close terminates all subscriptions after the build reaches a terminal state.
func (s *Stream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, ch)
	}
}
