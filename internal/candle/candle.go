// Package candle builds exact event-time OHLC candles and deterministic rollups.
package candle

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// Interval is a supported candle width in seconds.
type Interval int32

// Supported base and rollup intervals.
const (
	OneMinute   Interval = 60
	FiveMinutes Interval = 300
	OneHour     Interval = 3600
)

// ParseInterval converts the public interval token into its exact duration.
func ParseInterval(value string) (Interval, error) {
	switch value {
	case "1m":
		return OneMinute, nil
	case "5m":
		return FiveMinutes, nil
	case "1h":
		return OneHour, nil
	default:
		return 0, fmt.Errorf("parse candle interval: unsupported interval")
	}
}

// String returns the public interval token or an empty string for an invalid value.
func (i Interval) String() string {
	switch i {
	case OneMinute:
		return "1m"
	case FiveMinutes:
		return "5m"
	case OneHour:
		return "1h"
	default:
		return ""
	}
}

// Duration returns the interval width.
func (i Interval) Duration() time.Duration {
	return time.Duration(i) * time.Second
}

// Trade is the exact event-time input used by candle aggregation.
type Trade struct {
	StreamID      string
	TradeID       string
	Symbol        string
	Sequence      uint64
	EventTime     time.Time
	PriceAtoms    int64
	PriceScale    uint8
	QuantityAtoms int64
	QuantityScale uint8
}

// Candle stores exact OHLCV atoms plus ordering evidence for late trades.
type Candle struct {
	Symbol         string
	Interval       Interval
	OpenTime       time.Time
	PriceScale     uint8
	QuantityScale  uint8
	OpenAtoms      int64
	HighAtoms      int64
	LowAtoms       int64
	CloseAtoms     int64
	VolumeAtoms    int64
	TradeCount     int64
	Final          bool
	FirstTradeTime time.Time
	LastTradeTime  time.Time
	FirstSequence  uint64
	LastSequence   uint64
}

// View is the decimal-string candle contract exposed over JSON.
type View struct {
	Symbol     string    `json:"symbol"`
	Interval   string    `json:"interval"`
	OpenTime   time.Time `json:"open_time"`
	CloseTime  time.Time `json:"close_time"`
	Open       string    `json:"open"`
	High       string    `json:"high"`
	Low        string    `json:"low"`
	Close      string    `json:"close"`
	Volume     string    `json:"volume"`
	TradeCount int64     `json:"trade_count"`
	Final      bool      `json:"is_final"`
}

var candleSymbolPattern = regexp.MustCompile(`^[A-Z0-9]{4,24}-PERP$`)

// ApplyTrade creates or updates one bucket, preserving event-time open and close.
func ApplyTrade(current *Candle, trade Trade, interval Interval) (Candle, error) {
	if err := validateTrade(trade); err != nil {
		return Candle{}, err
	}
	if interval.String() == "" {
		return Candle{}, fmt.Errorf("apply trade: invalid interval")
	}
	bucket := BucketStart(trade.EventTime, interval)
	if current == nil {
		return Candle{
			Symbol: trade.Symbol, Interval: interval, OpenTime: bucket,
			PriceScale: trade.PriceScale, QuantityScale: trade.QuantityScale,
			OpenAtoms: trade.PriceAtoms, HighAtoms: trade.PriceAtoms, LowAtoms: trade.PriceAtoms, CloseAtoms: trade.PriceAtoms,
			VolumeAtoms: trade.QuantityAtoms, TradeCount: 1,
			FirstTradeTime: trade.EventTime.UTC(), LastTradeTime: trade.EventTime.UTC(),
			FirstSequence: trade.Sequence, LastSequence: trade.Sequence,
		}, nil
	}
	result := *current
	if result.Symbol != trade.Symbol || result.Interval != interval || !result.OpenTime.Equal(bucket) || result.PriceScale != trade.PriceScale || result.QuantityScale != trade.QuantityScale {
		return Candle{}, fmt.Errorf("apply trade: candle identity mismatch")
	}
	if result.VolumeAtoms > math.MaxInt64-trade.QuantityAtoms || result.TradeCount == math.MaxInt64 {
		return Candle{}, fmt.Errorf("apply trade: aggregate overflow")
	}
	if trade.PriceAtoms > result.HighAtoms {
		result.HighAtoms = trade.PriceAtoms
	}
	if trade.PriceAtoms < result.LowAtoms {
		result.LowAtoms = trade.PriceAtoms
	}
	eventTime := trade.EventTime.UTC()
	if eventTime.Before(result.FirstTradeTime) || (eventTime.Equal(result.FirstTradeTime) && trade.Sequence < result.FirstSequence) {
		result.OpenAtoms = trade.PriceAtoms
		result.FirstTradeTime = eventTime
		result.FirstSequence = trade.Sequence
	}
	if eventTime.After(result.LastTradeTime) || (eventTime.Equal(result.LastTradeTime) && trade.Sequence > result.LastSequence) {
		result.CloseAtoms = trade.PriceAtoms
		result.LastTradeTime = eventTime
		result.LastSequence = trade.Sequence
	}
	result.VolumeAtoms += trade.QuantityAtoms
	result.TradeCount++
	return result, nil
}

