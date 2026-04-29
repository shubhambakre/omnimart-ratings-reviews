package config

import (
	"os"
)

type Config struct {
	SiteAddr      string
	InternalAddr  string
	InternalKey   string
	Seed          bool
	StorageDriver string // "memory" (default) or "sqlite"
	StoragePath   string // path to SQLite file when driver=sqlite
}

func Load() Config {
	return Config{
		SiteAddr:      getenv("SITE_ADDR", ":8080"),
		InternalAddr:  getenv("INTERNAL_ADDR", ":8081"),
		InternalKey:   getenv("INTERNAL_API_KEY", "dev-internal-key"),
		Seed:          getenv("SEED", "true") == "true",
		StorageDriver: getenv("STORAGE_DRIVER", "memory"),
		StoragePath:   getenv("STORAGE_PATH", "./data/omnimart.db"),
	}
}

func getenv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}
