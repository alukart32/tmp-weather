// Package storage provides predictive data storage functionality.
//
// PostgreSQL is the default repository driver.
package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

var ErrNoData = errors.New("no data")

// WeatherForecastRepo defines the forecast repository.
type WeatherForecastRepo struct {
	pool *pgxpool.Pool
	mtx  sync.Mutex
}

// NewWeatherForecastRepo returns a new ForecastRepo.
func NewWeatherForecastRepo(pool *pgxpool.Pool) (*WeatherForecastRepo, error) {
	if pool == nil {
		return nil, fmt.Errorf("postgres pool is nil")
	}

	return &WeatherForecastRepo{
		pool: pool,
	}, nil
}

// WeatherForecast represents the weather forecast that is stored in the repository.
type WeatherForecast struct {
	MadeAt time.Time
	City   string
	Desc   string
	Temp   float64
	Hum    int64
	Wind   float64
	MsgID  int
}

const upsertWeatherForecast = `
INSERT INTO
	forecasts(msg_id, city, description, temp, hum, wind, made_at)
VALUES
	($1, $2, $3, $4, $5, $6, $7)
`

// Upsert a new weather forecast data.
func (r *WeatherForecastRepo) Upsert(ctx context.Context, f WeatherForecast) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:       pgx.ReadCommitted,
		AccessMode:     pgx.ReadWrite,
		DeferrableMode: pgx.NotDeferrable,
	})
	if err != nil {
		return fmt.Errorf("unable to start transaction: %v", err.Error())
	}

	defer func() {
		err = r.finishTransaction(ctx, tx, err)
	}()

	_, err = tx.Exec(ctx, upsertWeatherForecast,
		f.MsgID,
		f.City,
		f.Desc,
		f.Temp,
		f.Hum,
		f.Wind,
		f.MadeAt,
	)

	return err
}

// WeatherForecastStat represents the weather forecast statistics.
type WeatherForecastStat struct {
	TopRecords struct {
		maxTempCity string
		maxTemp     float64
		maxHumCity  string
		maxHum      int64
		maxWindCity string
		maxWind     float64
	}
	firstRecordAt time.Time
	total         int
}

// ToMsg converts the ForecastStat data to the msg format of the telegram bot.
func (f WeatherForecastStat) ToMsg() string {
	var sb strings.Builder

	fmt.Fprint(&sb, "Total\n")
	fmt.Fprintf(&sb, "\t\trecords: %d\n", f.total)
	fmt.Fprintf(&sb, "\t\t1st at: %v\n\n", f.firstRecordAt.Format(time.RFC822))
	fmt.Fprintf(&sb, "Top forecast\n")
	fmt.Fprintf(&sb, "\t\t- max temp\n")
	fmt.Fprintf(&sb, "\t\t\t\t\t\tcity: %v\n", f.TopRecords.maxTempCity)
	fmt.Fprintf(&sb, "\t\t\t\t\t\ttemp: %.2f C\n\n", f.TopRecords.maxTemp)
	fmt.Fprintf(&sb, "\t\t- max humidity\n")
	fmt.Fprintf(&sb, "\t\t\t\t\t\tcity: %v\n", f.TopRecords.maxHumCity)
	fmt.Fprintf(&sb, "\t\t\t\t\t\thum: %d %%\n\n", f.TopRecords.maxHum)
	fmt.Fprintf(&sb, "\t\t- max wind\n")
	fmt.Fprintf(&sb, "\t\t\t\t\t\tcity: %v\n", f.TopRecords.maxWindCity)
	fmt.Fprintf(&sb, "\t\t\t\t\t\twind: %.2f m/s\n", f.TopRecords.maxWind)

	return sb.String()
}

// MarshalZerologObject adds ForecastStat to the logger as an object.
func (f WeatherForecastStat) MarshalZerologObject(e *zerolog.Event) {
	e.
		Int("total", f.total).
		Time("firstRecordAt", f.firstRecordAt).
		Dict("topRecords", zerolog.Dict().
			Str("maxTempCity", f.TopRecords.maxTempCity).
			Float64("maxTemp", f.TopRecords.maxTemp).
			Str("maxHumCity", f.TopRecords.maxHumCity).
			Int64("maxHum", f.TopRecords.maxHum).
			Str("maxWindCity", f.TopRecords.maxWindCity).
			Float64("maxWind", f.TopRecords.maxWind))
}

const getWeatherForecastStat = `
SELECT
  first_record.made_at AS first_record,
  total_records.count AS total,
  top_temp.city AS top_temp_city,
  top_temp.value AS top_temp,
  top_hum.city AS top_hum_city,
  top_hum.value AS top_hum,
  top_wind.city AS top_wind_city,
  top_wind.value AS top_wind
FROM
  (
    SELECT
      made_at
    FROM
      forecasts
    ORDER BY
      made_at ASC
    LIMIT
      1
  ) AS first_record,
  (
    SELECT
      COUNT(*) AS COUNT
    FROM
      forecasts
    LIMIT
      1
  ) AS total_records,
  (
    SELECT
      city,
      MAX(DISTINCT TEMP):: numeric(10, 2) AS value
    FROM
      forecasts
    GROUP BY
      city
    ORDER BY
      value DESC
    LIMIT
      1
  ) AS top_temp,
  (
    SELECT
      city,
      MAX(DISTINCT hum):: numeric(10, 2) AS value
    FROM
      forecasts
    GROUP BY
      city
    ORDER BY
      value DESC
    LIMIT
      1
  ) AS top_hum,
  (
    SELECT
      city,
      MAX(DISTINCT wind):: numeric(10, 2) AS value
    FROM
      forecasts
    GROUP BY
      city
    ORDER BY
      value DESC
    LIMIT
      1
  ) AS top_wind;
`

// Stat returns the weather forecast statistics.
func (r *WeatherForecastRepo) Stat(ctx context.Context) (WeatherForecastStat, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:       pgx.ReadCommitted,
		AccessMode:     pgx.ReadWrite,
		DeferrableMode: pgx.NotDeferrable,
	})
	if err != nil {
		return WeatherForecastStat{}, fmt.Errorf("unable to start transaction: %v", err.Error())
	}

	defer func() {
		err = r.finishTransaction(ctx, tx, err)
	}()

	r.mtx.Lock()
	defer r.mtx.Unlock()
	// Get top city stat.
	var stat WeatherForecastStat

	// Get main forecast stat.
	row := tx.QueryRow(ctx, getWeatherForecastStat)
	err = row.Scan(
		&stat.firstRecordAt,
		&stat.total,
		&stat.TopRecords.maxTempCity,
		&stat.TopRecords.maxTemp,
		&stat.TopRecords.maxHumCity,
		&stat.TopRecords.maxHum,
		&stat.TopRecords.maxWindCity,
		&stat.TopRecords.maxWind,
	)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		err = ErrNoData
	}

	return stat, err
}

// finishTransaction rollbacks transaction if error is provided.
// If err is nil transaction is committed.
func (r *WeatherForecastRepo) finishTransaction(ctx context.Context, tx pgx.Tx, err error) error {
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}

		return err
	} else {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return fmt.Errorf("failed to commit tx: %v", err)
		}

		return nil
	}
}
