package marketdata_test

import (
	"testing"

	"exchange-lab/internal/marketdata"
)

func TestSequencerClassifiesContinuousDuplicateOutOfOrderAndGap(t *testing.T) {
	t.Parallel()

	tracker := marketdata.NewSequencer()
	tests := []struct {
		event marketdata.Event
		want  marketdata.Disposition
	}{
		{event: sequenceEvent("stream-a", marketdata.EventBookSnapshot, 1), want: marketdata.Accepted},
		{event: sequenceEvent("stream-a", marketdata.EventTicker, 2), want: marketdata.Accepted},
		{event: sequenceEvent("stream-a", marketdata.EventTicker, 2), want: marketdata.Duplicate},
		{event: sequenceEvent("stream-a", marketdata.EventTrade, 1), want: marketdata.OutOfOrder},
		{event: sequenceEvent("stream-a", marketdata.EventBookDelta, 4), want: marketdata.Gap},
	}
	for _, test := range tests {
		got := tracker.Observe(test.event)
		if got.Disposition != test.want {
			t.Fatalf("Observe(sequence=%d) = %#v, want %s", test.event.Sequence, got, test.want)
		}
	}
}

func TestSequencerRequiresSnapshotForNewStream(t *testing.T) {
	t.Parallel()

	tracker := marketdata.NewSequencer()
	if got := tracker.Observe(sequenceEvent("stream-a", marketdata.EventTicker, 1)); got.Disposition != marketdata.Gap {
		t.Fatalf("first non-snapshot = %#v, want gap", got)
	}
	if got := tracker.Observe(sequenceEvent("stream-a", marketdata.EventBookSnapshot, 7)); got.Disposition != marketdata.Accepted {
		t.Fatalf("first snapshot = %#v, want accepted", got)
	}
	if got := tracker.Observe(sequenceEvent("stream-b", marketdata.EventTicker, 1)); got.Disposition != marketdata.Gap {
		t.Fatalf("new stream non-snapshot = %#v, want gap", got)
	}
	if got := tracker.Observe(sequenceEvent("stream-b", marketdata.EventBookSnapshot, 1)); got.Disposition != marketdata.Accepted {
		t.Fatalf("new stream snapshot = %#v, want accepted", got)
	}
}

func sequenceEvent(streamID string, kind marketdata.EventType, sequence uint64) marketdata.Event {
	return marketdata.Event{StreamID: streamID, Type: kind, Symbol: "BTCUSDT-PERP", Sequence: sequence}
}
