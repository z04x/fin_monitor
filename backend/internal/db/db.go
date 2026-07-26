// Package db opens the Postgres connection used for caching and ensures the
// tables the cache relies on exist. The whole DB layer is optional: with no
// DATABASE_URL the app runs against live providers (see internal/store).
package db

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/lib/pq" // registers the "postgres" driver used by sql.Open
)

func Open(databaseURL string) (*sql.DB, error) {
	pool, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// EnsureSchema creates the cache tables if they don't exist. Idempotent; kept
// in sync with migrations/002 (price_daily) plus cache-only tables. Using
// IF NOT EXISTS keeps startup simple without a migration runner.
func EnsureSchema(ctx context.Context, pool *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS price_daily (
    ticker       TEXT NOT NULL,
    date         DATE NOT NULL,
    adj_open     NUMERIC,
    adj_high     NUMERIC,
    adj_low      NUMERIC,
    adj_close    NUMERIC,
    adj_volume   BIGINT,
    UNIQUE (ticker, date)
);
CREATE INDEX IF NOT EXISTS idx_price_ticker_date ON price_daily (ticker, date);

CREATE TABLE IF NOT EXISTS earnings_dates (
    ticker       TEXT NOT NULL,
    report_date  DATE NOT NULL,
    session      TEXT,
    UNIQUE (ticker, report_date)
);

CREATE TABLE IF NOT EXISTS cache_meta (
    scope        TEXT NOT NULL,
    ticker       TEXT NOT NULL,
    refreshed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (scope, ticker)
);`
	_, err := pool.ExecContext(ctx, ddl)
	return err
}
