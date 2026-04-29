package config

import (
	"os"
)

type Config struct {
	SiteAddr     string
	InternalAddr string
	InternalKey  string
	Seed         bool
}

func Load() Config {
	return Config{
		SiteAddr:     getenv("SITE_ADDR", ":8080"),
		InternalAddr: getenv("INTERNAL_ADDR", ":8081"),
		InternalKey:  getenv("INTERNAL_API_KEY", "dev-internal-key"),
		Seed:         getenv("SEED", "true") == "true",
	}
}

func getenv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}
