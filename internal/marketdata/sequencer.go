package marketdata

// Disposition is the result of checking one event against stream sequence state.
type Disposition string

// Sequencer outcomes; gaps require reconnect and snapshot recovery.
const (
	Accepted   Disposition = "accepted"
	Duplicate  Disposition = "duplicate"
	OutOfOrder Disposition = "out_of_order"
	Gap        Disposition = "gap"
)

// GapObservation records the expected and received sequence for diagnostics.
type GapObservation struct {
	StreamID string
	Symbol   string
	Expected uint64
	Received uint64
}

// SequenceObservation contains a disposition and optional gap evidence.
type SequenceObservation struct {
	Disposition Disposition
	Gap         GapObservation
}

type sequenceState struct {
	streamID string
	last     uint64
}

// Sequencer tracks the last accepted sequence independently for each symbol.
type Sequencer struct {
	bySymbol map[string]sequenceState
}

// NewSequencer returns an empty sequence state machine.
func NewSequencer() *Sequencer {
	return &Sequencer{bySymbol: make(map[string]sequenceState)}
}

// Observe accepts new streams only from a snapshot and never skips a gap.
func (s *Sequencer) Observe(event Event) SequenceObservation {
	state, exists := s.bySymbol[event.Symbol]
	if !exists || state.streamID != event.StreamID {
		if event.Type != EventBookSnapshot {
			return gapObservation(event, 1)
		}
		s.bySymbol[event.Symbol] = sequenceState{streamID: event.StreamID, last: event.Sequence}
		return SequenceObservation{Disposition: Accepted}
	}

	switch {
	case event.Sequence == state.last:
		return SequenceObservation{Disposition: Duplicate}
	case event.Sequence < state.last:
		return SequenceObservation{Disposition: OutOfOrder}
	case event.Sequence > state.last+1:
		return gapObservation(event, state.last+1)
	default:
		state.last = event.Sequence
		s.bySymbol[event.Symbol] = state
		return SequenceObservation{Disposition: Accepted}
	}
}

func gapObservation(event Event, expected uint64) SequenceObservation {
	return SequenceObservation{
		Disposition: Gap,
		Gap:         GapObservation{StreamID: event.StreamID, Symbol: event.Symbol, Expected: expected, Received: event.Sequence},
	}
}
