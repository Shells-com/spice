package quic

import (
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuic(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	for _, fn := range []string{"test1"} { // , "test2", "test3", "test4"} {
		in := filepath.Join(wd, "testdata", fn)
		f, err := os.Open(in + ".quic")
		require.NoError(t, err)

		img, err := Decode(f)
		endpos, _ := f.Seek(0, io.SeekCurrent)
		st, _ := f.Stat()
		f.Close()

		assert.NoError(t, err, "quicImage()")

		if err == nil {
			assert.EqualValues(t, st.Size(), endpos, "final read position should be EOF")

			// open master file
			f, err = os.Open(in + ".png")
			require.NoError(t, err)
			raw, _, err := image.Decode(f)
			f.Close()
			require.NoError(t, err)

			dat := pixelsForImage(t, img)
			master := pixelsForImage(t, raw)

			if !assert.Equal(t, master, dat, "images equal") {
				writeImage(in+"_failed.png", img)
			}
		}
	}
}

func pixelsForImage(t *testing.T, img image.Image) []uint8 {
	switch data := img.(type) {
	case *image.RGBA:
		return data.Pix
	case *image.NRGBA:
		return data.Pix
	default:
		t.Error("Master image is unsupported type")
		return nil
	}
}

func writeImage(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
