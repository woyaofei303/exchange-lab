package contractprice

import (
	"errors"
	"testing"
	"time"
)

func TestIndexFiltersStaleAndOutlierSources(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	result, err := ComputeIndex([]Source{
		{Name: "a", PriceAtoms: 5_000_000, Weight: 1, UpdatedAt: now.Add(-time.Second)},
		{Name: "b", PriceAtoms: 5_000_010, Weight: 1, UpdatedAt: now.Add(-2 * time.Second)},
		{Name: "c", PriceAtoms: 4_999_990, Weight: 1, UpdatedAt: now.Add(-3 * time.Second)},
		{Name: "outlier", PriceAtoms: 9_000_000, Weight: 100, UpdatedAt: now},
		{Name: "stale", PriceAtoms: 5_000_000, Weight: 100, UpdatedAt: now.Add(-time.Minute)},
	}, IndexConfig{Now: now, MaxAge: 10 * time.Second, MaxDeviationBPS: 100, MinimumSources: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.PriceAtoms != 5_000_000 || len(result.Included) != 3 || len(result.Excluded) != 2 || !result.Fresh {
		t.Fatalf("index = %#v", result)
	}
}

func TestIndexFailsClosedWithoutEnoughFreshSources(t *testing.T) {
	now := time.Now().UTC()
	_, err := ComputeIndex([]Source{{Name: "a", PriceAtoms: 100, Weight: 1, UpdatedAt: now}}, IndexConfig{Now: now, MaxAge: time.Second, MaxDeviationBPS: 100, MinimumSources: 2})
	if !errors.Is(err, ErrInsufficientSources) {
		t.Fatalf("ComputeIndex() error = %v", err)
	}
}

func TestMarkPriceClampsPremiumAndUsesIntegerRounding(t *testing.T) {
	mark, applied, err := ComputeMark(5_000_001, 250, 100)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 100 || mark != 5_050_001 {
		t.Fatalf("positive mark/applied = %d/%d", mark, applied)
	}
	mark, applied, err = ComputeMark(5_000_001, -250, 100)
	if err != nil || applied != -100 || mark != 4_950_001 {
		t.Fatalf("negative mark/applied = %d/%d, %v", mark, applied, err)
	}
}
