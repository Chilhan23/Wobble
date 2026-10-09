package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// App
	AppName string
	AppEnv  string // development | production
	AppPort string
	BaseURL string

	// Database
	DatabaseURL string

	// Auth: kunci HMAC untuk token tiket (min. 32 karakter)
	TokenSecret       string
	TokenTTL          time.Duration
	FileSigningSecret string

	// Telegram
	TelegramBotToken      string
	TelegramChatID        int64 // id supergroup, biasanya negatif (-100...)
	TelegramWebhookSecret string

	// AI (opsional: kalau AIAPIKey kosong, AI dinonaktifkan)
	AIEndpoint     string
	AIAPIKey       string
	AIModel        string
	AITimeout      time.Duration
	AIHistoryLimit int

	// Upload
	UploadDir        string
	MaxUploadImageMB int64
	MaxUploadVideoMB int64
}

func (c *Config) IsProduction() bool { return c.AppEnv == "production" }
func (c *Config) AIEnabled() bool    { return c.AIAPIKey != "" }

// Load membaca .env (jika ada) lalu environment variable, memvalidasi semuanya,
// dan mengembalikan SEMUA error sekaligus agar tidak perlu tebak satu per satu.
func Load() (*Config, error) {
	_ = godotenv.Load()

	l := &loader{}

	tokenSecret := l.required("TOKEN_SECRET")
	fileSigningSecret := l.str("FILE_SIGNING_SECRET", "")
	if fileSigningSecret == "" {
		fileSigningSecret = tokenSecret
	}

	cfg := &Config{
		AppName: l.str("APP_NAME", "wobble"),
		AppEnv:  l.str("APP_ENV", "development"),
		AppPort: l.str("APP_PORT", "8080"),
		BaseURL: strings.TrimRight(l.str("BASE_URL", "http://localhost:8080"), "/"),

		DatabaseURL: l.required("DATABASE_URL"),

		TokenSecret:       tokenSecret,
		TokenTTL:          time.Duration(l.int64("TOKEN_TTL_HOURS", 24)) * time.Hour,
		FileSigningSecret: fileSigningSecret,

		TelegramBotToken:      l.required("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:        l.requiredInt64("TELEGRAM_CHAT_ID"),
		TelegramWebhookSecret: l.required("TELEGRAM_WEBHOOK_SECRET"),

		AIEndpoint:     l.str("AI_ENDPOINT", "https://openrouter.ai/api/v1/chat/completions"),
		AIAPIKey:       os.Getenv("AI_API_KEY"),
		AIModel:        l.str("AI_MODEL", "openai/gpt-4o-mini"),
		AITimeout:      time.Duration(l.int64("AI_TIMEOUT_SECONDS", 25)) * time.Second,
		AIHistoryLimit: int(l.int64("AI_HISTORY_LIMIT", 10)),

		UploadDir:        l.str("UPLOAD_DIR", "./uploads"),
		MaxUploadImageMB: l.int64("MAX_UPLOAD_IMAGE_MB", 5),
		MaxUploadVideoMB: l.int64("MAX_UPLOAD_VIDEO_MB", 15),
	}

	cfg.validate(l)

	if len(l.errs) > 0 {
		return nil, fmt.Errorf("konfigurasi tidak valid:\n%w", errors.Join(l.errs...))
	}
	return cfg, nil
}

func (c *Config) validate(l *loader) {
	if c.AppEnv != "development" && c.AppEnv != "production" {
		l.fail("APP_ENV harus 'development' atau 'production', bukan %q", c.AppEnv)
	}
	if c.TokenSecret != "" && len(c.TokenSecret) < 32 {
		l.fail("TOKEN_SECRET minimal 32 karakter (buat dengan: openssl rand -hex 32)")
	}
	if c.TelegramChatID == 0 {
		// requiredInt64 sudah melaporkan jika kosong/tidak valid; ini menangkap nilai "0"
		if os.Getenv("TELEGRAM_CHAT_ID") == "0" {
			l.fail("TELEGRAM_CHAT_ID tidak boleh 0")
		}
	}
	if c.TokenTTL <= 0 {
		l.fail("TOKEN_TTL_HOURS harus > 0")
	}
	if c.AIHistoryLimit < 1 {
		l.fail("AI_HISTORY_LIMIT harus >= 1")
	}
	if c.MaxUploadImageMB < 1 || c.MaxUploadVideoMB < 1 {
		l.fail("MAX_UPLOAD_IMAGE_MB dan MAX_UPLOAD_VIDEO_MB harus >= 1")
	}
	if c.IsProduction() && strings.HasPrefix(c.BaseURL, "http://") {
		l.fail("BASE_URL harus https:// di production (Telegram webhook dan wss:// mewajibkannya)")
	}
}

// ---------------------------------------------------------------------
// loader: helper kecil yang mengumpulkan error
// ---------------------------------------------------------------------

type loader struct{ errs []error }

func (l *loader) fail(format string, args ...any) {
	l.errs = append(l.errs, fmt.Errorf(format, args...))
}

func (l *loader) str(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func (l *loader) required(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		l.fail("%s wajib diisi", key)
	}
	return v
}

func (l *loader) int64(key string, def int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		l.fail("%s harus berupa angka, bukan %q", key, raw)
		return def
	}
	return n
}

func (l *loader) requiredInt64(key string) int64 {
	raw := l.required(key)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		l.fail("%s harus berupa angka, bukan %q", key, raw)
		return 0
	}
	return n
}
