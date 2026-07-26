package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"market-analyzer/backend/internal/adapters/edgar"
	"market-analyzer/backend/internal/prices"
)

func f(v float64) *float64 { return &v }

type fakePrices struct {
	calls int
	rows  []prices.DailyPrice
	err   error
}

func (f *fakePrices) GetDailyPrices(_ context.Context, _, _, _ string) ([]prices.DailyPrice, error) {
	f.calls++
	return f.rows, f.err
}

type fakeEdgar struct {
	calls int
	rows  []edgar.Announcement
	err   error
}

func (f *fakeEdgar) GetAnnouncements(_ context.Context, _ string) ([]edgar.Announcement, error) {
	f.calls++
	return f.rows, f.err
}

// With a nil DB the store must transparently delegate to the live sources —
// this is the "no DATABASE_URL" path that keeps the feature working uncached.
func TestNilDBDelegates(t *testing.T) {
	fp := &fakePrices{rows: []prices.DailyPrice{{Date: "2026-06-22"}}}
	fe := &fakeEdgar{rows: []edgar.Announcement{{Session: "amc"}}}
	s := New(nil, fp, fe)

	pr, err := s.GetDailyPrices(context.Background(), "MU", "2026-01-01", "2026-06-30")
	if err != nil || len(pr) != 1 || fp.calls != 1 {
		t.Fatalf("prices delegate failed: rows=%d calls=%d err=%v", len(pr), fp.calls, err)
	}
	an, err := s.GetAnnouncements(context.Background(), "MU")
	if err != nil || len(an) != 1 || fe.calls != 1 {
		t.Fatalf("announcements delegate failed: rows=%d calls=%d err=%v", len(an), fe.calls, err)
	}
}

func TestNilDBPropagatesError(t *testing.T) {
	fp := &fakePrices{err: errors.New("boom")}
	s := New(nil, fp, &fakeEdgar{})
	if _, err := s.GetDailyPrices(context.Background(), "MU", "a", "b"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestNullHelpers(t *testing.T) {
	if nf(sql.NullFloat64{Valid: false}) != nil {
		t.Fatal("invalid float should map to nil")
	}
	if v := nf(sql.NullFloat64{Valid: true, Float64: 1.5}); v == nil || *v != 1.5 {
		t.Fatal("valid float mismap")
	}
	if fptr((*float64)(nil)) != nil {
		t.Fatal("nil fptr should be nil interface value")
	}
	if fptr(f(2.0)) != 2.0 {
		t.Fatal("fptr deref wrong")
	}
}
