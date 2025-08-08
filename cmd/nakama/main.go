package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/alexedwards/scs/pgxstore"
	charmlog "github.com/charmbracelet/log"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nicolasparada/nakama/cockroach"
	"github.com/nicolasparada/nakama/cockroach/migrator"
	"github.com/nicolasparada/nakama/config"
	nakamaminio "github.com/nicolasparada/nakama/minio"
	"github.com/nicolasparada/nakama/service"
	"github.com/nicolasparada/nakama/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPool, err := pgxpool.New(context.Background(), cfg.CockroachURL)
	if err != nil {
		return fmt.Errorf("open cockroach connection pool: %w", err)
	}

	defer dbPool.Close()

	if err := dbPool.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping cockroach: %w", err)
	}

	if err := migrator.Migrate(context.Background(), dbPool, cockroach.MigrationsFS); err != nil {
		return fmt.Errorf("migrate cockroach schema: %w", err)
	}

	minioClient, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioSecure,
	})
	if err != nil {
		return fmt.Errorf("create minio client: %w", err)
	}

	minio := nakamaminio.New(context.Background(), minioClient, cfg.MinioCleanupTimeout)
	if err := minio.CreateReadOnlyBucket(context.Background(), "post-attachments"); err != nil {
		return fmt.Errorf("create minio bucket: %w", err)
	}

	errLogger := slog.New(charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		ReportTimestamp: true,
	}))
	infoLogger := slog.New(charmlog.NewWithOptions(os.Stdout, charmlog.Options{
		ReportTimestamp: true,
	}))

	cockroach := cockroach.New(dbPool)

	svc := &service.Service{
		Cockroach: cockroach,
		Minio:     minio,
	}
	handler := &web.Handler{
		Service:       svc,
		ErrorLogger:   errLogger,
		SesssionStore: pgxstore.New(dbPool),
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: handler,
	}

	infoLogger.Info("starting nakama server", "url", fmt.Sprintf("http://localhost:%d", cfg.Port))
	return srv.ListenAndServe()
}
