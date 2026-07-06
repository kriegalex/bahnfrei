// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := NewBus()
	ch, cancel := bus.Subscribe("meet-1")
	defer cancel()

	bus.Publish("meet-1", Event{Name: "result", Data: "100m final"})

	select {
	case ev := <-ch:
		if ev.Name != "result" || ev.Data != "100m final" {
			t.Errorf("received %+v, want Name=result Data=\"100m final\"", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func TestBusPublishToUnrelatedTopicNotDelivered(t *testing.T) {
	bus := NewBus()
	ch, cancel := bus.Subscribe("meet-1")
	defer cancel()

	bus.Publish("meet-2", Event{Name: "result", Data: "irrelevant"})

	select {
	case ev := <-ch:
		t.Fatalf("received event from unrelated topic: %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// expected: nothing delivered
	}
}

func TestBusMultipleSubscribersSameTopic(t *testing.T) {
	bus := NewBus()
	ch1, cancel1 := bus.Subscribe("meet-1")
	defer cancel1()
	ch2, cancel2 := bus.Subscribe("meet-1")
	defer cancel2()

	if got := bus.SubscriberCount("meet-1"); got != 2 {
		t.Fatalf("SubscriberCount = %d, want 2", got)
	}

	bus.Publish("meet-1", Event{Name: "ping"})
	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive broadcast event")
		}
	}
}

func TestBusCancelStopsDelivery(t *testing.T) {
	bus := NewBus()
	ch, cancel := bus.Subscribe("meet-1")
	cancel()

	bus.Publish("meet-1", Event{Name: "after-cancel"})

	if _, open := <-ch; open {
		t.Error("channel should be closed after cancel")
	}
	if got := bus.SubscriberCount("meet-1"); got != 0 {
		t.Errorf("SubscriberCount after cancel = %d, want 0", got)
	}
}

func TestBusPublishDropsWhenBufferFull(t *testing.T) {
	bus := NewBus()
	_, cancel := bus.Subscribe("meet-1")
	defer cancel()

	// Publish far more than the buffer holds; Publish must never block on
	// a subscriber that never drains its channel.
	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBuffer*4; i++ {
			bus.Publish("meet-1", Event{Name: "flood"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow/non-consuming subscriber")
	}
}

// TestSSEHandlerStreamsEvents drives sseHandler over a real HTTP
// connection (httptest.Server), reading the live response body as it
// streams — the only race-free way to observe a handler that writes
// concurrently with the test goroutine reading (a bytes.Buffer-backed
// httptest.ResponseRecorder is not safe for that).
func TestSSEHandlerStreamsEvents(t *testing.T) {
	bus := NewBus()
	mux := http.NewServeMux()
	mux.HandleFunc("/events/{topic}", sseHandler(bus, func(r *http.Request) string {
		return r.PathValue("topic")
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events/meet-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events/meet-1: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	waitForSubscriber(t, bus, "meet-1")
	bus.Publish("meet-1", Event{Name: "update", Data: "line one\nline two"})

	reader := bufio.NewReader(resp.Body)
	var got []string
	for len(got) < 3 {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE stream: %v (lines so far: %v)", err, got)
		}
		got = append(got, strings.TrimRight(line, "\n"))
	}

	want := []string{"event: update", "data: line one", "data: line two"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d = %q, want %q (all: %v)", i, got[i], w, got)
		}
	}
}

func waitForSubscriber(t *testing.T, bus *Bus, topic string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bus.SubscriberCount(topic) > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for SSE handler to subscribe")
}