// BucketStart floors a UTC event time to its interval boundary.
func BucketStart(eventTime time.Time, interval Interval) time.Time {
	seconds := int64(interval)
	if seconds <= 0 {
		return time.Time{}
	}
	unix := eventTime.UTC().Unix()
	return time.Unix(unix-unix%seconds, 0).UTC()
}

// View validates a candle and renders exact decimal strings for transport.
func (c Candle) View() (View, error) {
	if err := c.validate(); err != nil {
		return View{}, err
	}
	open, err := trading.FormatDecimal(c.OpenAtoms, c.PriceScale)
	if err != nil {
		return View{}, err
	}
	high, _ := trading.FormatDecimal(c.HighAtoms, c.PriceScale)
	low, _ := trading.FormatDecimal(c.LowAtoms, c.PriceScale)
	closeValue, _ := trading.FormatDecimal(c.CloseAtoms, c.PriceScale)
	volume, err := trading.FormatDecimal(c.VolumeAtoms, c.QuantityScale)
	if err != nil {
		return View{}, err
	}
	return View{
		Symbol: c.Symbol, Interval: c.Interval.String(), OpenTime: c.OpenTime.UTC(), CloseTime: c.OpenTime.Add(c.Interval.Duration()).UTC(),
		Open: open, High: high, Low: low, Close: closeValue, Volume: volume,
		TradeCount: c.TradeCount, Final: c.Final,
	}, nil
}

func (c Candle) validate() error {
	if !candleSymbolPattern.MatchString(c.Symbol) || c.Interval.String() == "" || c.OpenTime.IsZero() || !BucketStart(c.OpenTime, c.Interval).Equal(c.OpenTime.UTC()) ||
		c.PriceScale > 18 || c.QuantityScale > 18 || c.OpenAtoms <= 0 || c.HighAtoms <= 0 || c.LowAtoms <= 0 || c.CloseAtoms <= 0 ||
		c.HighAtoms < c.OpenAtoms || c.HighAtoms < c.CloseAtoms || c.HighAtoms < c.LowAtoms ||
		c.LowAtoms > c.OpenAtoms || c.LowAtoms > c.CloseAtoms || c.VolumeAtoms < 0 || c.TradeCount < 0 {
		return fmt.Errorf("validate candle: invalid candle")
	}
	return nil
}

func validateTrade(trade Trade) error {
	if !candleSymbolPattern.MatchString(trade.Symbol) || trade.StreamID == "" || len(trade.StreamID) > 64 ||
		trade.TradeID == "" || len(trade.TradeID) > 64 || trade.Sequence == 0 || trade.EventTime.IsZero() ||
		trade.PriceAtoms <= 0 || trade.QuantityAtoms <= 0 || trade.PriceScale > 18 || trade.QuantityScale > 18 {
		return fmt.Errorf("validate candle trade: invalid trade")
	}
	return nil
}

