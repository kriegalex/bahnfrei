// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Event is one server-sent event. Name becomes the SSE "event:" field
// (empty means the client's default "message" handler fires); Data becomes
// one or more "data:" lines (split on '\n' per the SSE wire format).
type Event struct {
	Name string
	Data string
}

// subscriberBuffer bounds how many pending events one slow subscriber can
// hold before Publish starts dropping its oldest-undelivered events. Live
// results are a "latest state wins" stream (SYS-071's budget is about
// freshness, not about replaying every intermediate frame): a client that
// falls behind is better served by dropping to the newest events than by
// blocking every other subscriber or unbounded memory growth.
const subscriberBuffer = 32

// Bus is a topic-based publish/subscribe broker for live updates over SSE
// (ADR-003: "SSE covers the ≤10s public-update budget without websocket
// infrastructure"). It is transport-agnostic — Handler adapts it to
// net/http — so the same Bus can be exercised directly in tests without a
// real HTTP round trip.
type Bus struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{}
}

// NewBus builds an empty broker.
func NewBus() *Bus {
	return &Bus{subs: make(map[string]map[chan Event]struct{})}
}

// Subscribe registers a new subscriber to topic and returns a channel of
// events plus a cancel function the caller MUST call when done (typically
// via defer) to release the subscription and stop the channel from being
// written to.
func (b *Bus) Subscribe(topic string) (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	set, ok := b.subs[topic]
	if !ok {
		set = make(map[chan Event]struct{})
		b.subs[topic] = set
	}
	set[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if set, ok := b.subs[topic]; ok {
			if _, present := set[ch]; present {
				delete(set, ch)
				close(ch)
			}
			if len(set) == 0 {
				delete(b.subs, topic)
			}
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

// Publish delivers ev to every current subscriber of topic. A subscriber
// whose buffer is full has its oldest event dropped to make room (see
// subscriberBuffer) rather than blocking the publisher.
func (b *Bus) Publish(topic string, ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[topic] {
		select {
		case ch <- ev:
		default:
			// Buffer full: drop the oldest pending event, then retry once.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
				// Subscriber's consumer is not keeping up even after
				// dropping one slot; skip it for this publish rather than
				// spin or block.
			}
		}
	}
}

// SubscriberCount reports how many live subscribers topic currently has
// (diagnostics/tests).
func (b *Bus) SubscriberCount(topic string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs[topic])
}

// writeEvent serializes ev in SSE wire format to w and flushes.
func writeEvent(w http.ResponseWriter, flusher http.Flusher, ev Event) error {
	var b strings.Builder
	if ev.Name != "" {
		fmt.Fprintf(&b, "event: %s\n", ev.Name)
	}
	for _, line := range strings.Split(ev.Data, "\n") {
		fmt.Fprintf(&b, "data: %s\n", line)
	}
	b.WriteString("\n")
	if _, err := w.Write([]byte(b.String())); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// sseHandler serves topic (taken from the request by topicOf) as a live
// text/event-stream, replaying Bus events until the client disconnects.
func sseHandler(bus *Bus, topicOf func(*http.Request) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		topic := topicOf(r)
		ch, cancel := bus.Subscribe(topic)
		defer cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, open := <-ch:
				if !open {
					return
				}
				if err := writeEvent(w, flusher, ev); err != nil {
					return
				}
			}
		}
	}
}
