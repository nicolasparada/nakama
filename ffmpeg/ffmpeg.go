package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
)

// ContentTypeToExtension maps content types to file extensions
var ContentTypeToExtension = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
	"image/avif": "avif",
}

// Stream represents a media stream from ffprobe output
type Stream struct {
	Width     uint32 `json:"width"`
	Height    uint32 `json:"height"`
	CodecName string `json:"codec_name"`
	CodecType string `json:"codec_type"`
	NbFrames  string `json:"nb_frames"` // Number of frames (may be "N/A")
}

// Format represents format information from ffprobe output
type Format struct {
	FormatName string `json:"format_name"`
}

// ProbeResult represents the complete ffprobe output
type ProbeResult struct {
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}

type Image struct {
	File        io.ReadSeekCloser
	Width       uint32
	Height      uint32
	ContentType string
	FileSize    uint64
}

func ResizeImages(ctx context.Context, maxRes uint32, images []io.ReadSeeker) ([]Image, error) {
	out := make([]Image, len(images))

	g, gctx := errgroup.WithContext(ctx)

	for i, img := range images {
		g.Go(func() error {
			resized, err := ResizeImage(gctx, maxRes, img)
			if err != nil {
				return fmt.Errorf("resize image %d: %w", i, err)
			}
			out[i] = resized
			return nil
		})
	}

	return out, g.Wait()
}

func ResizeImage(ctx context.Context, maxRes uint32, readSeeker io.ReadSeeker) (Image, error) {
	var out Image

	// Create temporary input file
	tmpInput, err := os.CreateTemp("", "ffmpeg_input_*")
	if err != nil {
		return out, fmt.Errorf("create temp input file: %w", err)
	}
	defer os.Remove(tmpInput.Name())
	defer tmpInput.Close()

	// Copy input to temp file
	if _, err := io.Copy(tmpInput, readSeeker); err != nil {
		return out, fmt.Errorf("write temp input file: %w", err)
	}

	// Detect content type and dimensions
	contentType, width, height, err := detectImageInfo(ctx, tmpInput.Name())
	if err != nil {
		return out, fmt.Errorf("detect image info: %w", err)
	}

	// Check if resizing is needed
	if width <= maxRes && height <= maxRes {
		// No processing needed for static images that don't need resizing
		if _, err := readSeeker.Seek(0, io.SeekStart); err != nil {
			return out, fmt.Errorf("seek to start: %w", err)
		}

		// Get original file size
		fileSize, err := getFileSize(readSeeker)
		if err != nil {
			return out, fmt.Errorf("get original file size: %w", err)
		}

		return Image{
			File:        &noopCloser{readSeeker},
			Width:       width,
			Height:      height,
			ContentType: contentType,
			FileSize:    fileSize,
		}, nil
	}

	// Check if image is animated
	isAnimated, err := isAnimatedImage(ctx, tmpInput.Name())
	if err != nil {
		return out, fmt.Errorf("detect animation: %w", err)
	}

	// Always resize and encode as AVIF (handles both static and animated)
	processedFile, newWidth, newHeight, fileSize, err := resizeToAVIF(ctx, tmpInput.Name(), maxRes, isAnimated)
	if err != nil {
		return out, fmt.Errorf("resize to AVIF: %w", err)
	}

	return Image{
		File:        processedFile,
		Width:       newWidth,
		Height:      newHeight,
		ContentType: "image/avif",
		FileSize:    fileSize,
	}, nil
}

