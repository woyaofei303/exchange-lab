package marketdata

// Disposition 表示根据流序列状态检查一个事件的结果。
type Disposition string

// 序列检查结果；出现缺口时需要重连并恢复快照。
const (
	Accepted   Disposition = "accepted"
	Duplicate  Disposition = "duplicate"
	OutOfOrder Disposition = "out_of_order"
	Gap        Disposition = "gap"
)

// GapObservation 记录预期和实际收到的序号，用于诊断。
type GapObservation struct {
	StreamID string
	Symbol   string
	Expected uint64
	Received uint64
}

// SequenceObservation 包含处理结果和可选的缺口证据。
type SequenceObservation struct {
	Disposition Disposition
	Gap         GapObservation
}

type sequenceState struct {
	streamID string
	last     uint64
}

// Sequencer 分别跟踪每个交易对最后接受的序号。
type Sequencer struct {
	bySymbol map[string]sequenceState
}

// NewSequencer 返回空的序列状态机。
func NewSequencer() *Sequencer {
	return &Sequencer{bySymbol: make(map[string]sequenceState)}
}

// Observe 仅从快照开始接受新数据流，绝不跳过序列缺口。
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
