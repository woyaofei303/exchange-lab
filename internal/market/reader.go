// Package market 组合权威数据源和缓存支持的行情读取模型。
package market

import (
	"context"
	"fmt"
	"time"

	"exchange-lab/internal/trading"
)

// Ticker 是只读行情视图。十进制字段保持为字符串，
// 防止传输层误将其作为二进制浮点数处理。
type Ticker struct {
	Symbol      string
	LastPrice   string
	MarkPrice   string
	IndexPrice  string
	PriceChange string
	Volume      string
	UpdatedAt   time.Time
}

// Reader 是只读行情传输层使用的业务接口。
type Reader interface {
	InstrumentReader
	TickerReader
}

// InstrumentReader 负责读取合约元数据。
type InstrumentReader interface {
	Instruments(context.Context, string) ([]trading.Instrument, error)
}

// TickerReader 负责读取最新行情快照。
type TickerReader interface {
	Tickers(context.Context, string) ([]Ticker, error)
}

// CompositeReader 使交易品种持久化和行情投递可以独立替换，
// 同时向传输层提供统一的 Reader。
type CompositeReader struct {
	instruments InstrumentReader
	tickers     TickerReader
}

// NewCompositeReader 组合相互独立的权威交易品种和行情读取器。
func NewCompositeReader(instruments InstrumentReader, tickers TickerReader) (*CompositeReader, error) {
	if instruments == nil || tickers == nil {
		return nil, fmt.Errorf("create market reader: instrument and ticker readers are required")
	}
	return &CompositeReader{instruments: instruments, tickers: tickers}, nil
}

// Instruments 委托给权威交易品种读取器。
func (r *CompositeReader) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	return r.instruments.Instruments(ctx, symbol)
}

// Tickers 委托给权威行情读取器。
func (r *CompositeReader) Tickers(ctx context.Context, symbol string) ([]Ticker, error) {
	return r.tickers.Tickers(ctx, symbol)
}

// StaticReader 是第 3 周的内存适配器，后续行情功能可在不修改 HTTP 处理器的情况下替换它。
type StaticReader struct {
	instruments []trading.Instrument
	tickers     []Ticker
}

// NewStaticReader 复制输入，防止调用方修改读取器状态。
func NewStaticReader(instruments []trading.Instrument, tickers []Ticker) *StaticReader {
	return &StaticReader{
		instruments: append([]trading.Instrument(nil), instruments...),
		tickers:     append([]Ticker(nil), tickers...),
	}
}

// Instruments 返回全部交易品种，或仅返回精确匹配请求交易对的品种。
func (r *StaticReader) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]trading.Instrument, 0, len(r.instruments))
	for _, instrument := range r.instruments {
		if symbol == "" || instrument.Symbol() == symbol {
			result = append(result, instrument)
		}
	}
	return result, nil
}

// Tickers 返回全部行情，或仅返回精确匹配请求交易对的行情。
func (r *StaticReader) Tickers(ctx context.Context, symbol string) ([]Ticker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]Ticker, 0, len(r.tickers))
	for _, ticker := range r.tickers {
		if symbol == "" || ticker.Symbol == symbol {
			result = append(result, ticker)
		}
	}
	return result, nil
}
