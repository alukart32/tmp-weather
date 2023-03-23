CREATE TABLE IF NOT EXISTS "forecasts" (
    msg_id SERIAL,
    city  text NOT NULL CHECK(LENGTH(city) > 0),
    description  text NOT NULL,
    temp real NOT NULL,
    hum  int NOT NULL CHECK(hum > 0.0),
    wind  real NOT NULL CHECK(wind > 0.0),
    made_at timestamptz NOT NULL
);