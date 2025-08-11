package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

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
	errLogger := slog.New(charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		ReportTimestamp: true,
	}))
	infoLogger := slog.New(charmlog.NewWithOptions(os.Stdout, charmlog.Options{
		ReportTimestamp: true,
	}))

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

	migrationStart := time.Now()
	infoLogger.Info("starting cockroach migrations")

	if err := migrator.Migrate(context.Background(), dbPool, cockroach.MigrationsFS); err != nil {
		return fmt.Errorf("migrate cockroach schema: %w", err)
	}

	infoLogger.Info("finished cockroach migrations", "took", time.Since(migrationStart))

	minioClient, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioSecure,
	})
	if err != nil {
		return fmt.Errorf("create minio client: %w", err)
	}

	minio := nakamaminio.New(context.Background(), minioClient, cfg.MinioCleanupTimeout)
	go func() {
		for err := range minio.Errs() {
			errLogger.Error("minio error", "error", err)
		}
	}()

	bucketsStart := time.Now()
	infoLogger.Info("creating minio buckets")

	if err := minio.CreateReadOnlyBucket(context.Background(), "post-attachments"); err != nil {
		return fmt.Errorf("create minio bucket: %w", err)
	}

	infoLogger.Info("finished creating minio buckets", "took", time.Since(bucketsStart))

	svcErrChan := make(chan error, 1)
	defer close(svcErrChan)

	go func() {
		for err := range svcErrChan {
			errLogger.Error("service error", "error", err)
		}
	}()

	svc := &service.Service{
		Cockroach:         cockroach.New(dbPool),
		Minio:             minio,
		Errs:              svcErrChan,
		BaseContext:       context.Background(),
		BackgroundTimeout: 15 * time.Second,
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
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start nakama server: %w", err)
	}

	svc.Wait()

	return nil
}