// Rollup combines ordered one-minute candles into a five-minute or hourly candle.
func Rollup(source []Candle, target Interval) (Candle, error) {
	if len(source) == 0 || (target != FiveMinutes && target != OneHour) {
		return Candle{}, fmt.Errorf("roll up candles: invalid input")
	}
	items := append([]Candle(nil), source...)
	sort.Slice(items, func(i, j int) bool { return items[i].OpenTime.Before(items[j].OpenTime) })
	first := items[0]
	targetStart := BucketStart(first.OpenTime, target)
	if first.Interval != OneMinute {
		return Candle{}, fmt.Errorf("roll up candles: source interval must be 1m")
	}
	result := Candle{
		Symbol: first.Symbol, Interval: target, OpenTime: targetStart,
		PriceScale: first.PriceScale, QuantityScale: first.QuantityScale,
		OpenAtoms: first.OpenAtoms, HighAtoms: first.HighAtoms, LowAtoms: first.LowAtoms, CloseAtoms: first.CloseAtoms,
		VolumeAtoms: first.VolumeAtoms, TradeCount: first.TradeCount,
		FirstTradeTime: first.FirstTradeTime, LastTradeTime: first.LastTradeTime,
		FirstSequence: first.FirstSequence, LastSequence: first.LastSequence,
	}
	contiguousAndFinal := first.Final
	for index := 1; index < len(items); index++ {
		item := items[index]
		if item.Interval != OneMinute || item.Symbol != result.Symbol || item.PriceScale != result.PriceScale || item.QuantityScale != result.QuantityScale ||
			!BucketStart(item.OpenTime, target).Equal(result.OpenTime) {
			return Candle{}, fmt.Errorf("roll up candles: source identity mismatch")
		}
		if !item.OpenTime.Equal(items[index-1].OpenTime.Add(time.Minute)) {
			contiguousAndFinal = false
		}
		if result.VolumeAtoms > math.MaxInt64-item.VolumeAtoms || result.TradeCount > math.MaxInt64-item.TradeCount {
			return Candle{}, fmt.Errorf("roll up candles: aggregate overflow")
		}
		if item.HighAtoms > result.HighAtoms {
			result.HighAtoms = item.HighAtoms
		}
		if item.LowAtoms < result.LowAtoms {
			result.LowAtoms = item.LowAtoms
		}
		result.CloseAtoms = item.CloseAtoms
		result.VolumeAtoms += item.VolumeAtoms
		result.TradeCount += item.TradeCount
		result.LastTradeTime = item.LastTradeTime
		result.LastSequence = item.LastSequence
		contiguousAndFinal = contiguousAndFinal && item.Final
	}
	wantCount := int(target / OneMinute)
	result.Final = contiguousAndFinal && len(items) == wantCount && first.OpenTime.Equal(targetStart)
	if err := result.validate(); err != nil {
		return Candle{}, err
	}
	return result, nil
}

// RepairGaps fills missing buckets with final zero-volume candles at the prior close.
func RepairGaps(source []Candle, start, end time.Time, interval Interval) ([]Candle, error) {
	if interval.String() == "" || !start.Before(end) || !BucketStart(start, interval).Equal(start.UTC()) || !BucketStart(end, interval).Equal(end.UTC()) {
		return nil, fmt.Errorf("repair candle gaps: invalid range")
	}
	items := append([]Candle(nil), source...)
	sort.Slice(items, func(i, j int) bool { return items[i].OpenTime.Before(items[j].OpenTime) })
	byTime := make(map[int64]Candle, len(items))
	for _, item := range items {
		if item.Interval != interval || item.OpenTime.Before(start) || !item.OpenTime.Before(end) {
			return nil, fmt.Errorf("repair candle gaps: source outside range")
		}
		byTime[item.OpenTime.Unix()] = item
	}
	result := make([]Candle, 0, int(end.Sub(start)/interval.Duration()))
	var previous *Candle
	for cursor := start.UTC(); cursor.Before(end); cursor = cursor.Add(interval.Duration()) {
		if item, ok := byTime[cursor.Unix()]; ok {
			copy := item
			result = append(result, copy)
			previous = &result[len(result)-1]
			continue
		}
		if previous == nil {
			return nil, fmt.Errorf("repair candle gaps: previous close is unavailable")
		}
		flat := Candle{
			Symbol: previous.Symbol, Interval: interval, OpenTime: cursor,
			PriceScale: previous.PriceScale, QuantityScale: previous.QuantityScale,
			OpenAtoms: previous.CloseAtoms, HighAtoms: previous.CloseAtoms, LowAtoms: previous.CloseAtoms, CloseAtoms: previous.CloseAtoms,
			VolumeAtoms: 0, TradeCount: 0, Final: true,
			FirstTradeTime: cursor, LastTradeTime: cursor,
		}
		result = append(result, flat)
		previous = &result[len(result)-1]
	}
	return result, nil
}
