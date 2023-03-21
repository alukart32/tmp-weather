# TmpWeather

TmpWeather is a telegram bot for predicting the current weather by city name.

It consists of the following parts:

1. go service (App)
2. telegram bot

App can handle the following telegram commands:

- start
- help
- info
- stat

## Weather forecast

Weather forecast data is taken from the resource https://openweathermap.org/.

The following query is used to get the current weather forecast: https://api.openweathermap.org/data/2.5/weather?units=metric.
For details read: https://openweathermap.org/current#name.

Specification of weather data:https://openweathermap.org/weather-data.

Forecast data: https://openweathermap.org/current#current_JSON.

## Use cases

A typical scenario for using a telegram bot:

1. /start - start chatting with bot
2. /info city_name - do forecast for the city
3. /stat - get some statistical data
4. /help - get help

While receiving the current weather forecast, the following errors are possible:

- city not found
- forecast error
- internal error

While receiving the statistical data, the following errors are possible:

- no data

## Build, deploy and run

To run the telegram bot server side locally, you need to perform the following steps:

1. install docker compose
2. git clone project
3. set the env parameters in the .env file
4. docker compose up

To set env parameters you need to know:

- Telegram bot access API token (get after bot creation)
- openweathermap API token (get from https://openweathermap.org/ after registration)
