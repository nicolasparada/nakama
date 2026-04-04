package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strings"

	_ "golang.org/x/image/webp"

	charmlog "charm.land/log/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const minioBucket = "media"
const batchSize = 100

const sqlSelectPostsBatch = `
	SELECT id::STRING, media
	FROM posts
	WHERE media IS NOT NULL
		AND (@after_id = '' OR id::STRING > @after_id)
		AND EXISTS (
			SELECT 1
			FROM jsonb_array_elements(media) AS item
			WHERE item->>'path' IS NOT NULL
				AND (
					item->>'width' IS NULL
					OR item->>'height' IS NULL
					OR item->>'contentType' IS NULL
				)
		)
	ORDER BY id::STRING
	LIMIT @limit
`

const sqlUpdatePostMedia = `
	UPDATE posts
	SET media = @media::JSONB
	WHERE id = @id
`

func main() {
	logger := slog.New(charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		ReportTimestamp: true,
	}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("media migration failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	databaseURL := cmp.Or(os.Getenv("DATABASE_URL"), "postgresql://root@127.0.0.1:26257/nakama?sslmode=disable")
	minioEndpoint := cmp.Or(os.Getenv("S3_ENDPOINT"), "localhost:9000")
	minioAccessKey := cmp.Or(os.Getenv("S3_ACCESS_KEY"), "minioadmin")
	minioSecretKey := cmp.Or(os.Getenv("S3_SECRET_KEY"), "minioadmin")
	minioSecure := cmp.Or(os.Getenv("S3_SECURE"), "false") == "true"

	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	minioClient, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: minioSecure,
	})
	if err != nil {
		return fmt.Errorf("create minio client: %w", err)
	}

	slog.Info(
		"starting media migration",
		"component", "media-migrator",
		"batch_size", batchSize,
		"minio_endpoint", minioEndpoint,
		"minio_bucket", minioBucket,
	)

	m := &Migrator{
		DBPool:      dbPool,
		MinioClient: minioClient,
		BatchSize:   batchSize,
	}

	return m.Run(ctx)
}

type MediaDetails struct {
	Width       uint
	Height      uint
	ContentType string
}

type Migrator struct {
	DBPool      *pgxpool.Pool
	MinioClient *minio.Client
	BatchSize   int
}

var ErrObjectNotFound = fmt.Errorf("object not found")

type mediaItem struct {
	Path        string  `json:"path"`
	Width       *uint   `json:"width,omitempty"`
	Height      *uint   `json:"height,omitempty"`
	ContentType *string `json:"contentType,omitempty"`
}

type postMediaRow struct {
	ID    string
	Media []mediaItem
}

func (m *Migrator) Run(ctx context.Context) error {
	processedPosts := 0
	updatedPosts := 0
	errorCount := 0
	lastID := ""
	batchNumber := 0

	slog.Info("media migration loop started")

	for {
		batchNumber++
		slog.Info("loading post batch", "batch", batchNumber, "after_id", lastID)
		posts, skippedRows, err := m.loadPostsBatch(ctx, lastID)
		if err != nil {
			return err
		}
		if len(posts) == 0 {
			slog.Info("no more posts to process", "batch", batchNumber, "after_id", lastID, "skipped_rows", skippedRows)
			break
		}

		slog.Info("loaded post batch", "batch", batchNumber, "post_count", len(posts))
		batchProcessedPosts := 0
		batchUpdatedPosts := 0
		batchErrors := 0

		for _, post := range posts {
			processedPosts++
			batchProcessedPosts++
			changed, err := m.migratePostMedia(ctx, post)
			if err != nil {
				errorCount++
				batchErrors++
				lastID = post.ID
				continue
			}
			if changed {
				updatedPosts++
				batchUpdatedPosts++
			}
			lastID = post.ID
		}

		slog.Info(
			"finished post batch",
			"batch", batchNumber,
			"selected_posts", len(posts),
			"processed_posts", batchProcessedPosts,
			"updated_posts", batchUpdatedPosts,
			"error_posts", batchErrors,
			"skipped_rows", skippedRows,
			"total_processed_posts", processedPosts,
			"total_updated_posts", updatedPosts,
			"total_errors", errorCount,
		)
	}

	slog.Info("media migration finished", "processed_posts", processedPosts, "updated_posts", updatedPosts, "errors", errorCount)
	return nil
}

