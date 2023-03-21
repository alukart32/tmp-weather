// Package weather provides a weather forecaster.
//
// The weather forecaster executes a request, which uses the name of the city
// to get the current weather: https://openweathermap.org/current#name.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alukart32/tmp-weather/internal/pkg/zerologx"
	"github.com/rs/zerolog"
)

// CityForecaster defines a weather forecaster by city name.
type CityForecaster struct {
	msgs chan string         // incoming city names
	res  chan forecastResult // forecast data
}

// NewCityForecaster returns a new CityForecaster.
func NewCityForecaster(ctx context.Context) CityForecaster {
	forecaster := CityForecaster{
		msgs: make(chan string),
	}

	forecaster.res = worker(ctx, forecaster.msgs)
	return forecaster
}

// Forecast accepts the city name and returns the weather forecast.
func (f *CityForecaster) Forecast(ctx context.Context, cityName string) (Forecast, error) {
	select {
	case <-ctx.Done():
	case f.msgs <- cityName:
	}

	res := <-f.res
	return res.Forecast, res.Err
}

// openweathermap request errors.
var (
	ErrCityNotFound  = errors.New("city not found")
	ErrExternal      = errors.New("external error")
	ErrCorruptedCall = errors.New("corrupted call")
)

// forecastResult represents the respond forecast.
type forecastResult struct {
	Forecast
	Err error
}

// worker sends forecast requests to openweathermap and returns a response.
func worker(ctx context.Context, in chan string) chan forecastResult {
	out := make(chan forecastResult)

	go func() {
		defer close(out)

		logger := zerologx.Get()

		apiToken := os.Getenv("OPENWEATHERMAP_API_TOKEN")
		if len(apiToken) == 0 {
			logger.Error().Msg("invalid openweathermap api key")
			return
		}
		api := "https://api.openweathermap.org/data/2.5/weather"

		client := &http.Client{
			Timeout: time.Second * 1,
			Transport: &http.Transport{
				MaxIdleConns: 15,
			},
		}

		for {
			select {
			case <-ctx.Done():
			case cityName, ok := <-in:
				if !ok {
					return
				}

				url := fmt.Sprintf("%s?units=metric&q=%s&appid=%s", api, cityName, apiToken)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				if err != nil {
					out <- forecastResult{Forecast: Forecast{}, Err: err}
				}

				logger.Info().
					Str("op", "get forecast").
					Str("city", cityName).Send()
				resp, err := client.Do(req)
				logger.Info().
					Str("op", "forecast respond").
					Str("city", cityName).
					Int("respCode", resp.StatusCode).Send()
				if err != nil {
					out <- forecastResult{
						Forecast: Forecast{},
						Err:      ErrCorruptedCall,
					}
					break
				}
				defer resp.Body.Close()

				switch resp.StatusCode {
				case http.StatusNotFound:
					err = ErrCityNotFound
				case http.StatusBadRequest, http.StatusBadGateway:
					err = ErrExternal
				default:
				}
				if err != nil {
					out <- forecastResult{
						Forecast: Forecast{},
						Err:      err,
					}
					break
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					out <- forecastResult{
						Forecast: Forecast{},
						Err:      fmt.Errorf("read response body: %v", err),
					}
					break
				}

				var forecast Forecast
				if err = json.Unmarshal(body, &forecast); err != nil {
					out <- forecastResult{
						Forecast: Forecast{},
						Err:      fmt.Errorf("unmarshal response body: %v", err),
					}
				}
				forecast.MadeAt = time.Now()

				out <- forecastResult{
					Forecast: forecast,
				}
			}
		}
	}()

	return out
}

// Forecast represents the openweathermap weather forecast: https://openweathermap.org/current#current_JSON.
type Forecast struct {
	MadeAt time.Time
	Main   struct {
		Temp      float64
		FeelsLike float64 `json:"feels_like"`
		Humidity  int64
	}
	Weather []struct {
		Description string
	}
	Wind struct {
		Speed float64
	}
	Err error
}

// ToMsg converts the Forecast to the msg format of the telegram bot.
func (f Forecast) ToMsg() string {
	var sb strings.Builder

	// https://openweathermap.org/weather-data
	fmt.Fprintf(&sb, "%v\n\n", f.Weather[0].Description)
	fmt.Fprintf(&sb, "temp: %.2f C\n", f.Main.Temp)
	fmt.Fprintf(&sb, "feels like: %.2f C\n\n", f.Main.FeelsLike)
	fmt.Fprintf(&sb, "hum: %d %%\n", f.Main.Humidity)
	fmt.Fprintf(&sb, "wind: %.2f m/s\n", f.Wind.Speed)

	return sb.String()
}

// MarshalZerologObject adds Forecast to the logger as an object.
func (f Forecast) MarshalZerologObject(e *zerolog.Event) {
	e.
		Time("madeAt", f.MadeAt).
		Str("description", f.Weather[0].Description).
		Float64("temp", f.Main.Temp).
		Float64("feelsLike", f.Main.FeelsLike).
		Int64("hum", f.Main.Humidity).
		Float64("wind", f.Wind.Speed)
}
