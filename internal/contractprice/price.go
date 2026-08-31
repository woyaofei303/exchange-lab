// Package contractprice 计算可审计的指数价格和受限的标记价格。
package contractprice

import (
	"errors"
	"math/big"
	"sort"
	"time"
)

// 价格计算错误会区分输入格式错误和有效来源数量不足。
var (
	ErrInvalidPrice        = errors.New("invalid contract price input")
	ErrInsufficientSources = errors.New("insufficient fresh price sources")
)

// Source 表示一个有名称、有权重且以整数最小单位记录的价格观测值。
type Source struct {
	Name       string
	PriceAtoms int64
	Weight     int64
	UpdatedAt  time.Time
}

// IndexConfig 定义时效性、偏差和有效来源数量要求。
type IndexConfig struct {
	Now             time.Time
	MaxAge          time.Duration
	MaxDeviationBPS int64
	MinimumSources  int
}

// IndexResult 包含计算后的价格及来源级审计证据。
type IndexResult struct {
	PriceAtoms int64
	Included   []string
	Excluded   []string
	Fresh      bool
	AsOf       time.Time
}

// Snapshot 是提供给风险层和传输层的权威风险价格，特意不包含最新成交价。
type Snapshot struct {
	Symbol          string
	IndexAtoms      int64
	MarkAtoms       int64
	PriceScale      uint8
	PremiumBPS      int64
	IncludedSources []string
	ExcludedSources []string
	Fresh           bool
	AsOf            time.Time
}

// ComputeIndex 剔除过期和偏离中位数的来源，然后返回加权价格。
func ComputeIndex(sources []Source, config IndexConfig) (IndexResult, error) {
	if config.Now.IsZero() || config.MaxAge <= 0 || config.MaxDeviationBPS < 0 || config.MaxDeviationBPS > 5_000 || config.MinimumSources < 1 || config.MinimumSources > 100 || len(sources) > 100 {
		return IndexResult{}, ErrInvalidPrice
	}
	seen := make(map[string]struct{}, len(sources))
	fresh := make([]Source, 0, len(sources))
	excluded := make([]string, 0)
	for _, source := range sources {
		if source.Name == "" || len(source.Name) > 64 || source.PriceAtoms <= 0 || source.Weight <= 0 || source.Weight > 1_000_000 || source.UpdatedAt.IsZero() {
			return IndexResult{}, ErrInvalidPrice
		}
		if _, exists := seen[source.Name]; exists {
			return IndexResult{}, ErrInvalidPrice
		}
		seen[source.Name] = struct{}{}
		age := config.Now.Sub(source.UpdatedAt)
		if age < -time.Second || age > config.MaxAge {
			excluded = append(excluded, source.Name)
			continue
		}
		fresh = append(fresh, source)
	}
	if len(fresh) < config.MinimumSources {
		return IndexResult{}, ErrInsufficientSources
	}
	prices := make([]int64, len(fresh))
	for index, source := range fresh {
		prices[index] = source.PriceAtoms
	}
	sort.Slice(prices, func(i, j int) bool { return prices[i] < prices[j] })
	median := prices[len(prices)/2]
	if len(prices)%2 == 0 {
		median = roundedRatio(new(big.Int).Add(big.NewInt(prices[len(prices)/2-1]), big.NewInt(prices[len(prices)/2])), big.NewInt(2))
	}
	included := make([]Source, 0, len(fresh))
	for _, source := range fresh {
		deviation := new(big.Int).Sub(big.NewInt(source.PriceAtoms), big.NewInt(median))
		deviation.Abs(deviation).Mul(deviation, big.NewInt(10_000))
		limit := new(big.Int).Mul(big.NewInt(median), big.NewInt(config.MaxDeviationBPS))
		if deviation.Cmp(limit) > 0 {
			excluded = append(excluded, source.Name)
			continue
		}
		included = append(included, source)
	}
	if len(included) < config.MinimumSources {
		return IndexResult{}, ErrInsufficientSources
	}
	weighted := big.NewInt(0)
	totalWeight := big.NewInt(0)
	asOf := included[0].UpdatedAt.UTC()
	names := make([]string, 0, len(included))
	for _, source := range included {
		weighted.Add(weighted, new(big.Int).Mul(big.NewInt(source.PriceAtoms), big.NewInt(source.Weight)))
		totalWeight.Add(totalWeight, big.NewInt(source.Weight))
		if source.UpdatedAt.Before(asOf) {
			asOf = source.UpdatedAt.UTC()
		}
		names = append(names, source.Name)
	}
	price := roundedRatio(weighted, totalWeight)
	if price <= 0 {
		return IndexResult{}, ErrInvalidPrice
	}
	sort.Strings(names)
	sort.Strings(excluded)
	return IndexResult{PriceAtoms: price, Included: names, Excluded: excluded, Fresh: config.Now.Sub(asOf) <= config.MaxAge, AsOf: asOf}, nil
}

// ComputeMark 使用整数运算，将受限溢价应用于指数价格。
func ComputeMark(indexAtoms, premiumBPS, maximumPremiumBPS int64) (int64, int64, error) {
	if indexAtoms <= 0 || maximumPremiumBPS < 0 || maximumPremiumBPS > 5_000 {
		return 0, 0, ErrInvalidPrice
	}
	applied := premiumBPS
	if applied > maximumPremiumBPS {
		applied = maximumPremiumBPS
	}
	if applied < -maximumPremiumBPS {
		applied = -maximumPremiumBPS
	}
	multiplier := int64(10_000) + applied
	if multiplier <= 0 {
		return 0, 0, ErrInvalidPrice
	}
	mark := roundedRatio(new(big.Int).Mul(big.NewInt(indexAtoms), big.NewInt(multiplier)), big.NewInt(10_000))
	if mark <= 0 {
		return 0, 0, ErrInvalidPrice
	}
	return mark, applied, nil
}

func roundedRatio(numerator, denominator *big.Int) int64 {
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0
	}
	return quotient.Int64()
}
