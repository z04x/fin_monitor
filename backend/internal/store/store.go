// Package store is a cache-aside layer over the price and announcement
// providers, backed by Postgres. Past prices and past announcement dates never
// change, so once fetched they're served from the DB; a per-ticker TTL bounds
// how often we re-hit the providers to pick up new data. Any DB error degrades
// silently to a live provider call — the cache never breaks the feature.
package store

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"market-analyzer/backend/internal/adapters/edgar"
	"market-analyzer/backend/internal/prices"
)

type PriceSource interface {
	GetDailyPrices(ctx context.Context, ticker, from, to string) ([]prices.DailyPrice, error)
}

type AnnouncementSource interface {
	GetAnnouncements(ctx context.Context, ticker string) ([]edgar.Announcement, error)
}

type Store struct {
	db       *sql.DB
	prices   PriceSource
	edgar    AnnouncementSource
	priceTTL time.Duration
	annTTL   time.Duration
}

func New(db *sql.DB, p PriceSource, e AnnouncementSource) *Store {
	return &Store{db: db, prices: p, edgar: e, priceTTL: 12 * time.Hour, annTTL: 24 * time.Hour}
}

// GetDailyPrices serves cached prices for [from,to] when the ticker's cache is
// fresh; otherwise refetches the full range, upserts, and serves that.
func (s *Store) GetDailyPrices(ctx context.Context, ticker, from, to string) ([]prices.DailyPrice, error) {
	if s.db == nil {
		return s.prices.GetDailyPrices(ctx, ticker, from, to)
	}
	if s.isFresh(ctx, "prices", ticker, s.priceTTL) {
		if rows, err := s.readPrices(ctx, ticker, from, to); err == nil && len(rows) > 0 {
			return rows, nil
		}
	}

	fresh, err := s.prices.GetDailyPrices(ctx, ticker, from, to)
	if err != nil {
		// Provider failed — fall back to whatever we have cached, if anything.
		if rows, rerr := s.readPrices(ctx, ticker, from, to); rerr == nil && len(rows) > 0 {
			slog.Warn("store: price provider failed, serving stale cache", "ticker", ticker, "error", err)
			return rows, nil
		}
		return nil, err
	}
	if err := s.upsertPrices(ctx, ticker, fresh); err != nil {
		slog.Warn("store: upsert prices failed", "ticker", ticker, "error", err)
	} else {
		s.touch(ctx, "prices", ticker)
	}
	return fresh, nil
}

// GetAnnouncements serves cached announcement dates when fresh; else refetches.
func (s *Store) GetAnnouncements(ctx context.Context, ticker string) ([]edgar.Announcement, error) {
	if s.db == nil {
		return s.edgar.GetAnnouncements(ctx, ticker)
	}
	if s.isFresh(ctx, "edgar", ticker, s.annTTL) {
		if rows, err := s.readAnnouncements(ctx, ticker); err == nil && len(rows) > 0 {
			return rows, nil
		}
	}

	fresh, err := s.edgar.GetAnnouncements(ctx, ticker)
	if err != nil {
		if rows, rerr := s.readAnnouncements(ctx, ticker); rerr == nil && len(rows) > 0 {
			slog.Warn("store: edgar failed, serving stale cache", "ticker", ticker, "error", err)
			return rows, nil
		}
		return nil, err
	}
	if err := s.upsertAnnouncements(ctx, ticker, fresh); err != nil {
		slog.Warn("store: upsert announcements failed", "ticker", ticker, "error", err)
	} else {
		s.touch(ctx, "edgar", ticker)
	}
	return fresh, nil
}

// --- freshness ---------------------------------------------------------------

func (s *Store) isFresh(ctx context.Context, scope, ticker string, ttl time.Duration) bool {
	var refreshedAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT refreshed_at FROM cache_meta WHERE scope=$1 AND ticker=$2`, scope, ticker).Scan(&refreshedAt)
	if err != nil {
		return false
	}
	return time.Since(refreshedAt) < ttl
}

func (s *Store) touch(ctx context.Context, scope, ticker string) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cache_meta (scope, ticker, refreshed_at) VALUES ($1,$2,now())
		 ON CONFLICT (scope, ticker) DO UPDATE SET refreshed_at=now()`, scope, ticker)
	if err != nil {
		slog.Warn("store: touch meta failed", "scope", scope, "ticker", ticker, "error", err)
	}
}

// --- prices persistence ------------------------------------------------------

func (s *Store) readPrices(ctx context.Context, ticker, from, to string) ([]prices.DailyPrice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT to_char(date,'YYYY-MM-DD'), adj_open, adj_high, adj_low, adj_close, adj_volume
		 FROM price_daily WHERE ticker=$1 AND date BETWEEN $2 AND $3 ORDER BY date`, ticker, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []prices.DailyPrice
	for rows.Next() {
		var (
			date       string
			o, h, l, c sql.NullFloat64
			vol        sql.NullInt64
		)
		if err := rows.Scan(&date, &o, &h, &l, &c, &vol); err != nil {
			return nil, err
		}
		out = append(out, prices.DailyPrice{
			Date:      date,
			AdjOpen:   nf(o),
			AdjHigh:   nf(h),
			AdjLow:    nf(l),
			AdjClose:  nf(c),
			AdjVolume: ni(vol),
		})
	}
	return out, rows.Err()
}

func (s *Store) upsertPrices(ctx context.Context, ticker string, rows []prices.DailyPrice) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO price_daily (ticker, date, adj_open, adj_high, adj_low, adj_close, adj_volume)
		 VALUES ($1, $2::date, $3, $4, $5, $6, $7)
		 ON CONFLICT (ticker, date) DO UPDATE SET
		   adj_open=EXCLUDED.adj_open, adj_high=EXCLUDED.adj_high, adj_low=EXCLUDED.adj_low,
		   adj_close=EXCLUDED.adj_close, adj_volume=EXCLUDED.adj_volume`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range rows {
		if len(r.Date) < 10 {
			continue
		}
		if _, err := stmt.ExecContext(ctx, ticker, r.Date[:10],
			fptr(r.AdjOpen), fptr(r.AdjHigh), fptr(r.AdjLow), fptr(r.AdjClose), iptr(r.AdjVolume)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- announcements persistence ----------------------------------------------

func (s *Store) readAnnouncements(ctx context.Context, ticker string) ([]edgar.Announcement, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT report_date, COALESCE(session,'') FROM earnings_dates WHERE ticker=$1 ORDER BY report_date DESC`, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []edgar.Announcement
	for rows.Next() {
		var (
			date    time.Time
			session string
		)
		if err := rows.Scan(&date, &session); err != nil {
			return nil, err
		}
		out = append(out, edgar.Announcement{Date: date, Session: session})
	}
	return out, rows.Err()
}

func (s *Store) upsertAnnouncements(ctx context.Context, ticker string, anns []edgar.Announcement) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO earnings_dates (ticker, report_date, session) VALUES ($1, $2, $3)
		 ON CONFLICT (ticker, report_date) DO UPDATE SET session=EXCLUDED.session`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range anns {
		if _, err := stmt.ExecContext(ctx, ticker, a.Date, a.Session); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- null helpers ------------------------------------------------------------

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func ni(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func fptr(v *float64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func iptr(v *int64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}
