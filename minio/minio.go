package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/sync/errgroup"
)

type Upload struct {
	Key         string
	Reader      io.Reader
	FileSize    int64
	ContentType string
}

type Store struct {
	client *minio.Client
}

type StoreOptions struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Secure    bool
}

func NewStore(options StoreOptions) *Store {
	client, err := minio.New(options.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(options.AccessKey, options.SecretKey, ""),
		Secure: options.Secure,
	})
	if err != nil {
		panic(fmt.Sprintf("create minio client: %v", err))
	}

	return &Store{client: client}
}

type cleanupRef struct {
	filePath string
	cleanup  func(ctx context.Context) error
}

func (m *Store) UploadMany(ctx context.Context, bucket string, files []Upload) (func(ctx context.Context) error, error) {
	if len(files) == 0 {
		return func(context.Context) error { return nil }, nil
	}

	var cleanups []cleanupRef
	var cleanupsMu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, file := range files {
		g.Go(func() error {
			cleanup, err := m.Upload(gctx, bucket, file)
			if err != nil {
				return fmt.Errorf("upload %q: %w", file.Key, err)
			}

			cleanupsMu.Lock()
			cleanups = append(cleanups, cleanupRef{
				filePath: file.Key,
				cleanup:  cleanup,
			})
			cleanupsMu.Unlock()

			return nil
		})
	}

	cleanup := func(ctx context.Context) error {
		if len(cleanups) == 0 {
			return nil
		}

		var wg sync.WaitGroup
		var errs error
		var errsMu sync.Mutex

		for _, ref := range cleanups {
			wg.Go(func() {
				if err := ref.cleanup(ctx); err != nil {
					errsMu.Lock()
					errs = errors.Join(errs, fmt.Errorf("cleanup %q: %w", ref.filePath, err))
					errsMu.Unlock()
				}
			})
		}

		wg.Wait()

		return errs
	}

	if err := g.Wait(); err != nil {
		if errCleanup := cleanup(context.WithoutCancel(ctx)); errCleanup != nil {
			return nil, errors.Join(
				fmt.Errorf("upload group failed: %w", err),
				fmt.Errorf("cleanup after upload failure: %w", errCleanup),
			)
		}

		return nil, fmt.Errorf("upload group failed: %w", err)
	}

	return cleanup, nil
}

func (m *Store) Upload(ctx context.Context, bucket string, in Upload) (func(ctx context.Context) error, error) {
	info, err := m.client.PutObject(ctx, bucket, in.Key, in.Reader, in.FileSize, minio.PutObjectOptions{
		ContentType: in.ContentType,
	})
	if err != nil {
		return nil, fmt.Errorf("put object: %w", err)
	}

	return func(ctx context.Context) error {
		err := m.client.RemoveObject(ctx, bucket, in.Key, minio.RemoveObjectOptions{
			VersionID: info.VersionID,
		})
		if err != nil {
			return fmt.Errorf("remove object: %w", err)
		}

		return nil
	}, nil
}

func (m *Store) Delete(ctx context.Context, bucket string, objectName string) error {
	err := m.client.RemoveObject(ctx, bucket, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("remove object: %w", err)
	}

	return nil
}

func (m *Store) CreateReadOnlyBuckets(ctx context.Context, buckets ...string) error {
	g, gctx := errgroup.WithContext(ctx)

	for _, bucket := range buckets {
		g.Go(func() error {
			if err := m.CreateReadOnlyBucket(gctx, bucket); err != nil {
				return fmt.Errorf("create read-only bucket %s: %w", bucket, err)
			}
			return nil
		})
	}

	return g.Wait()
}

// CreateReadOnlyBucket creates a bucket and sets up read-only public access policy
func (m *Store) CreateReadOnlyBucket(ctx context.Context, bucketName string) error {
	exists, err := m.client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}

	if !exists {
		err := m.client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
	}

	// Define read-only policy for the bucket
	readOnlyPolicy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": "*",
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}
		]
	}`, bucketName)

	err = m.client.SetBucketPolicy(ctx, bucketName, readOnlyPolicy)
	if err != nil {
		return fmt.Errorf("set bucket policy: %w", err)
	}

	return nil
}
