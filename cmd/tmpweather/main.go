package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alukart32/tmp-weather/internal/pkg/db/postgres"
	"github.com/alukart32/tmp-weather/internal/pkg/zerologx"
	"github.com/alukart32/tmp-weather/internal/tmpweather/storage"
	"github.com/alukart32/tmp-weather/internal/tmpweather/telegram"
	"github.com/alukart32/tmp-weather/internal/tmpweather/weather"
)

func main() {
	logger := zerologx.Get()

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.Info().Msg("prepare postgres pool")
	pgxPool, err := postgres.Get()
	if err != nil {
		logger.Panic().Err(err).Msg("prepare postgres pool")
	}

	logger.Info().Msg("prepare forecast repo")
	forecastRepo, err := storage.NewWeatherForecastRepo(pgxPool)
	if err != nil {
		logger.Panic().Err(err).Msg("prepare forecast repo")
	}

	logger.Info().Msg("prepare forecaster")
	forecaster := weather.NewCityForecaster(appCtx)

	logger.Info().Msg("prepare telegram bot msgs handler")
	msgsHandler, err := telegram.NewMsgHandler(
		forecaster,
		forecastRepo,
		false,
	)
	if err != nil {
		logger.Panic().Err(err).Msg("prepare telegram bot msgs handler")
	}

	logger.Info().Msg("start telegram bot msgs handler")
	msgsHandler.Handle(appCtx)

	// Waiting signal.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	s := <-interrupt
	logger.Info().Msg(s.String())
}
