//go:build integration

package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/alukart32/tmp-weather/internal/pkg/db/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pool *pgxpool.Pool

var (
	dbName     = os.Getenv("POSTGRES_DB")
	dbPassword = os.Getenv("POSTGRES_PASSWORD")
	dbUser     = os.Getenv("POSTGRES_USER")
	dbPort     = os.Getenv("POSTGRES_PORT")
)

func TestMain(m *testing.M) {
	var (
		containerName = os.Getenv("POSTGRES_CONTAINER_NAME")
		image         = os.Getenv("POSTGRES_IMAGE")
		imageTag      = os.Getenv("POSTGRES_IMAGE_TAG")
	)

	// Prepare docker pool.
	dockerPool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatal(err)
	}
	dockerPool.MaxWait = 60 * time.Second

	err = dockerPool.Client.Ping()
	if err != nil {
		log.Fatalf("Could not connect to Docker: %s", err)
	}

	// Run the Docker container.
	opts := dockertest.RunOptions{
		Repository: image,
		Tag:        imageTag,
		Name:       containerName,
		Env: []string{
			"POSTGRES_USER=" + dbUser,
			"POSTGRES_PASSWORD=" + dbPassword,
			"POSTGRES_DB=" + dbName,
		},
		ExposedPorts: []string{"5432"},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"5432": {
				{HostIP: "0.0.0.0", HostPort: dbPort},
			},
		},
	}

	if c, ok := dockerPool.ContainerByName(containerName); ok {
		if err := dockerPool.Purge(c); err != nil {
			log.Fatalf("Could not purge container: %s", err)
		}
	}

	container, err := dockerPool.RunWithOptions(&opts)
	if err != nil {
		log.Fatalf("Could not start container: %s", err)
	}

	// Wait until the container is ready.
	if err := dockerPool.Retry(func() error {
		ctx := context.Background()
		pool, err = pgxpool.New(ctx, testDatabaseURI())
		if err != nil {
			return err
		}
		return pool.Ping(ctx)
	}); err != nil {
		log.Fatalf("Could not connect to database: %s", err)
	}

	// Run tests.
	code := m.Run()

	// Purge the test container.
	if err := dockerPool.Purge(container); err != nil {
		log.Fatalf("Could not purge container: %s", err)
	}
	// Close the postgres pool.
	pool.Close()

	os.Exit(code)
}

func TestForecastRepo_Upsert(t *testing.T) {
	tests := []struct {
		name    string
		data    WeatherForecast
		wantErr bool
	}{
		{
			name: "Valid weather forecast",
			data: WeatherForecast{
				MsgID:  1,
				City:   "Test",
				Desc:   "testable",
				Temp:   1.0,
				Hum:    2,
				Wind:   3.0,
				MadeAt: time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPostgresTest(context.TODO(), t, func(t *testing.T, pool *pgxpool.Pool) {
				t.Parallel()
				repo, err := NewWeatherForecastRepo(pool)
				if err != nil {
					t.Fatalf("unable to create repo: %v", err)
				}

				upsertCtx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
				defer cancel()

				err = repo.Upsert(upsertCtx, tt.data)
				if tt.wantErr {
					require.Error(t, err)
				}
			})
		})
	}
}

