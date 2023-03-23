// Package migrate provides postgres db migration.
//
// Migration engine is a golang-migrate project. Sql files are in /migrations directory.
// If the filepath is empty, the nearest migrations directory will be selected.
package migrate

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const (
	_defaultAttempts = 5
	_defaultTimeout  = time.Second
	_driverName      = "postgres"
	_migrations      = "migrations"
)

// Up starts db migrations. If the filepath is empty, the nearest migrations folder will be selected.
func Up(uri string, filepath string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("migrate up: %w", err)
		}
	}()
	if len(uri) == 0 {
		return fmt.Errorf("empty uri")
	}
	if len(filepath) == 0 {
		filepath = _migrations
	}

	errCh := make(chan error)
	go func() {
		var (
			attempts = _defaultAttempts
			path     = filepath
			err      error
			m        *migrate.Migrate
		)

		u, err := url.Parse(uri)
		if err != nil {
			errCh <- fmt.Errorf("invalid uri: %q", uri)
			return
		}
		queryValues := u.Query()
		if !queryValues.Has("sslmode") {
			queryValues.Set("sslmode", "disable")
		}
		u.RawQuery = queryValues.Encode()

		for attempts > 0 {
			m, err = migrate.New("file://"+path, u.String())
			if err == nil {
				break
			}

			log.Printf("migrate: trying to connect, attempts left: %d", attempts)
			time.Sleep(_defaultTimeout)
			attempts--
		}
		if m == nil {
			errCh <- fmt.Errorf("unable to create migration")
			return
		}

		err = m.Up()
		defer m.Close()
		if err != nil && errors.Is(err, migrate.ErrNoChange) {
			err = nil
		}
		errCh <- err
	}()

	return <-errCh
}