func (m *Migrator) loadPostsBatch(ctx context.Context, afterID string) ([]postMediaRow, int, error) {
	rows, err := m.DBPool.Query(ctx, sqlSelectPostsBatch, pgx.StrictNamedArgs{
		"after_id": afterID,
		"limit":    m.BatchSize,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("select posts batch: %w", err)
	}
	defer rows.Close()

	var posts []postMediaRow
	skippedRows := 0
	for rows.Next() {
		var postID string
		var rawMedia []byte
		if err := rows.Scan(&postID, &rawMedia); err != nil {
			skippedRows++
			continue
		}

		var media []mediaItem
		if err := json.Unmarshal(rawMedia, &media); err != nil {
			skippedRows++
			continue
		}

		posts = append(posts, postMediaRow{
			ID:    postID,
			Media: media,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, skippedRows, fmt.Errorf("iterate post media rows: %w", err)
	}

	return posts, skippedRows, nil
}

func (m *Migrator) migratePostMedia(ctx context.Context, post postMediaRow) (bool, error) {
	changed := false

	for i := range post.Media {
		if !post.Media[i].needsDetails() {
			continue
		}

		if post.Media[i].Path == "" {
			continue
		}

		details, err := m.GetMediaDetails(ctx, post.Media[i].Path)
		if err != nil {
			if err == ErrObjectNotFound {
				continue
			}
			return false, fmt.Errorf("get media details for path %q: %w", post.Media[i].Path, err)
		}

		if post.Media[i].applyDetails(details) {
			changed = true
		}
	}

	if !changed {
		return false, nil
	}

	rawMedia, err := json.Marshal(post.Media)
	if err != nil {
		return false, fmt.Errorf("marshal post %s media: %w", post.ID, err)
	}

	_, err = m.DBPool.Exec(ctx, sqlUpdatePostMedia, pgx.StrictNamedArgs{
		"id":    post.ID,
		"media": string(rawMedia),
	})
	if err != nil {
		return false, fmt.Errorf("update post %s media: %w", post.ID, err)
	}

	return true, nil
}

func (m mediaItem) needsDetails() bool {
	return m.Width == nil || *m.Width == 0 || m.Height == nil || *m.Height == 0 || m.ContentType == nil || *m.ContentType == ""
}

func (m *mediaItem) applyDetails(details MediaDetails) bool {
	changed := false

	if (m.ContentType == nil || *m.ContentType == "") && details.ContentType != "" {
		contentType := details.ContentType
		m.ContentType = &contentType
		changed = true
	}

	if (m.Width == nil || *m.Width == 0) && details.Width > 0 {
		width := details.Width
		m.Width = &width
		changed = true
	}

	if (m.Height == nil || *m.Height == 0) && details.Height > 0 {
		height := details.Height
		m.Height = &height
		changed = true
	}

	return changed
}

func (m *Migrator) GetMediaDetails(ctx context.Context, path string) (MediaDetails, error) {
	var details MediaDetails
	path = strings.TrimLeft(path, "/")
	if path == "" {
		return details, fmt.Errorf("empty media path")
	}

	object, err := m.MinioClient.GetObject(ctx, minioBucket, path, minio.GetObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return details, ErrObjectNotFound
		}

		return details, fmt.Errorf("get object from minio: %w", err)
	}

	defer object.Close()

	stat, err := object.Stat()
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return details, ErrObjectNotFound
		}

		return details, fmt.Errorf("stat object from minio: %w", err)
	}

	details.ContentType = stat.ContentType
	if details.ContentType == "" || details.ContentType == "application/octet-stream" {
		details.ContentType, err = detectContentType(object)
		if err != nil {
			return details, fmt.Errorf("detect content type: %w", err)
		}
	}

	if !strings.HasPrefix(details.ContentType, "image/") {
		return details, nil
	}

	if _, err := object.Seek(0, io.SeekStart); err != nil {
		return details, fmt.Errorf("seek object to start: %w", err)
	}

	img, _, err := image.DecodeConfig(object)
	if err != nil {
		return details, fmt.Errorf("decode image config: %w", err)
	}

	details.Width = uint(img.Width)
	details.Height = uint(img.Height)

	return details, nil
}

func detectContentType(r io.ReadSeeker) (string, error) {
	h := make([]byte, 512)
	_, err := r.Read(h)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read head: %w", err)
	}

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek to start: %w", err)
	}

	mediaType, _, err := mime.ParseMediaType(http.DetectContentType(h))
	if err != nil {
		return "", fmt.Errorf("parse media type: %w", err)
	}

	return mediaType, nil
}
