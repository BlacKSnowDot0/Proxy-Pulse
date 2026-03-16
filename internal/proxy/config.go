package proxy

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Queries                []string
	MaxReposPerQuery       int
	MaxGistsPerQuery       int
	MaxFilesPerSource      int
	MaxCandidates          int
	MaxFileBytes           int64
	ValidationTimeout      time.Duration
	ValidationStageTimeout time.Duration
	ValidationLogInterval  time.Duration
	Concurrency            int
	GitHubToken            string
	GitHubAPIBase          string
	GitHubRawBase          string
	GistWebBase            string
	UserAgent              string
	OutputDir              string
	IPEchoURL              string
}

func LoadConfigFromEnv() Config {
	return Config{
		Queries:                parseQueries(getEnv("PROXY_QUERIES", "")),
		MaxReposPerQuery:       getEnvInt("MAX_REPOS_PER_QUERY", 8),
		MaxGistsPerQuery:       getEnvInt("MAX_GISTS_PER_QUERY", 8),
		MaxFilesPerSource:      getEnvInt("MAX_FILES_PER_SOURCE", 24),
		MaxCandidates:          getEnvInt("MAX_CANDIDATES", 180),
		MaxFileBytes:           int64(getEnvInt("MAX_FILE_BYTES", 512*1024)),
		ValidationTimeout:      getEnvDuration("VALIDATION_TIMEOUT", 8*time.Second),
		ValidationStageTimeout: getEnvDuration("VALIDATION_STAGE_TIMEOUT", 2*time.Minute),
		ValidationLogInterval:  getEnvDuration("VALIDATION_LOG_INTERVAL", 5*time.Second),
		Concurrency:            getEnvInt("VALIDATION_CONCURRENCY", 24),
		GitHubToken:            strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		GitHubAPIBase:          getEnv("GITHUB_API_BASE", "https://api.github.com"),
		GitHubRawBase:          getEnv("GITHUB_RAW_BASE", "https://raw.githubusercontent.com"),
		GistWebBase:            getEnv("GIST_WEB_BASE", "https://gist.github.com"),
		UserAgent:              getEnv("USER_AGENT", "proxy-pulse/1.0"),
		OutputDir:              getEnv("OUTPUT_DIR", "."),
		IPEchoURL:              getEnv("IP_ECHO_URL", "http://api.ipify.org"),
	}
}

func parseQueries(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{
			"proxy list",
			"http proxy",
			"socks4 proxy",
			"socks5 proxy",
			"proxies.txt",
			"proxy scraper",
		}
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return parseQueries("")
	}
	return out
}

func getEnv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
