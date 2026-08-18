package proxy

import (
	"context"
	"log"
	"time"

	geovault "github.com/obeliskdev/geovault"
)

type GeoLookup struct {
	CountryCode string
	CountryName string
	ASN         uint32
	Org         string
}

type GeoResolver interface {
	Lookup(ip string) (GeoLookup, bool)
	Close() error
}

type geovaultResolver struct {
	client *geovault.Client
}

// NewGeoResolver builds an offline GeoIP resolver. It never fails hard: on
// download errors a nil resolver is returned and callers publish without geo
// fields rather than failing the run.
func NewGeoResolver(cfg Config) GeoResolver {
	if cfg.GeoIPDisabled {
		return nil
	}

	opts := []geovault.Option{
		geovault.WithAutoUpdate(false),
		geovault.WithCityDatabaseURL(cfg.GeoIPCityURL),
		geovault.WithASNDatabaseURL(cfg.GeoIPASNURL),
		geovault.WithDownloadRetries(2),
		geovault.WithTimeout(5 * time.Minute),
	}
	if cfg.GeoVaultDataDir != "" {
		opts = append(opts, geovault.WithDataDir(cfg.GeoVaultDataDir))
	}
	if cfg.UserAgent != "" {
		opts = append(opts, geovault.WithUserAgent(cfg.UserAgent))
	}

	client, err := geovault.New(opts...)
	if err != nil {
		log.Printf("geoip resolver unavailable (%v); publishing without geo fields", err)
		return nil
	}

	updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := client.Update(updateCtx); err != nil {
		log.Printf("geoip database refresh failed (%v); using cached databases if present", err)
	}

	return &geovaultResolver{client: client}
}

func (r *geovaultResolver) Lookup(ip string) (GeoLookup, bool) {
	result, err := r.client.Lookup(ip)
	if err != nil {
		return GeoLookup{}, false
	}
	return GeoLookup{
		CountryCode: result.Country.Code,
		CountryName: result.Country.Name,
		ASN:         uint32(result.ASN),
		Org:         result.Organization,
	}, true
}

func (r *geovaultResolver) Close() error {
	return r.client.Close()
}
