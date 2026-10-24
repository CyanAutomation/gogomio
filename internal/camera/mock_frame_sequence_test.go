package camera

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func writeMockFrameJPEG(t *testing.T, dir, name string, width, height int, fill color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var data bytes.Buffer
	if err := jpeg.Encode(&data, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("encode fixture %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data.Bytes(), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func TestLoadJPEGFrameSequenceSortsValidatesAndEncodesConfiguredFrames(t *testing.T) {
	dir := t.TempDir()
	writeMockFrameJPEG(t, dir, "02-blue.jpg", 8, 6, color.RGBA{R: 10, G: 20, B: 230, A: 255})
	writeMockFrameJPEG(t, dir, "01-red.jpeg", 8, 6, color.RGBA{R: 230, G: 20, B: 10, A: 255})
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}

	frames, err := LoadJPEGFrameSequence(dir, 8, 6, 60)
	if err != nil {
		t.Fatalf("LoadJPEGFrameSequence failed: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("loaded %d frames, want 2", len(frames))
	}
	for i, frame := range frames {
		decoded, err := jpeg.Decode(bytes.NewReader(frame))
		if err != nil {
			t.Fatalf("frame %d is not a JPEG: %v", i, err)
		}
		if got := decoded.Bounds().Size(); got != image.Pt(8, 6) {
			t.Fatalf("frame %d dimensions = %v, want (8,6)", i, got)
		}
	}
	first := color.RGBAModel.Convert(mustDecodeMockFrame(t, frames[0]).At(4, 3)).(color.RGBA)
	second := color.RGBAModel.Convert(mustDecodeMockFrame(t, frames[1]).At(4, 3)).(color.RGBA)
	if first.R <= first.B || second.B <= second.R {
		t.Fatalf("frames are not in filename order (red then blue): first=%v second=%v", first, second)
	}
}

func TestLoadJPEGFrameSequenceRejectsInvalidAndMismatchedJPEGs(t *testing.T) {
	t.Run("invalid jpeg", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "broken.jpg"), []byte("not a JPEG"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadJPEGFrameSequence(dir, 8, 6, 80); err == nil {
			t.Fatal("LoadJPEGFrameSequence accepted an invalid JPEG")
		}
	})

	t.Run("wrong dimensions", func(t *testing.T) {
		dir := t.TempDir()
		writeMockFrameJPEG(t, dir, "wrong-size.jpg", 10, 6, color.RGBA{R: 20, G: 40, B: 60, A: 255})
		if _, err := LoadJPEGFrameSequence(dir, 8, 6, 80); err == nil {
			t.Fatal("LoadJPEGFrameSequence accepted a mismatched frame size")
		}
	})

	t.Run("empty sequence", func(t *testing.T) {
		if _, err := LoadJPEGFrameSequence(t.TempDir(), 8, 6, 80); err == nil {
			t.Fatal("LoadJPEGFrameSequence accepted a directory without JPEG frames")
		}
	})
}

func mustDecodeMockFrame(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode JPEG fixture: %v", err)
	}
	return img
}