func detectImageInfo(ctx context.Context, filePath string) (contentType string, width, height uint32, err error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		filePath)

	output, err := cmd.Output()
	if err != nil {
		return "", 0, 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probe ProbeResult
	if err := json.Unmarshal(output, &probe); err != nil {
		return "", 0, 0, fmt.Errorf("parse ffprobe output: %w", err)
	}

	if len(probe.Streams) == 0 {
		return "", 0, 0, fmt.Errorf("no streams found")
	}

	// Find the video stream (images are video streams with 1 frame)
	var stream *Stream
	for i := range probe.Streams {
		if probe.Streams[i].CodecType == "video" {
			stream = &probe.Streams[i]
			break
		}
	}

	if stream == nil {
		return "", 0, 0, fmt.Errorf("no video stream found")
	}

	if stream.Width == 0 || stream.Height == 0 {
		return "", 0, 0, fmt.Errorf("invalid dimensions: %dx%d", stream.Width, stream.Height)
	}

	// Map format/codec to content type
	contentType = formatToContentType(probe.Format.FormatName, stream.CodecName)
	if contentType == "" {
		return "", 0, 0, fmt.Errorf("unsupported image format: %s/%s", probe.Format.FormatName, stream.CodecName)
	}

	return contentType, stream.Width, stream.Height, nil
}

// isAnimatedImage detects if the image has multiple frames (animated)
func isAnimatedImage(ctx context.Context, filePath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-select_streams", "v:0",
		"-count_frames",
		"-show_entries", "stream=nb_frames",
		"-of", "csv=p=0",
		filePath)

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("ffprobe frame count failed: %w", err)
	}

	frameCountStr := strings.TrimSpace(string(output))
	if frameCountStr == "N/A" || frameCountStr == "" {
		// For formats like GIF, try alternative approach
		return isAnimatedByFormat(ctx, filePath)
	}

	frameCount, err := strconv.Atoi(frameCountStr)
	if err != nil {
		return false, fmt.Errorf("parse frame count: %w", err)
	}

	return frameCount > 1, nil
}

// isAnimatedByFormat detects animation for specific formats where frame counting might not work
func isAnimatedByFormat(ctx context.Context, filePath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath)

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("ffprobe format check failed: %w", err)
	}

	var probe ProbeResult
	if err := json.Unmarshal(output, &probe); err != nil {
		return false, fmt.Errorf("parse ffprobe output: %w", err)
	}

	formatName := probe.Format.FormatName

	// For GIF, check if duration indicates animation
	if strings.Contains(formatName, "gif") {
		// GIFs with duration > 0 are likely animated
		cmd = exec.CommandContext(ctx, "ffprobe",
			"-v", "quiet",
			"-show_entries", "format=duration",
			"-of", "csv=p=0",
			filePath)

		durationOutput, err := cmd.Output()
		if err == nil {
			durationStr := strings.TrimSpace(string(durationOutput))
			if duration, err := strconv.ParseFloat(durationStr, 64); err == nil && duration > 0 {
				return true, nil
			}
		}
	}

	return false, nil
}

func formatToContentType(formatName, codecName string) string {
	// Only accept formats that work in <img> elements
	switch {
	case strings.Contains(formatName, "jpeg") || codecName == "mjpeg":
		return "image/jpeg"
	case strings.Contains(formatName, "png") || codecName == "png":
		return "image/png"
	case strings.Contains(formatName, "gif") || codecName == "gif":
		return "image/gif"
	case strings.Contains(formatName, "webp") || codecName == "webp":
		return "image/webp"
	case (strings.Contains(formatName, "avif") || codecName == "av1"):
		return "image/avif"
	default:
		return "" // Unsupported
	}
}

