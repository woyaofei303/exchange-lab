package funding

import "testing"

func TestComputeRateClampsAndSettlementConserves(t *testing.T) {
	rate, err := ComputeRate(250, 10, 100)
	if err != nil || rate != 100 {
		t.Fatalf("rate=%d,%v", rate, err)
	}
	long, err := Payment(1_000, 5_000_000, 3, rate)
	if err != nil {
		t.Fatal(err)
	}
	short, err := Payment(-1_000, 5_000_000, 3, rate)
	if err != nil {
		t.Fatal(err)
	}
	if long != -50_000 || short != 50_000 || long+short != 0 {
		t.Fatalf("payments=%d,%d", long, short)
	}
}

func TestNegativeFundingReversesPayer(t *testing.T) {
	long, _ := Payment(1_000, 5_000_000, 3, -50)
	short, _ := Payment(-1_000, 5_000_000, 3, -50)
	if long != 25_000 || short != -25_000 {
		t.Fatalf("payments=%d,%d", long, short)
	}
}

func TestSystemOffsetBalancesResidualFundingExposure(t *testing.T) {
	offset, err := SystemOffset([]int64{-100, 40, 10})
	if err != nil || offset != 50 {
		t.Fatalf("SystemOffset() = %d, %v; want 50", offset, err)
	}
	zero, err := SystemOffset([]int64{0, 0})
	if err != nil || zero != 0 {
		t.Fatalf("SystemOffset(zero) = %d, %v", zero, err)
	}
}

func TestSystemOffsetRejectsOverflow(t *testing.T) {
	if _, err := SystemOffset([]int64{-1 << 63}); err == nil {
		t.Fatal("SystemOffset(min int64) error = nil")
	}
}
