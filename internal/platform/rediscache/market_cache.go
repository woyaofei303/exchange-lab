package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"exchange-lab/internal/market"
	"exchange-lab/internal/trading"
	redisclient "github.com/redis/go-redis/v9"
)

var cacheSymbolPattern = regexp.MustCompile(`^[A-Z0-9]{4,24}-PERP$`)

// MarketCacheConfig 限制每个 Redis 操作和投影的生命周期。
type MarketCacheConfig struct {
	OperationTimeout time.Duration
	InstrumentTTL    time.Duration
	TickerTTL        time.Duration
}

// MarketCache 仅存储可重建的行情读取模型，每个键都有 TTL。
type MarketCache struct {
	client        redisclient.Cmdable
	timeout       time.Duration
	instrumentTTL time.Duration
	tickerTTL     time.Duration
}

// NewMarketCache 校验可重建的行情缓存配置。
func NewMarketCache(client redisclient.Cmdable, config MarketCacheConfig) (*MarketCache, error) {
	if client == nil || config.OperationTimeout <= 0 || config.InstrumentTTL <= 0 || config.TickerTTL <= 0 {
		return nil, fmt.Errorf("create market cache: invalid configuration")
	}
	return &MarketCache{
		client:        client,
		timeout:       config.OperationTimeout,
		instrumentTTL: config.InstrumentTTL,
		tickerTTL:     config.TickerTTL,
	}, nil
}

type cachedInstrument struct {
	Symbol            string `json:"symbol"`
	BaseAsset         string `json:"base_asset"`
	QuoteAsset        string `json:"quote_asset"`
	BaseScale         uint8  `json:"base_scale"`
	QuoteScale        uint8  `json:"quote_scale"`
	PriceScale        uint8  `json:"price_scale"`
	QuantityScale     uint8  `json:"quantity_scale"`
	PriceStepAtoms    int64  `json:"price_step_atoms"`
	QuantityStepAtoms int64  `json:"quantity_step_atoms"`
	MinNotionalAtoms  int64  `json:"min_notional_atoms"`
}

// GetInstruments 返回解码后的交易品种，并区分缓存未命中和故障。
func (c *MarketCache) GetInstruments(ctx context.Context, symbol string) ([]trading.Instrument, bool, error) {
	key, err := marketKey("instruments", symbol)
	if err != nil {
		return nil, false, err
	}
	operationContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	payload, err := c.client.Get(operationContext, key).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read instrument cache: %w", err)
	}
	var cached []cachedInstrument
	if err := json.Unmarshal(payload, &cached); err != nil {
		return nil, false, fmt.Errorf("decode instrument cache: %w", err)
	}
	result := make([]trading.Instrument, 0, len(cached))
	for _, item := range cached {
		instrument, err := decodeInstrument(item)
		if err != nil {
			return nil, false, err
		}
		result = append(result, instrument)
	}
	return result, true, nil
}

// PutInstruments 使用有限 TTL 存储精确最小单位值和精度。
func (c *MarketCache) PutInstruments(ctx context.Context, symbol string, instruments []trading.Instrument) error {
	key, err := marketKey("instruments", symbol)
	if err != nil {
		return err
	}
	cached := make([]cachedInstrument, 0, len(instruments))
	for _, instrument := range instruments {
		if instrument.Symbol() == "" {
			return fmt.Errorf("write instrument cache: invalid instrument")
		}
		cached = append(cached, cachedInstrument{
			Symbol:            instrument.Symbol(),
			BaseAsset:         instrument.Base().Code(),
			QuoteAsset:        instrument.Quote().Code(),
			BaseScale:         instrument.Base().Scale(),
			QuoteScale:        instrument.Quote().Scale(),
			PriceScale:        instrument.PriceScale(),
			QuantityScale:     instrument.QuantityScale(),
			PriceStepAtoms:    instrument.PriceStep().Atoms(),
			QuantityStepAtoms: instrument.QuantityStep().Atoms(),
			MinNotionalAtoms:  instrument.MinNotional().Atoms(),
		})
	}
	payload, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("encode instrument cache: %w", err)
	}
	operationContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.client.Set(operationContext, key, payload, c.instrumentTTL).Err(); err != nil {
		return fmt.Errorf("write instrument cache: %w", err)
	}
	return nil
}

