package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"sync"

	"github.com/go-kit/log/level"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/nakamauwu/nakama/storage"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
	"golang.org/x/image/webp"
	"golang.org/x/sync/errgroup"
)

const MediaBucket = "media"

const (
	MaxMediaItemBytes = 5 << 20  // 5MB
	MaxMediaBytes     = 15 << 20 // 15MB
)

const ErrMediaItemTooLarge = errs.InvalidArgumentError("media item too large")

var imageContentTypeToExtension = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

func (svc *Service) storeMedia(ctx context.Context, media []types.Media) error {
	if len(media) == 0 {
		return nil
	}
	g, gctx := errgroup.WithContext(ctx)
	for _, mediaItem := range media {
		g.Go(func() error {
			err := svc.Store.Store(gctx, MediaBucket, mediaItem.Name, mediaItem.Contents, storage.StoreWithContentType(mediaItem.ContentType))
			if err != nil {
				return fmt.Errorf("store media item: %w", err)
			}
			return nil
		})
	}

	return g.Wait()
}

func processMedia(readers []io.ReadSeeker) ([]types.Media, error) {
	if len(readers) == 0 {
		return nil, nil
	}

	out := make([]types.Media, len(readers))
	var mu sync.Mutex

	var g errgroup.Group

	for i, r := range readers {
		g.Go(func() error {
			ct, err := detectContentType(r)
			if err != nil {
				return err
			}

			if !isMediaContentTypeSupported(ct) {
				return errs.InvalidArgumentError("unsupported media item format")
			}

			contents, err := io.ReadAll(io.LimitReader(r, MaxMediaItemBytes+1))
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return ErrMediaItemTooLarge
			}

			if err != nil {
				return fmt.Errorf("read media item contents: %w", err)
			}

			if err := validateImage(bytes.NewReader(contents), ct); err != nil {
				return err
			}

			name, err := gonanoid.New()
			if err != nil {
				return fmt.Errorf("generate media item name: %w", err)
			}

			name += contentTypeExtension(ct)

			mu.Lock()
			out[i] = types.Media{
				Name:        name,
				ContentType: ct,
				Contents:    contents,
			}
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return out, nil
}

func (svc *Service) cleanupMedia(media []types.Media) {
	if len(media) == 0 {
		return
	}

	g, gctx := errgroup.WithContext(context.Background())
	for _, mediaItem := range media {
		g.Go(func() error {
			err := svc.Store.Delete(gctx, MediaBucket, mediaItem.Name)
			if err != nil {
				return fmt.Errorf("delete media item: %w", err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		_ = level.Error(svc.Logger).Log("msg", "could not cleanup media", "err", err)
	}
}

func collectMediaNames(media []types.Media) []string {
	if len(media) == 0 {
		return nil
	}

	names := make([]string, len(media))
	for i, m := range media {
		names[i] = m.Name
	}
	return names
}

func mediaTotalBytes(media []types.Media) int64 {
	var total int64
	for _, m := range media {
		total += int64(len(m.Contents))
	}
	return total
}

func isMediaContentTypeSupported(ct string) bool {
	_, ok := imageContentTypeToExtension[ct]
	return ok
}

func validateImage(r io.Reader, ct string) error {
	switch ct {
	case "image/jpeg":
		_, err := jpeg.Decode(r)
		if err != nil {
			return errs.InvalidArgumentError("invalid media item format")
		}
		return nil
	case "image/png":
		_, err := png.Decode(r)
		if err != nil {
			return errs.InvalidArgumentError("invalid media item format")
		}
		return nil
	case "image/gif":
		_, err := gif.DecodeAll(r)
		if err != nil {
			return errs.InvalidArgumentError("invalid media item format")
		}
		return nil
	case "image/webp":
		_, err := webp.Decode(r)
		if err != nil {
			return errs.InvalidArgumentError("invalid media item format")
		}
		return nil
	default:
		return errs.InvalidArgumentError("unsupported media item format")
	}
}

func contentTypeExtension(ct string) string {
	ext, ok := imageContentTypeToExtension[ct]
	if !ok {
		return ".bin"
	}
	return ext
}

func detectContentType(r io.ReadSeeker) (string, error) {
	// http.DetectContentType uses at most 512 bytes to make its decision.
	h := make([]byte, 512)
	_, err := r.Read(h)
	if err != nil {
		return "", fmt.Errorf("detect content type: read head: %w", err)
	}

	// Reset the reader so it can be used again.
	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("detect content type: seek to start: %w", err)
	}

	mt, _, err := mime.ParseMediaType(http.DetectContentType(h))
	if err != nil {
		return "", fmt.Errorf("detect content type: parsing media type: %w", err)
	}

	return mt, nil
}