func TestForecastRepo_Stat(t *testing.T) {
	firstCreatedAt := time.Now()
	tests := []struct {
		name      string
		forecasts []WeatherForecast
		wantStat  WeatherForecastStat
		wantErr   bool
	}{
		{
			name: "Valid stat",
			forecasts: []WeatherForecast{
				{
					MsgID:  1,
					City:   "A",
					Desc:   "max temp",
					Temp:   30.0,
					Hum:    2,
					Wind:   3.0,
					MadeAt: firstCreatedAt,
				},
				{
					MsgID:  2,
					City:   "B",
					Desc:   "max hum",
					Temp:   1.0,
					Hum:    90,
					Wind:   3.0,
					MadeAt: time.Now(),
				},
				{
					MsgID:  3,
					City:   "C",
					Desc:   "max wind",
					Temp:   1.0,
					Hum:    2,
					Wind:   12.0,
					MadeAt: time.Now(),
				},
				{
					MsgID:  4,
					City:   "D",
					Desc:   "casual weather",
					Temp:   1.0,
					Hum:    2,
					Wind:   3.0,
					MadeAt: time.Now(),
				},
			},
			wantStat: WeatherForecastStat{
				total:         4,
				firstRecordAt: firstCreatedAt,
				TopRecords: struct {
					maxTempCity string
					maxTemp     float64
					maxHumCity  string
					maxHum      int64
					maxWindCity string
					maxWind     float64
				}{
					maxTempCity: "A",
					maxTemp:     30.0,
					maxHumCity:  "B",
					maxHum:      90,
					maxWindCity: "C",
					maxWind:     12.0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPostgresTest(context.TODO(), t, func(t *testing.T, pool *pgxpool.Pool) {
				t.Parallel()
				repo, err := NewWeatherForecastRepo(pool)
				if err != nil {
					t.Fatalf("unable to create repo: %v", err)
				}

				upsertCtx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
				defer cancel()

				for _, v := range tt.forecasts {
					require.NoError(t, repo.Upsert(upsertCtx, v))
				}

				statCtx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
				defer cancel()

				stat, err := repo.Stat(statCtx)
				if tt.wantErr {
					require.Error(t, err)
					return
				}

				assert.Equal(t, tt.wantStat.total, stat.total,
					"expected total: %d, was %d", tt.wantStat.total, stat.total)
				assert.Equal(t, 0, tt.wantStat.firstRecordAt.Compare(stat.firstRecordAt),
					"expected firstRecordAt: %v, was %v", tt.wantStat.firstRecordAt, stat.firstRecordAt)
				assert.Equal(t, tt.wantStat.TopRecords.maxTempCity, stat.TopRecords.maxTempCity,
					"expected maxTempCity: %v, was %v", tt.wantStat.TopRecords.maxTempCity, stat.TopRecords.maxTempCity)
				assert.Equal(t, tt.wantStat.TopRecords.maxTemp, stat.TopRecords.maxTemp,
					"expected maxTemp: %v, was %v", tt.wantStat.TopRecords.maxTemp, stat.TopRecords.maxTemp)
				assert.Equal(t, tt.wantStat.TopRecords.maxHumCity, stat.TopRecords.maxHumCity,
					"expected maxHumCity: %v, was %v", tt.wantStat.TopRecords.maxHumCity, stat.TopRecords.maxHumCity)
				assert.Equal(t, tt.wantStat.TopRecords.maxHum, stat.TopRecords.maxHum,
					"expected maxHum: %v, was %v", tt.wantStat.TopRecords.maxHum, stat.TopRecords.maxHum)
				assert.Equal(t, tt.wantStat.TopRecords.maxWindCity, stat.TopRecords.maxWindCity,
					"expected maxWindCity: %v, was %v", tt.wantStat.TopRecords.maxWindCity, stat.TopRecords.maxWindCity)
				assert.Equal(t, tt.wantStat.TopRecords.maxWind, stat.TopRecords.maxWind,
					"expected maxWind: %v, was %v", tt.wantStat.TopRecords.maxWind, stat.TopRecords.maxWind)
			})
		})
	}
}

func withPostgresTest[TB testing.TB](ctx context.Context, tb TB, test func(t TB, pool *pgxpool.Pool)) {
	sanitize := func(schema string) string {
		return pgx.Identifier{schema}.Sanitize()
	}
	createSchema := func(ctx context.Context, c *pgxpool.Conn, name string) error {
		_, err := c.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+sanitize(name))
		return err
	}
	dropSchema := func(ctx context.Context, c *pgxpool.Conn, name string) error {
		_, err := c.Exec(ctx, `DROP SCHEMA IF EXISTS `+sanitize(name)+` CASCADE`)
		return err
	}

	// Create a unique database name so that our parallel tests don't clash.
	var id [8]byte
	rand.Read(id[:])
	schemaName := tb.Name() + "_" + hex.EncodeToString(id[:])

	// Create the main db connection.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		tb.Fatalf("unable to create tx: %v", err)
	}
	defer func() {
		conn.Release()
	}()

	// Create test database.
	if err := createSchema(ctx, conn, schemaName); err != nil {
		tb.Fatalf("unable to create schema: %v", err)
	}
	defer func() {
		if err := dropSchema(ctx, conn, schemaName); err != nil {
			tb.Fatalf("unable to drop schema: %v", err)
		}
	}()

	// Connect to the test database.
	connURI, err := uriWithSchema(testDatabaseURI(), sanitize(schemaName))
	if err != nil {
		tb.Fatal(err)
	}

	migrateDb(tb, connURI)

	// Create a new connection to the database.
	db, err := pgxpool.New(ctx, connURI)
	if err != nil {
		tb.Fatalf("Unable to create the postgres pool to %v: %v", schemaName, err)
	}
	if err = db.Ping(ctx); err != nil {
		tb.Fatalf("Unable to ping: %v", err)
	}
	defer db.Close()

	// Run test code.
	test(tb, db)
}

// migrateDb migrates the sql schema of the database.
func migrateDb[TB testing.TB](tb TB, uri string) {
	if err := migrate.Up(uri, ""); err != nil {
		tb.Fatalf("Unable to migrate: %v", err)
	}
}

// testDatabaseURI returns the test database connection URI.
func testDatabaseURI() string {
	return fmt.Sprintf("postgres://%v:%v@localhost:%v/%v",
		dbUser, dbPassword, dbPort, dbName)
}

// uriWithSchema adds a schema to the database connection URI.
func uriWithSchema(uri, schema string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("invalid connstr: %q", uri)
	}

	values := u.Query()
	values.Set("search_path", schema)
	u.RawQuery = values.Encode()

	return u.String(), nil
}
