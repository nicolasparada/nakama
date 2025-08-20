package minio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/nicolasparada/nakama/types"
	"golang.org/x/sync/errgroup"
)

type Minio struct {
	client         *minio.Client
	baseCtx        context.Context
	cleanupTimeout time.Duration
	wg             sync.WaitGroup
	errs           chan error
}

func New(ctx context.Context, client *minio.Client, cleanupTimeout time.Duration) *Minio {
	return &Minio{
		client:         client,
		baseCtx:        ctx,
		cleanupTimeout: cleanupTimeout,
		errs:           make(chan error, 1),
	}
}

func (m *Minio) UploadMany(ctx context.Context, bucket string, files []types.Attachment) (func(), error) {
	if len(files) == 0 {
		return func() {}, nil
	}

	var cleanupFuncs []func()

	g, gctx := errgroup.WithContext(ctx)

	for _, file := range files {
		g.Go(func() error {
			cleanup, err := m.Upload(gctx, bucket, file)
			if err != nil {
				return fmt.Errorf("upload %s failed: %w", file.Path, err)
			}

			cleanupFuncs = append(cleanupFuncs, cleanup)
			return nil
		})
	}

	cleanup := func() {
		if len(cleanupFuncs) == 0 {
			return
		}

		var wg sync.WaitGroup

		for _, fn := range cleanupFuncs {
			wg.Go(fn)
		}

		wg.Wait()
	}

	if err := g.Wait(); err != nil {
		go cleanup()
		return nil, fmt.Errorf("upload group failed: %w", err)
	}

	return cleanup, nil
}

func (m *Minio) Upload(ctx context.Context, bucket string, file types.Attachment) (func(), error) {
	info, err := m.client.PutObject(ctx, bucket, file.Path, file.Reader(), int64(file.FileSize), minio.PutObjectOptions{
		ContentType: file.ContentType,
	})
	if err != nil {
		return nil, fmt.Errorf("put object: %w", err)
	}

	return func() {
		m.wg.Go(func() {
			defer func() {
				if rcv := recover(); rcv != nil {
					select {
					case m.errs <- fmt.Errorf("remove object panic %s: %v", file.Path, rcv):
					default:
					}
				}
			}()

			ctx, cancel := context.WithTimeout(m.baseCtx, m.cleanupTimeout)
			defer cancel()

			err := m.client.RemoveObject(ctx, bucket, file.Path, minio.RemoveObjectOptions{
				VersionID: info.VersionID,
			})
			if err != nil {
				select {
				case m.errs <- fmt.Errorf("remove object %s: %w", file.Path, err):
				default:
				}
			}
		})
	}, nil
}

func (m *Minio) CreateReadOnlyBuckets(ctx context.Context, buckets ...string) error {
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
func (m *Minio) CreateReadOnlyBucket(ctx context.Context, bucketName string) error {
	// Check if bucket already exists
	exists, err := m.client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}

	// Create bucket if it doesn't exist
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

	// Set the bucket policy
	err = m.client.SetBucketPolicy(ctx, bucketName, readOnlyPolicy)
	if err != nil {
		return fmt.Errorf("set bucket policy: %w", err)
	}

	return nil
}

func (m *Minio) Errs() <-chan error {
	return m.errs
}

func (m *Minio) Close() {
	m.wg.Wait()
	close(m.errs)
}
