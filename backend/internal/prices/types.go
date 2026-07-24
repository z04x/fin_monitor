package prices

type Metadata struct {
	Ticker       string `json:"ticker"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	StartDate    string `json:"startDate"`
	EndDate      string `json:"endDate"`
	ExchangeCode string `json:"exchangeCode"`
}

type DailyPrice struct {
	Date        string   `json:"date"`
	Open        *float64 `json:"open"`
	High        *float64 `json:"high"`
	Low         *float64 `json:"low"`
	Close       *float64 `json:"close"`
	Volume      *int64   `json:"volume"`
	AdjOpen     *float64 `json:"adjOpen"`
	AdjHigh     *float64 `json:"adjHigh"`
	AdjLow      *float64 `json:"adjLow"`
	AdjClose    *float64 `json:"adjClose"`
	AdjVolume   *int64   `json:"adjVolume"`
	DivCash     *float64 `json:"divCash"`
	SplitFactor *float64 `json:"splitFactor"`
}
