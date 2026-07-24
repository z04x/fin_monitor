CREATE TABLE company_profile (
    ticker              TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    exchange            TEXT,
    currency            TEXT,
    industry            TEXT,
    ipo_date            DATE,
    market_cap          NUMERIC,
    share_outstanding   NUMERIC,
    logo_url            TEXT,
    web_url             TEXT,
    description         TEXT,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE earnings_calendar (
    id                  BIGSERIAL PRIMARY KEY,
    ticker              TEXT NOT NULL,
    report_date         DATE NOT NULL,
    hour                TEXT,
    quarter             SMALLINT,
    year                SMALLINT,
    eps_estimate        NUMERIC,
    eps_actual          NUMERIC,
    revenue_estimate    NUMERIC,
    revenue_actual      NUMERIC,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticker, report_date, quarter, year)
);

CREATE INDEX idx_calendar_date ON earnings_calendar (report_date);

CREATE TABLE company_earnings (
    id                  BIGSERIAL PRIMARY KEY,
    ticker              TEXT NOT NULL,
    period              DATE NOT NULL,
    year                SMALLINT,
    quarter             SMALLINT,
    eps_estimate        NUMERIC,
    eps_actual          NUMERIC,
    surprise            NUMERIC,
    surprise_percent    NUMERIC,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticker, period)
);

CREATE TABLE recommendations (
    id                  BIGSERIAL PRIMARY KEY,
    ticker              TEXT NOT NULL,
    period              DATE NOT NULL,
    strong_buy          SMALLINT,
    buy                 SMALLINT,
    hold                SMALLINT,
    sell                SMALLINT,
    strong_sell         SMALLINT,
    UNIQUE (ticker, period)
);
