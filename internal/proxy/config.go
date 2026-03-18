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
	ShardCount             int
	GitHubToken            string
	GitHubAPIBase          string
	GitHubRawBase          string
	GistWebBase            string
	UserAgent              string
	OutputDir              string
	IPEchoURL              string
	IPEchoURLPrimary       string
	IPEchoURLSecondary     string
	DirectIPEchoURL        string
	GEOIPURLTemplate       string
	AnonCheckURL           string
}

func LoadConfigFromEnv() Config {
	legacyIPEchoURL := getEnv("IP_ECHO_URL", "http://api.ipify.org")
	primaryIPEchoURL := getEnv("IP_ECHO_URL_PRIMARY", legacyIPEchoURL)
	return Config{
		Queries:                parseQueries(getEnv("PROXY_QUERIES", "")),
		MaxReposPerQuery:       getEnvInt("MAX_REPOS_PER_QUERY", 12),
		MaxGistsPerQuery:       getEnvInt("MAX_GISTS_PER_QUERY", 12),
		MaxFilesPerSource:      getEnvInt("MAX_FILES_PER_SOURCE", 32),
		MaxCandidates:          getEnvNonNegativeInt("MAX_CANDIDATES", 0),
		MaxFileBytes:           int64(getEnvInt("MAX_FILE_BYTES", 1024*1024)),
		ValidationTimeout:      getEnvDuration("VALIDATION_TIMEOUT", 8*time.Second),
		ValidationStageTimeout: getEnvDuration("VALIDATION_STAGE_TIMEOUT", 3*time.Minute),
		ValidationLogInterval:  getEnvDuration("VALIDATION_LOG_INTERVAL", 5*time.Second),
		Concurrency:            getEnvInt("VALIDATION_CONCURRENCY", 32),
		ShardCount:             getEnvInt("VALIDATION_SHARDS", 16),
		GitHubToken:            strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		GitHubAPIBase:          getEnv("GITHUB_API_BASE", "https://api.github.com"),
		GitHubRawBase:          getEnv("GITHUB_RAW_BASE", "https://raw.githubusercontent.com"),
		GistWebBase:            getEnv("GIST_WEB_BASE", "https://gist.github.com"),
		UserAgent:              getEnv("USER_AGENT", "proxy-pulse/1.0"),
		OutputDir:              getEnv("OUTPUT_DIR", "."),
		IPEchoURL:              legacyIPEchoURL,
		IPEchoURLPrimary:       primaryIPEchoURL,
		IPEchoURLSecondary:     getEnv("IP_ECHO_URL_SECONDARY", "http://ifconfig.me/ip"),
		DirectIPEchoURL:        getEnv("DIRECT_IP_ECHO_URL", primaryIPEchoURL),
		GEOIPURLTemplate:       getEnv("GEOIP_URL_TEMPLATE", "http://ip-api.com/json/%s?fields=status,country,countryCode"),
		AnonCheckURL:           getEnv("ANON_CHECK_URL", "http://httpbin.org/get"),
	}
}

func parseQueries(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{
			"proxy list",
			"free proxy",
			"proxy-list",
			"proxylist",
			"http proxy",
			"https proxy",
			"socks4 proxy",
			"socks5 proxy",
			"socks proxy",
			"open proxy",
			"working proxy",
			"alive proxy",
			"proxies.txt",
			"proxy.txt",
			"http.txt",
			"socks4.txt",
			"socks5.txt",
			"valid proxy",
			"fresh proxy",
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

func getEnvNonNegativeInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
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
