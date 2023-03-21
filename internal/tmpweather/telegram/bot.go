// Package telegram provides telegram chat processing.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/alukart32/tmp-weather/internal/pkg/zerologx"
	"github.com/alukart32/tmp-weather/internal/tmpweather/storage"
	"github.com/alukart32/tmp-weather/internal/tmpweather/weather"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

// MsgHandler  is a telegram bot message handler.
type MsgHandler struct {
	ForecastRepo *storage.WeatherForecastRepo
	Bot          *tgbotapi.BotAPI
	Forecaster   weather.CityForecaster
}

// NewMsgHandler returns a new MsgHandler.
func NewMsgHandler(
	forecaster weather.CityForecaster,
	forecastRepo *storage.WeatherForecastRepo,
	debugOn bool,
) (MsgHandler, error) {
	botAPIToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if len(botAPIToken) == 0 {
		return MsgHandler{}, fmt.Errorf("empty bot API token")
	}

	bot, err := tgbotapi.NewBotAPI(botAPIToken)
	if err != nil {
		return MsgHandler{}, err
	}

	if debugOn {
		bot.Debug = true
	}

	return MsgHandler{
		Bot:          bot,
		Forecaster:   forecaster,
		ForecastRepo: forecastRepo,
	}, nil
}

// Handle handles incoming chat messages.
func (p MsgHandler) Handle(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := p.Bot.GetUpdatesChan(u)
	go func() {
		logger := zerologx.Get()

		// https://stackoverflow.com/a/25677072
		cityNameReg := regexp.MustCompile("^([a-zA-Z\u0080-\u024F]+(?:. |-| |'))*[a-zA-Z\u0080-\u024F]*$")
		for {
			select {
			case <-ctx.Done():
				return
			case update, ok := <-updates:
				if !ok {
					return
				}

				// Ignore any non-command Messages.
				if update.Message == nil {
					continue
				}
				if !update.Message.IsCommand() {
					continue
				}

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
				msg.ReplyToMessageID = update.Message.MessageID

				// Update logger context.
				logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Dict("params", zerolog.Dict().
						Int64("chatID", update.Message.Chat.ID).
						Int("msgID", update.Message.MessageID),
					)
				})

				switch update.Message.Command() {
				case "info":
					cityName := update.Message.CommandArguments()
					if !cityNameReg.MatchString(cityName) {
						logger.Info().
							Str("cmd", "info").
							Msg("invalid name")
						msg.Text = "invalid city, try again"
						break
					}

					forecast, err := p.Forecaster.Forecast(ctx, cityName)
					if err != nil {
						logger.Error().
							Str("cmd", "info").
							Err(err).Send()

						switch err {
						case weather.ErrCityNotFound:
							msg.Text = "unknown city, try again"
						case weather.ErrExternal, weather.ErrCorruptedCall:
							msg.Text = "forecast error, try again"
						default:
							msg.Text = "internal error, try again"
						}
						break
					}
					logger.Debug().Object("forecast", forecast).Msg("forecast respond")

					err = p.ForecastRepo.Upsert(ctx, storage.WeatherForecast{
						MsgID:  update.Message.MessageID,
						City:   cityName,
						Desc:   forecast.Weather[0].Description,
						Temp:   forecast.Main.Temp,
						Hum:    forecast.Main.Humidity,
						Wind:   forecast.Wind.Speed,
						MadeAt: forecast.MadeAt,
					})
					if err != nil {
						logger.Error().
							Str("cmd", "info").
							Err(err).Send()
					}

					msg.Text = forecast.ToMsg()
				case "stat":
					stat, err := p.ForecastRepo.Stat(ctx)
					if err != nil {
						logger.Error().
							Str("cmd", "stat").
							Err(err).Send()
						if errors.Is(storage.ErrNoData, err) {
							msg.Text = "no stat data"
						} else {
							msg.Text = "could not stat, try again"
						}
						break
					}
					logger.Debug().Object("stat", stat).Msg("collected stat")

					msg.Text = stat.ToMsg()
				case "start":
					msg.Text = `Enter "/info city_name" to forecast`
				case "help":
					msg.Text = "/info city_name - do forecast\n/stat - take statistics"
				default:
					msg.Text = "I don't know that command"
				}
				p.reply(msg)
			}
		}
	}()
}

// reply sends a response message.
func (p *MsgHandler) reply(msg tgbotapi.MessageConfig) error {
	_, err := p.Bot.Send(msg)
	return err
}
