package service

import (
	"context"
	"fmt"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/nakamauwu/nakama/minio"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
	"golang.org/x/image/webp"
	"golang.org/x/sync/errgroup"
)

const MediaBucket = "media"

const MaxMediaItemBytes = 5 << 20 // 5MB

const ErrMediaItemTooLarge = errs.InvalidArgumentError("media item too large")

var imageContentTypeToExtension = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

func (svc *Service) storeMedia(ctx context.Context, media []types.Media) (func(ctx context.Context) error, error) {
	if len(media) == 0 {
		return func(context.Context) error { return nil }, nil
	}

	var uploadItems []minio.Upload
	for _, m := range media {
		uploadItems = append(uploadItems, minio.Upload{
			Key:         m.Path,
			Reader:      m.Reader(),
			ContentType: m.ContentType,
			FileSize:    -1,
		})
	}

	return svc.MinioStore.UploadMany(ctx, MediaBucket, uploadItems)
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

			width, height, err := decodeMedia(r, ct)
			if err != nil {
				return err
			}

			_, err = r.Seek(0, io.SeekStart)
			if err != nil {
				return fmt.Errorf("seek media item to start: %w", err)
			}

			name, err := gonanoid.New()
			if err != nil {
				return fmt.Errorf("generate media item name: %w", err)
			}

			name += contentTypeExtension(ct)

			mu.Lock()
			media := types.Media{
				Path:        name,
				Width:       width,
				Height:      height,
				ContentType: ct,
			}
			media.SetReader(r)
			out[i] = media
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return out, nil
}

func isMediaContentTypeSupported(ct string) bool {
	_, ok := imageContentTypeToExtension[ct]
	return ok
}

func decodeMedia(r io.Reader, ct string) (uint, uint, error) {
	r = io.LimitReader(r, MaxMediaItemBytes+1)

	switch ct {
	case "image/jpeg":
		img, err := jpeg.Decode(r)
		if err != nil {
			return 0, 0, errs.InvalidArgumentError("invalid media item format")
		}
		return uint(img.Bounds().Dx()), uint(img.Bounds().Dy()), nil
	case "image/png":
		img, err := png.Decode(r)
		if err != nil {
			return 0, 0, errs.InvalidArgumentError("invalid media item format")
		}
		return uint(img.Bounds().Dx()), uint(img.Bounds().Dy()), nil
	case "image/gif":
		img, err := gif.DecodeAll(r)
		if err != nil {
			return 0, 0, errs.InvalidArgumentError("invalid media item format")
		}
		return uint(img.Config.Width), uint(img.Config.Height), nil
	case "image/webp":
		img, err := webp.Decode(r)
		if err != nil {
			return 0, 0, errs.InvalidArgumentError("invalid media item format")
		}
		return uint(img.Bounds().Dx()), uint(img.Bounds().Dy()), nil
	default:
		return 0, 0, errs.InvalidArgumentError("unsupported media item format")
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

func (svc *Service) objectStoreURL(bucket, key string) string {
	base := strings.TrimRight(svc.ObjectsBaseURL, "/")
	bucket = strings.Trim(bucket, "/")
	key = strings.TrimLeft(key, "/")

	return fmt.Sprintf("%s/%s/%s", base, bucket, key)
}
