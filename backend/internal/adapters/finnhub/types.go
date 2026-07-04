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
