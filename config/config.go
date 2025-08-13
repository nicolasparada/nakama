package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
)

type Config struct {
	CockroachURL   string        `ff:"long: cockroach-url, default: postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable, usage: URL for the CockroachDB database"`
	Port           uint32        `ff:"long: port, short: p, default: 4444, usage: Port for the HTTP server"`
	MinioEndpoint  string        `ff:"long: minio-endpoint, default: localhost:9000, usage: MinIO endpoint"`
	MinioAccessKey string        `ff:"long: minio-access-key, default: minioadmin, usage: MinIO access key"`
	MinioSecretKey string        `ff:"long: minio-secret-key, default: minioadmin, usage: MinIO secret key"`
	MinioSecure    bool          `ff:"long: minio-secure, default: false, usage: Use secure connection to MinIO"`
	CleanupTimeout time.Duration `ff:"long: cleanup-timeout, default: 5s, usage: Timeout for background cleanup operations"`
}

func Load() (Config, error) {
	_ = godotenv.Load()

	var cfg Config
	fs := ff.NewFlagSetFrom("nakama", &cfg)
	err := ff.Parse(fs, os.Args[1:], ff.WithEnvVarPrefix("NAKAMA"))
	if errors.Is(err, ff.ErrHelp) {
		fmt.Println(ffhelp.Flags(fs))
		os.Exit(0)
	}

	return cfg, err
}