func resizeToAVIF(ctx context.Context, inputPath string, maxRes uint32, isAnimated bool) (io.ReadSeekCloser, uint32, uint32, uint64, error) {
	tmpOutput, err := os.CreateTemp("", "ffmpeg_output_*.avif")
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("create temp output file: %w", err)
	}
	defer os.Remove(tmpOutput.Name())
	defer tmpOutput.Close()

	// Smart CPU usage based on image size and available time
	cpuUsed := getCPUUsedSetting(maxRes)
	crf := getCRFSetting(maxRes)

	// Determine if input has alpha to choose proper pixel format
	hasAlpha, _ := hasAlphaPixelFormat(ctx, inputPath)
	// Build filter chain: scale to fit, ensure even dims for 4:2:0, set pixel format
	// Enforce even dimensions because AV1 4:2:0 requires even width/height.
	scaleFit := fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease", maxRes, maxRes)
	scaleEven := "scale=ceil(iw/2)*2:ceil(ih/2)*2"
	pixFmt := "yuv420p"
	if hasAlpha {
		pixFmt = "yuva420p"
	}
	vf := fmt.Sprintf("%s,%s,format=%s", scaleFit, scaleEven, pixFmt)

	var cmd *exec.Cmd

	if isAnimated {
		// For animated AVIF - do not use still-picture mode
		cmd = exec.CommandContext(ctx, "ffmpeg",
			"-hide_banner", "-nostdin", "-loglevel", "error",
			"-i", inputPath,
			"-vf", vf,
			"-map", "0:v:0",
			"-an",
			"-c:v", "libaom-av1",
			"-crf", fmt.Sprintf("%d", crf),
			"-cpu-used", fmt.Sprintf("%d", cpuUsed),
			"-row-mt", "1",
			"-tiles", "2x2",
			"-y",
			tmpOutput.Name())
	} else {
		// For static AVIF - use still-picture mode to prevent flickering
		cmd = exec.CommandContext(ctx, "ffmpeg",
			"-hide_banner", "-nostdin", "-loglevel", "error",
			"-i", inputPath,
			"-vf", vf,
			"-map", "0:v:0",
			"-an",
			"-c:v", "libaom-av1",
			"-crf", fmt.Sprintf("%d", crf),
			"-cpu-used", fmt.Sprintf("%d", cpuUsed),
			"-row-mt", "1",
			"-tiles", "2x2",
			"-still-picture", "1",
			"-frames:v", "1",
			"-y",
			tmpOutput.Name())
	}

	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, 0, 0, 0, fmt.Errorf("ffmpeg resize failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	processedFile, err := os.Open(tmpOutput.Name())
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("open processed file: %w", err)
	}

	// Get final dimensions
	_, width, height, err := detectImageInfo(ctx, tmpOutput.Name())
	if err != nil {
		processedFile.Close()
		return nil, 0, 0, 0, fmt.Errorf("get final dimensions: %w", err)
	}

	// Get file size
	fileSize, err := getFileSize(processedFile)
	if err != nil {
		processedFile.Close()
		return nil, 0, 0, 0, fmt.Errorf("get processed file size: %w", err)
	}

	return processedFile, width, height, fileSize, nil
}

// hasAlphaPixelFormat inspects the input's pixel format to guess if it contains an alpha channel.
func hasAlphaPixelFormat(ctx context.Context, filePath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=pix_fmt",
		"-of", "default=nw=1:nk=1",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("ffprobe pix_fmt failed: %w", err)
	}
	pf := strings.TrimSpace(string(out))
	if pf == "" {
		return false, nil
	}
	// Heuristic: alpha formats usually contain 'a'
	return strings.Contains(pf, "a"), nil
}

func getCPUUsedSetting(maxRes uint32) int {
	switch {
	case maxRes <= 512:
		return 4 // Small images - can afford better quality
	case maxRes <= 1024:
		return 5 // Medium images - balanced
	case maxRes <= 2000:
		return 6 // Large images - prioritize speed
	default:
		return 7 // Very large - fastest encoding
	}
}

func getCRFSetting(maxRes uint32) int {
	switch {
	case maxRes <= 512:
		return 28 // Small images - better quality
	case maxRes <= 1024:
		return 30 // Medium images - balanced
	case maxRes <= 2000:
		return 32 // Large images - current setting
	default:
		return 35 // Very large - more compression
	}
}

func getFileSize(file io.ReadSeeker) (uint64, error) {
	// Seek to end to get file size
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("seek to end: %w", err)
	}

	// Seek back to start for future reads
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek to start: %w", err)
	}

	return uint64(size), nil
}

type noopCloser struct {
	io.ReadSeeker
}

func (noopCloser) Close() error { return nil }
