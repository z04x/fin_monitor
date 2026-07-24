package db

import (
	"database/sql"
	"time"
)

func Open(databaseURL string) (*sql.DB, error) {
	pool, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(time.Hour)
	return pool, nil
}
