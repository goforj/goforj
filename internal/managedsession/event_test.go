package managedsession

import (
	"reflect"
	"strings"
	"testing"
)

// managedSessionEventFixture returns a valid pre-decoration output event for protocol tests.
func managedSessionEventFixture() Event {
	return Event{
		SchemaVersion: SchemaVersion,
		ProjectID:     "orders",
		SessionID:     "session-orders",
		Sequence:      42,
		Timestamp:     "2026-07-21T04:51:00Z",
		Kind:          EventKindLogChunk,
		AppID:         "api",
		WatcherID:     "api.runtime",
		Stream:        EventStreamStdout,
		Text:          "ready\n",
	}
}

// TestManagedSessionEventRoundTripsBoundedRecords proves both output and explicit gap records retain their wire shape.
func TestManagedSessionEventRoundTripsBoundedRecords(t *testing.T) {
	logEvent := managedSessionEventFixture()
	gapEvent := Event{
		SchemaVersion: SchemaVersion,
		ProjectID:     "orders",
		SessionID:     "session-orders",
		Sequence:      46,
		Timestamp:     "2026-07-21T04:51:01Z",
		Kind:          EventKindOutputGap,
		WatcherID:     "api.runtime",
		Stream:        EventStreamStdout,
		DroppedFrom:   43,
		DroppedTo:     45,
		DroppedCount:  3,
	}
	for _, test := range []struct {
		name  string
		event Event
	}{
		{name: "log chunk", event: logEvent},
		{name: "output gap", event: gapEvent},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := MarshalEvent(test.event)
			if err != nil {
				t.Fatalf("marshal error = %v", err)
			}
			got, err := DecodeEvent(payload)
			if err != nil {
				t.Fatalf("decode error = %v; payload = %s", err, payload)
			}
			if !reflect.DeepEqual(got, test.event) {
				t.Fatalf("decoded = %#v, want %#v", got, test.event)
			}
		})
	}
}

// TestManagedSessionEventValidationRejectsLossyOrAmbiguousRecords keeps future replay explicit about ordering and drops.
func TestManagedSessionEventValidationRejectsLossyOrAmbiguousRecords(t *testing.T) {
	valid := managedSessionEventFixture()
	tests := []struct {
		name   string
		mutate func(*Event)
	}{
		{name: "sequence", mutate: func(event *Event) { event.Sequence = 0 }},
		{name: "timestamp zone", mutate: func(event *Event) { event.Timestamp = "2026-07-21T04:51:00+01:00" }},
		{name: "source", mutate: func(event *Event) { event.AppID, event.WatcherID = "", "" }},
		{name: "stream", mutate: func(event *Event) { event.Stream = "combined" }},
		{name: "empty text", mutate: func(event *Event) { event.Text = "" }},
		{name: "oversized text", mutate: func(event *Event) { event.Text = strings.Repeat("x", maximumManagedSessionEventText+1) }},
		{name: "unexpected gap", mutate: func(event *Event) { event.DroppedFrom, event.DroppedTo, event.DroppedCount = 1, 1, 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			test.mutate(&event)
			if err := event.Validate(); err == nil {
				t.Fatal("invalid event passed validation")
			}
		})
	}

	gap := valid
	gap.Kind = EventKindOutputGap
	gap.Text = ""
	gap.DroppedFrom, gap.DroppedTo, gap.DroppedCount = 2, 3, 1
	if err := gap.Validate(); err == nil {
		t.Fatal("gap with mismatched count passed validation")
	}
	gap.DroppedFrom, gap.DroppedTo, gap.DroppedCount = 42, 42, 1
	if err := gap.Validate(); err == nil {
		t.Fatal("gap that reaches its own sequence passed validation")
	}
}

// TestManagedSessionEventDecoderRemainsStrict prevents duplicate, unknown, and trailing fields from entering the event stream.
func TestManagedSessionEventDecoderRemainsStrict(t *testing.T) {
	event := managedSessionEventFixture()
	payload, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	valid := strings.TrimSuffix(string(payload), "}")
	for _, invalid := range []string{
		valid + `,"unknown":true}`,
		valid + `,"sequence":42}`,
		string(payload) + `{}`,
	} {
		if _, err := DecodeEvent([]byte(invalid)); err == nil {
			t.Fatalf("event decoder accepted invalid payload %s", invalid)
		}
	}
}