// InvalidateInstruments 删除一个交易对或全部交易品种投影。
func (c *MarketCache) InvalidateInstruments(ctx context.Context, symbol string) error {
	key, err := marketKey("instruments", symbol)
	if err != nil {
		return err
	}
	return c.delete(ctx, key, "invalidate instrument cache")
}

// GetTickers 返回行情投影，并区分缓存未命中和故障。
func (c *MarketCache) GetTickers(ctx context.Context, symbol string) ([]market.Ticker, bool, error) {
	key, err := marketKey("tickers", symbol)
	if err != nil {
		return nil, false, err
	}
	operationContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	payload, err := c.client.Get(operationContext, key).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read ticker cache: %w", err)
	}
	var result []market.Ticker
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, false, fmt.Errorf("decode ticker cache: %w", err)
	}
	return result, true, nil
}

// PutTickers 使用有限 TTL 存储可重建的行情投影。
func (c *MarketCache) PutTickers(ctx context.Context, symbol string, tickers []market.Ticker) error {
	key, err := marketKey("tickers", symbol)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(tickers)
	if err != nil {
		return fmt.Errorf("encode ticker cache: %w", err)
	}
	operationContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.client.Set(operationContext, key, payload, c.tickerTTL).Err(); err != nil {
		return fmt.Errorf("write ticker cache: %w", err)
	}
	return nil
}

// PutTicker 将 MarketCache 适配到行情工作进程的窄化最新行情接收接口。
func (c *MarketCache) PutTicker(ctx context.Context, ticker market.Ticker) error {
	if ticker.Symbol == "" {
		return fmt.Errorf("write ticker cache: invalid ticker")
	}
	if err := c.PutTickers(ctx, ticker.Symbol, []market.Ticker{ticker}); err != nil {
		return err
	}
	return c.PutTickers(ctx, "", []market.Ticker{ticker})
}

// InvalidateTickers 删除一个交易对或全部行情投影。
func (c *MarketCache) InvalidateTickers(ctx context.Context, symbol string) error {
	key, err := marketKey("tickers", symbol)
	if err != nil {
		return err
	}
	return c.delete(ctx, key, "invalidate ticker cache")
}

func (c *MarketCache) delete(ctx context.Context, key, operation string) error {
	operationContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.client.Del(operationContext, key).Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func marketKey(kind, symbol string) (string, error) {
	if symbol == "" {
		return keyNamespace + ":cache:" + kind + ":all", nil
	}
	if !cacheSymbolPattern.MatchString(symbol) {
		return "", fmt.Errorf("create market cache key: invalid symbol")
	}
	return keyNamespace + ":cache:" + kind + ":symbol:" + symbol, nil
}

func decodeInstrument(item cachedInstrument) (trading.Instrument, error) {
	base, err := trading.NewAsset(item.BaseAsset, item.BaseScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("decode instrument cache base: %w", err)
	}
	quote, err := trading.NewAsset(item.QuoteAsset, item.QuoteScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("decode instrument cache quote: %w", err)
	}
	priceStep, err := trading.FormatDecimal(item.PriceStepAtoms, item.PriceScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("decode instrument cache price step: %w", err)
	}
	quantityStep, err := trading.FormatDecimal(item.QuantityStepAtoms, item.QuantityScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("decode instrument cache quantity step: %w", err)
	}
	minNotional, err := trading.FormatDecimal(item.MinNotionalAtoms, item.QuoteScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("decode instrument cache minimum notional: %w", err)
	}
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base: base, Quote: quote,
		PriceScale: item.PriceScale, QuantityScale: item.QuantityScale,
		PriceStep: priceStep, QuantityStep: quantityStep, MinNotional: minNotional,
	})
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("decode instrument cache: %w", err)
	}
	if instrument.Symbol() != item.Symbol {
		return trading.Instrument{}, fmt.Errorf("decode instrument cache: symbol mismatch")
	}
	return instrument, nil
}
