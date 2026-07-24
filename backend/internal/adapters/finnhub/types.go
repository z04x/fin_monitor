package finnhub

type EarningsCalendarResponse struct {
	EarningsCalendar []CalendarEvent `json:"earningsCalendar"`
}

type CalendarEvent struct {
	Symbol          string   `json:"symbol"`
	Date            string   `json:"date"`
	Hour            string   `json:"hour"`
	Quarter         int      `json:"quarter"`
	Year            int      `json:"year"`
	EPSEstimate     *float64 `json:"epsEstimate"`
	EPSActual       *float64 `json:"epsActual"`
	RevenueEstimate *float64 `json:"revenueEstimate"`
	RevenueActual   *float64 `json:"revenueActual"`
}

// EarningHistory is one row of stock/earnings — the beat/miss history.
// NOTE: Period is the fiscal quarter END, not the publication date. Do not
// use it as the report date (see spec §6 Step 0).
type EarningHistory struct {
	Symbol          string   `json:"symbol"`
	Period          string   `json:"period"`
	Year            int      `json:"year"`
	Quarter         int      `json:"quarter"`
	EPSEstimate     *float64 `json:"estimate"`
	EPSActual       *float64 `json:"actual"`
	Surprise        *float64 `json:"surprise"`
	SurprisePercent *float64 `json:"surprisePercent"`
}

// MetricsResponse wraps stock/metric?metric=all. We only decode the handful
// of TTM/valuation fields the /reports card shows.
type MetricsResponse struct {
	Metric Metrics `json:"metric"`
}

type Metrics struct {
	PE               *float64 `json:"peTTM"`
	ROE              *float64 `json:"roeTTM"`
	GrossMargin      *float64 `json:"grossMarginTTM"`
	OperatingMargin  *float64 `json:"operatingMarginTTM"`
	NetMargin        *float64 `json:"netProfitMarginTTM"`
	RevenueGrowthYoY *float64 `json:"revenueGrowthTTMYoy"`
	Week52High       *float64 `json:"52WeekHigh"`
	Week52Low        *float64 `json:"52WeekLow"`
}

type Profile struct {
	Ticker               string   `json:"ticker"`
	Name                 string   `json:"name"`
	Country              string   `json:"country"`
	Currency             string   `json:"currency"`
	EstimateCurrency     string   `json:"estimateCurrency"`
	Exchange             string   `json:"exchange"`
	IPO                  string   `json:"ipo"`
	MarketCapitalization *float64 `json:"marketCapitalization"`
	Logo                 string   `json:"logo"`
	ShareOutstanding     *float64 `json:"shareOutstanding"`
	Industry             string   `json:"finnhubIndustry"`
	Phone                string   `json:"phone"`
	WebURL               string   `json:"weburl"`
	FloatingShare        *float64 `json:"floatingShare"`
}
