// go:build integration

package storage

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/alukart32/tmp-weather/internal/pkg/db/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/suite"
)

func TestWeatherForecastSuite(t *testing.T) {
	suite.Run(t, new(WeatherForecastTestSuite))
}

type WeatherForecastTestSuite struct {
	suite.Suite
	pool           *pgxpool.Pool
	purgeContainer func()
}

func (suite *WeatherForecastTestSuite) SetupSuite() {
	var (
		dbName        = os.Getenv("POSTGRES_DB")
		dbPassword    = os.Getenv("POSTGRES_PASSWORD")
		dbUser        = os.Getenv("POSTGRES_USER")
		dbPort        = os.Getenv("POSTGRES_PORT")
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

	// Purge existed test container.
	container, ok := dockerPool.ContainerByName(containerName)
	if ok {
		if err := dockerPool.Purge(container); err != nil {
			log.Fatalf("Could not purge container: %s", err)
		}
	}
	// Run a new test container.
	container, err = dockerPool.RunWithOptions(&opts)
	if err != nil {
		log.Fatalf("Could not start container: %s", err)
	}

	// Wait until the container is ready.
	if err := dockerPool.Retry(func() error {
		// Test pool connectivity.
		ctx := context.Background()
		suite.pool, err = pgxpool.New(ctx, fmt.Sprintf("postgres://%v:%v@localhost:%v/%v",
			dbUser, dbPassword, dbPort, dbName))
		if err != nil {
			return err
		}
		return suite.pool.Ping(ctx)
	}); err != nil {
		log.Fatalf("Could not connect to database: %s", err)
	}

	migrateDb(suite.T(), suite.pool.Config().ConnString())

	suite.purgeContainer = func() {
		if err := dockerPool.Purge(container); err != nil {
			log.Fatalf("Could not purge container: %s", err)
		}
	}
}

func (suite *WeatherForecastTestSuite) TearDownSuite() {
	// Purge the test container.
	suite.purgeContainer()
	// Close the postgres pool.
	suite.pool.Close()
}

// this function executes after each test case
func (suite *WeatherForecastTestSuite) TearDownTest() {
	fmt.Printf("-- TearDown %v\n", suite.T().Name())

	_, err := suite.pool.Exec(context.Background(), "TRUNCATE TABLE forecasts CASCADE")
	if err != nil {
		suite.Fail(err.Error())
	}
}

func (suite *WeatherForecastTestSuite) Test_Insert() {
	repo, err := NewWeatherForecastRepo(suite.pool)
	if err != nil {
		suite.Fail("failed to create WeatherForecastRepo: %v", err)
	}

	upsertCtx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
	defer cancel()

	data := WeatherForecast{
		MsgID:  1,
		City:   "Test",
		Desc:   "testable",
		Temp:   1.0,
		Hum:    2,
		Wind:   3.0,
		MadeAt: time.Now(),
	}

	err = repo.Insert(upsertCtx, data)
	suite.Require().NoError(err)
}

func (suite *WeatherForecastTestSuite) Test_Stat() {
	firstCreatedAt := time.Now()

	forecasts := []WeatherForecast{
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
	}
	wantStat := WeatherForecastStat{
		total:         4,
		firstRecordAt: firstCreatedAt,
		TopRecords: struct {
			city    string
			maxTemp float64
		}{
			city:    "A",
			maxTemp: 30.0,
		},
	}

	repo, err := NewWeatherForecastRepo(suite.pool)
	if err != nil {
		suite.Fail("failed to create repo: %v", err)
	}

	insertCtx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
	defer cancel()

	for _, v := range forecasts {
		suite.Require().NoError(repo.Insert(insertCtx, v))
	}

	statCtx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
	defer cancel()

	stat, err := repo.Stat(statCtx)
	suite.Require().NoError(err)

	suite.Equal(wantStat.total, stat.total,
		"expected total: %d, was %d", wantStat.total, stat.total)
	suite.Equal(wantStat.firstRecordAt.Format(time.RFC3339), stat.firstRecordAt.Format(time.RFC3339),
		"expected firstRecordAt: %v, was %v", wantStat.firstRecordAt, stat.firstRecordAt)
	suite.Equal(wantStat.TopRecords.city, stat.TopRecords.city,
		"expected maxTempCity: %v, was %v", wantStat.TopRecords.city, stat.TopRecords.city)
	suite.Equal(wantStat.TopRecords.maxTemp, stat.TopRecords.maxTemp,
		"expected maxTemp: %v, was %v", wantStat.TopRecords.maxTemp, stat.TopRecords.maxTemp)
}

// migrateDb migrates the sql schema of the database.
func migrateDb[TB testing.TB](tb TB, uri string) {
	if err := migrate.Up(uri, ""); err != nil {
		tb.Fatalf("Unable to migrate: %v", err)
	}
}
