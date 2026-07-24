CREATE TABLE price_daily (
    id                  BIGSERIAL PRIMARY KEY,
    ticker              TEXT NOT NULL,
    date                DATE NOT NULL,
    open                NUMERIC,
    high                NUMERIC,
    low                 NUMERIC,
    close               NUMERIC,
    volume              BIGINT,
    adj_open            NUMERIC,
    adj_high            NUMERIC,
    adj_low             NUMERIC,
    adj_close           NUMERIC,
    adj_volume          BIGINT,
    div_cash            NUMERIC,
    split_factor        NUMERIC,
    UNIQUE (ticker, date)
);

CREATE INDEX idx_price_ticker_date ON price_daily (ticker, date);

CREATE TABLE earnings_reaction (
    id                  BIGSERIAL PRIMARY KEY,
    ticker              TEXT NOT NULL,
    report_date         DATE NOT NULL,
    hour                TEXT,
    quarter             SMALLINT,
    year                SMALLINT,
    surprise_percent    NUMERIC,
    price_before        NUMERIC,
    price_after         NUMERIC,
    reaction_percent    NUMERIC,
    computed_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticker, report_date)
);
