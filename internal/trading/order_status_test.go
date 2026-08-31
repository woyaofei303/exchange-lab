package trading

import (
	"errors"
	"testing"
)

func TestParseOrderStatusNormalizesKnownValue(t *testing.T) {
	t.Parallel()

	status, err := ParseOrderStatus("PARTIALLY_FILLED")
	if err != nil {
		t.Fatalf("ParseOrderStatus() error = %v", err)
	}
	if got, want := status, OrderStatusPartiallyFilled; got != want {
		t.Fatalf("ParseOrderStatus() = %q, want %q", got, want)
	}
	if got, want := status.String(), "partially_filled"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseOrderStatusAcceptsEveryLifecycleValue(t *testing.T) {
	t.Parallel()

	statuses := []OrderStatus{
		OrderStatusPending,
		OrderStatusOpen,
		OrderStatusPartiallyFilled,
		OrderStatusFilled,
		OrderStatusCanceled,
		OrderStatusRejected,
	}
	for _, want := range statuses {
		want := want
		t.Run(want.String(), func(t *testing.T) {
			t.Parallel()

			got, err := ParseOrderStatus(want.String())
			if err != nil {
				t.Fatalf("ParseOrderStatus() error = %v", err)
			}
			if got != want {
				t.Fatalf("ParseOrderStatus() = %q, want %q", got, want)
			}
		})
	}
}

func TestParseOrderStatusRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	_, err := ParseOrderStatus("expired")
	if !errors.Is(err, ErrInvalidOrderStatus) {
		t.Fatalf("ParseOrderStatus() error = %v, want ErrInvalidOrderStatus", err)
	}
}
