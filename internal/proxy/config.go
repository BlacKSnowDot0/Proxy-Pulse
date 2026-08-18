package proxy

import (
	"crypto/x509"
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
	MaxFreshCandidates     int
	MaxFileBytes           int64
	ValidationTimeout      time.Duration
	ValidationStageTimeout time.Duration
	ValidationLogInterval  time.Duration
	EnrichmentTimeout      time.Duration
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
	HTTPSProbeURL          string
	AnonCheckURL           string
	AnonCheckURLSecondary  string
	GeoIPCityURL           string
	GeoIPASNURL            string
	GeoVaultDataDir        string

	GeoIPDisabled bool
	httpsTLSRoots *x509.CertPool
}

const (
	defaultGeoIPCityURL = "https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-City.mmdb"
	defaultGeoIPASNURL  = "https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-ASN.mmdb"
)

func LoadConfigFromEnv() Config {
	legacyIPEchoURL := getEnv("IP_ECHO_URL", "http://api.ipify.org")
	primaryIPEchoURL := getEnv("IP_ECHO_URL_PRIMARY", legacyIPEchoURL)
	return Config{
		Queries:                parseQueries(getEnv("PROXY_QUERIES", "")),
		MaxReposPerQuery:       getEnvInt("MAX_REPOS_PER_QUERY", 12),
		MaxGistsPerQuery:       getEnvInt("MAX_GISTS_PER_QUERY", 12),
		MaxFilesPerSource:      getEnvInt("MAX_FILES_PER_SOURCE", 32),
		MaxCandidates:          getEnvNonNegativeInt("MAX_CANDIDATES", 0),
		MaxFreshCandidates:     getEnvNonNegativeInt("MAX_FRESH_CANDIDATES", 0),
		MaxFileBytes:           int64(getEnvInt("MAX_FILE_BYTES", 1024*1024)),
		ValidationTimeout:      getEnvDuration("VALIDATION_TIMEOUT", 8*time.Second),
		ValidationStageTimeout: getEnvDuration("VALIDATION_STAGE_TIMEOUT", 3*time.Minute),
		ValidationLogInterval:  getEnvDuration("VALIDATION_LOG_INTERVAL", 5*time.Second),
		EnrichmentTimeout:      getEnvDuration("ENRICHMENT_TIMEOUT", 8*time.Second),
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
		HTTPSProbeURL:          getEnv("HTTPS_PROBE_URL", "https://api.ipify.org"),
		AnonCheckURL:           getEnv("ANON_CHECK_URL", "http://httpbin.org/get"),
		AnonCheckURLSecondary:  getEnv("ANON_CHECK_URL_SECONDARY", ""),
		GeoIPCityURL:           getEnv("GEOIP_CITY_URL", defaultGeoIPCityURL),
		GeoIPASNURL:            getEnv("GEOIP_ASN_URL", defaultGeoIPASNURL),
		GeoVaultDataDir:        strings.TrimSpace(os.Getenv("GEOVAULT_DATA_DIR")),
		GeoIPDisabled:          getEnvBool("GEOIP_DISABLED", false),
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

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
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
