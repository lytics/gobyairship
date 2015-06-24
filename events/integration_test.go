package events_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lytics/gobyairship"
	"github.com/lytics/gobyairship/events"
)

func TestEventsIntegration(t *testing.T) {
	if os.Getenv("UA_CREDS") == "" {
		t.Skip("Skipping integration test as UA_CREDS isn't set.")
	}
	parts := strings.SplitN(os.Getenv("UA_CREDS"), ":", 2)
	if len(parts) != 2 {
		t.Skip("Invalid UA_CREDS. Expected <app key>:<master secret>")
	}
	client := gobyairship.NewClient(parts[0], parts[1])

	// Allow overriding the events url
	events.SetURL(os.Getenv("UA_EVENTS_URL"))

	start := time.Now()
	resp, err := events.Fetch(client, events.StartFirst, 0, nil)
	if err != nil {
		t.Fatalf("Error fetching events from %s: %v", events.SetURL(""), err)
	}
	if len(resp.ID) < 2 {
		// Just logging for now since the UA API doesn't return it
		//t.Errorf("Invalid/missing response ID: %q", resp.ID)
		t.Logf("Invalid/missing response ID: %q", resp.ID)
	}

	// Consume events for up to 15s or when there's a 3s pause
	last := time.Now()
	events := 0
	deadline := time.Now().Add(15 * time.Second)

	maxpause := 3 * time.Second
	timer := time.NewTimer(maxpause)
	defer timer.Stop()

consume:
	for time.Now().Before(deadline) {
		select {
		case ev, ok := <-resp.Events():
			if !ok {
				break consume
			}
			events++
			checkEvent(t, ev.Type, ev)
			timer.Reset(maxpause)
			last = time.Now()
		case <-timer.C:
			break consume
		}
	}
	resp.Close()

	if events == 0 {
		t.Error("No events received.")
	} else {
		t.Logf("Processed %d events in %s", events, last.Sub(start))
	}
}
