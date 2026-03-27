package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"strings"

	"github.com/nakamauwu/nakama/minio"
)

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

type cleanupRef struct {
	filePath string
	cleanup  func(ctx context.Context) error
}

func (s *Store) UploadMany(ctx context.Context, bucket string, files []minio.Upload) (func(ctx context.Context) error, error) {
	if len(files) == 0 {
		return func(context.Context) error { return nil }, nil
	}

	cleanups := make([]cleanupRef, 0, len(files))

	for _, file := range files {
		cleanup, err := s.Upload(ctx, bucket, file)
		if err != nil {
			cleanupErr := joinCleanupErrors(context.WithoutCancel(ctx), cleanups)
			if cleanupErr != nil {
				return nil, errors.Join(
					fmt.Errorf("upload %q: %w", file.Key, err),
					fmt.Errorf("cleanup after upload failure: %w", cleanupErr),
				)
			}

			return nil, fmt.Errorf("upload %q: %w", file.Key, err)
		}

		cleanups = append(cleanups, cleanupRef{
			filePath: file.Key,
			cleanup:  cleanup,
		})
	}

	cleanup := func(ctx context.Context) error {
		return joinCleanupErrors(ctx, cleanups)
	}

	return cleanup, nil
}

func (s *Store) Upload(ctx context.Context, bucket string, in minio.Upload) (func(ctx context.Context) error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	objectName, err := s.objectName(bucket, in.Key)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("open store root: %w", err)
	}
	defer root.Close()

	if err := root.MkdirAll(path.Dir(objectName), 0o755); err != nil {
		return nil, fmt.Errorf("create object directory: %w", err)
	}

	file, err := root.OpenFile(objectName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create object file: %w", err)
	}
	removeObject := true
	defer func() {
		if removeObject {
			_ = root.Remove(objectName)
		}
	}()

	if _, err := copyObject(file, in.Reader, in.FileSize); err != nil {
		_ = file.Close()
		return nil, err
	}

	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close object file: %w", err)
	}

	removeObject = false

	return func(ctx context.Context) error {
		return s.Delete(ctx, bucket, in.Key)
	}, nil
}

func (s *Store) Delete(ctx context.Context, bucket string, objectName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	objectName, err := s.objectName(bucket, objectName)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(s.root)
	if err != nil {
		return fmt.Errorf("open store root: %w", err)
	}
	defer root.Close()

	err = root.Remove(objectName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove object: %w", err)
	}

	return nil
}

func (s *Store) objectName(bucket, objectPath string) (string, error) {
	if s.root == "" {
		return "", errors.New("store root is required")
	}

	if err := validateBucket(bucket); err != nil {
		return "", err
	}

	if objectPath == "." || !iofs.ValidPath(objectPath) {
		return "", fmt.Errorf("object path %q is invalid", objectPath)
	}

	return path.Join(bucket, objectPath), nil
}

func validateBucket(bucket string) error {
	if bucket == "" {
		return errors.New("bucket is required")
	}

	if bucket == "." || strings.ContainsAny(bucket, `/\\`) || !iofs.ValidPath(bucket) {
		return fmt.Errorf("bucket %q is invalid", bucket)
	}

	return nil
}

func joinCleanupErrors(ctx context.Context, cleanups []cleanupRef) error {
	var errs error

	for _, ref := range cleanups {
		if err := ref.cleanup(ctx); err != nil {
			errs = errors.Join(errs, fmt.Errorf("cleanup %q: %w", ref.filePath, err))
		}
	}

	return errs
}

func copyObject(dst io.Writer, src io.Reader, size int64) (int64, error) {
	if size >= 0 {
		n, err := io.CopyN(dst, src, size)
		if err != nil {
			return n, fmt.Errorf("write object: %w", err)
		}

		return n, nil
	}

	n, err := io.Copy(dst, src)
	if err != nil {
		return n, fmt.Errorf("write object: %w", err)
	}

	return n, nil
}
