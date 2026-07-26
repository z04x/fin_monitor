package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	FrontendOrigin        string
	FinnhubToken          string
	FinnhubBaseURL        string
	TiingoToken           string
	TiingoBaseURL         string
	CalendarWindowDays    int
	CompanyCacheTTLHours  int
	NewsCacheTTLHours     int
	PriceRefreshHourUTC   int
	TelegramBotToken      string
	TelegramChatID        string
	TelegramWebhookSecret string
	EdgarUserAgent        string
}

// Load reads process env vars, falling back to values from a .env file in
// the working directory if present (local dev only; CI/prod set real env).
func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:                  getEnv("PORT", "8080"),
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		FrontendOrigin:        getEnv("FRONTEND_ORIGIN", "http://localhost:3000"),
		FinnhubToken:          getEnv("FINNHUB_TOKEN", ""),
		FinnhubBaseURL:        getEnv("FINNHUB_BASE_URL", "https://finnhub.io/api/v1"),
		TiingoToken:           getEnv("TIINGO_TOKEN", ""),
		TiingoBaseURL:         getEnv("TIINGO_BASE_URL", "https://api.tiingo.com"),
		CalendarWindowDays:    getEnvInt("CALENDAR_WINDOW_DAYS", 30),
		CompanyCacheTTLHours:  getEnvInt("COMPANY_CACHE_TTL_HOURS", 24),
		NewsCacheTTLHours:     getEnvInt("NEWS_CACHE_TTL_HOURS", 6),
		PriceRefreshHourUTC:   getEnvInt("PRICE_REFRESH_HOUR_UTC", 23),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:        getEnv("TELEGRAM_CHAT_ID", ""),
		TelegramWebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		// SEC requires a descriptive User-Agent with contact info; override in
		// prod via env with a real email.
		EdgarUserAgent: getEnv("EDGAR_USER_AGENT", "MarketAnalyzer/1.0 (+https://github.com/z04x/fin_monitor)"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
