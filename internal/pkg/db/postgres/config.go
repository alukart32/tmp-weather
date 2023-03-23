package postgres

import (
	"time"

	"github.com/caarlos0/env/v6"
)

// poolConf is the representation of postgres pool settings.
type poolConf struct {
	DSN         string        `env:"POSTGRES_URI" envDefault:""`
	MaxConns    int32         `env:"POSTGRES_MAX_CONNS" envDefault:"5"`
	PingTimeout time.Duration `env:"POSTGRES_PING_TIMEOUT" envDefault:"300ms"`
}

// newPoolConfig returns a new config.
func newPoolConfig() (*poolConf, error) {
	opts := env.Options{RequiredIfNoDef: true}

	var cfg poolConf
	err := env.Parse(&cfg, opts)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
