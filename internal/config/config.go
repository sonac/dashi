package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr             string
	DataDir          string
	DBPath           string
	DockerSocket     string
	MetricsInterval  time.Duration
	RulesInterval    time.Duration
	RetentionDays    int
	DebugRestarts    bool
	SkipSelfLogs     bool
	TelegramBotToken string
	TelegramChatID   string
}

func Load() Config {
	dataDir := getenv("APP_DATA_DIR", "./data")
	retention := getenvInt("APP_RETENTION_DAYS", 14)
	return Config{
		Addr:             getenv("APP_ADDR", ":8080"),
		DataDir:          dataDir,
		DBPath:           getenv("APP_DB_PATH", dataDir+"/app.db"),
		DockerSocket:     getenv("DOCKER_SOCKET", "/var/run/docker.sock"),
		MetricsInterval:  getenvDuration("APP_METRICS_INTERVAL", 10*time.Second),
		RulesInterval:    getenvDuration("APP_RULES_INTERVAL", 15*time.Second),
		RetentionDays:    retention,
		DebugRestarts:    getenvBool("APP_DEBUG_RESTART_ALERTS", false),
		SkipSelfLogs:     getenvBool("APP_SKIP_SELF_LOGS", true),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getenvInt(k string, d int) int {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func getenvDuration(k string, d time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	dur, err := time.ParseDuration(v)
	if err != nil {
		return d
	}
	return dur
}

func getenvBool(k string, d bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return d
	}
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		return true
	}
	if v == "0" || v == "false" || v == "no" || v == "off" {
		return false
	}
	return d
}
