package main

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestedFundingTimeUsesExplicitRFC3339OrCurrentTime(t *testing.T) {
	now := time.Date(2026, 8, 10, 7, 30, 0, 0, time.UTC)

	request := httptest.NewRequest("POST", "/run", nil)
	got, err := requestedFundingTime(request, now)
	if err != nil || !got.Equal(now) {
		t.Fatalf("default funding time=%s err=%v", got, err)
	}

	request = httptest.NewRequest("POST", "/run?at=2026-08-09T16:00:00Z", nil)
	got, err = requestedFundingTime(request, now)
	want := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	if err != nil || !got.Equal(want) {
		t.Fatalf("explicit funding time=%s err=%v, want %s", got, err, want)
	}
}

func TestRequestedFundingTimeRejectsInvalidValue(t *testing.T) {
	request := httptest.NewRequest("POST", "/run?at=tomorrow", nil)
	if _, err := requestedFundingTime(request, time.Now()); err == nil {
		t.Fatal("invalid funding time accepted")
	}
}
