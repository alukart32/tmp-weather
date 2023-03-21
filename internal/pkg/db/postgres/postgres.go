// Package postgres provides pgxpool.Pool.
package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pool *pgxpool.Pool
	once sync.Once
)

// Get returns an instance of pgxpool.Pool.
func Get() (*pgxpool.Pool, error) {
	var err error

	once.Do(func() {
		var cfg *pgxpool.Config

		cfg, err = prepareConf()
		if err != nil {
			return
		}

		pool, err = pgxpool.NewWithConfig(context.Background(), cfg)
		if err != nil {
			return
		}

		// Ping a new pool.
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err = pool.Ping(ctx)
		if err != nil {
			return
		}
	})

	return pool, err
}

// prepareConf prepares pgxpool.Config.
func prepareConf() (*pgxpool.Config, error) {
	cfg, err := newPoolConfig()
	if err != nil {
		return nil, err
	}

	if len(cfg.DSN) == 0 {
		return nil, errors.New("DSN is empty")
	}
	conf, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, err
	}
	conf.MaxConns = int32(cfg.MaxConns)

	return conf, nil
}
