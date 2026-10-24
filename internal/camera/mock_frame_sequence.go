package camera

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
)

// LoadJPEGFrameSequence loads JPEG frames from a directory in lexical filename
// order. Frames must match the configured output size and are re-encoded at the
// configured quality so fixture mode follows the normal mock camera contract.
func LoadJPEGFrameSequence(dir string, width, height, quality int) ([][]byte, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("mock frame directory must not be empty")
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid mock frame resolution: %dx%d", width, height)
	}
	if quality < 1 || quality > 100 {
		return nil, fmt.Errorf("invalid mock frame JPEG quality: %d", quality)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read mock frame directory %q: %w", dir, err)
	}

	frames := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".jpg" && extension != ".jpeg" {
			continue
		}

		filename := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read mock frame %q: %w", filename, err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode mock frame %q as JPEG: %w", filename, err)
		}
		bounds := img.Bounds()
		if bounds.Dx() != width || bounds.Dy() != height {
			return nil, fmt.Errorf("mock frame %q has size %dx%d; configured output is %dx%d", filename, bounds.Dx(), bounds.Dy(), width, height)
		}

		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode mock frame %q: %w", filename, err)
		}
		frames = append(frames, encoded.Bytes())
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("mock frame directory %q contains no .jpg or .jpeg files", dir)
	}
	return frames, nil
}
