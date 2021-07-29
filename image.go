package spice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"

	"github.com/Shells-com/spice/quic"
)

type Image struct {
	image.Image

	ID     uint64
	Type   uint8 // 0=bitmap 1=quic 2=reserved 100=lz_plt 101=lz_rgb 102=glz_rgb 103=from_cache 104=surface 105=jpeg 106=from_cache_lossless 107=ZLIB_GLZ_RGB 108=jpeg_alpha 109=lz4
	Flags  uint8 // 1=cache_me 2=high_bits_set 4=cache_replace_me
	Width  uint32
	Height uint32
}

func DecodeImage(buf []byte) (*Image, error) {
	i := &Image{}

	if len(buf) < 18 {
		return nil, io.ErrUnexpectedEOF
	}

	// read header (18 bytes)
	i.ID = binary.LittleEndian.Uint64(buf[:8])
	i.Type = buf[8]
	i.Flags = buf[9]
	i.Width = binary.LittleEndian.Uint32(buf[10:14])
	i.Height = binary.LittleEndian.Uint32(buf[14:18])

	//log.Printf("decode spice img %dx%d id=%d typ=%d flags=%d", i.Width, i.Height, i.ID, i.Type, i.Flags)

	buf = buf[18:]

	switch i.Type {
	case 0: // bitmap
		img, err := bitmapImage(buf)
		if err != nil {
			return nil, err
		}
		i.Image = img
		return i, nil
	case 1: // quic
		if len(buf) < 4 {
			// invalid
			return nil, errors.New("invalid data for image")
		}
		ln := binary.LittleEndian.Uint32(buf[:4])
		if len(buf) < 4+int(ln) {
			return nil, errors.New("data is missing")
		}

		img_buf := buf[4 : ln+4]

		img, err := quic.Decode(bytes.NewReader(img_buf))
		if err != nil {
			return nil, err
		}
		i.Image = img
		return i, nil
	case 100: // lz_plt
		// Flags: LZPALETTE_FLAG_PAL_FROM_CACHE, LZPALETTE_FLAG_PAL_CACHE_ME, etc
		//var palette []color.RGBA
		return nil, fmt.Errorf("todo palette lz_plt")
	case 101: // lz_rgb
		if len(buf) < 4 {
			// invalid
			return nil, errors.New("invalid data for image")
		}
		ln := binary.LittleEndian.Uint32(buf[:4])
		if len(buf) < 4+int(ln) {
			return nil, errors.New("data is missing")
		}

		img_buf := buf[4 : ln+4]

		img, err := lzImage(img_buf, nil, nil)
		if err != nil {
			return nil, err
		}
		i.Image = img

		// XXX let's save the image
		/*
			fn := "img_" + time.Now().Format("20060102_150405.000000000") + ".png"
			if f, err := os.Create(fn); err == nil {
				png.Encode(f, img)
				f.Close()
			}
			// */
		return i, nil
	case 105: // 105=jpeg
		if len(buf) < 4 {
			// invalid
			return nil, errors.New("invalid data for image")
		}
		ln := binary.LittleEndian.Uint32(buf[:4])
		if len(buf) < 4+int(ln) {
			return nil, errors.New("data is missing")
		}

		img_buf := buf[4 : ln+4]

		// decode jpeg
		img, err := jpeg.Decode(bytes.NewReader(img_buf))
		if err != nil {
			return nil, fmt.Errorf("failed to decode jpeg: %w", err)
		}

		i.Image = img
		return i, nil
	case 108: // 108=jpeg_alpha
		if len(buf) < 9 {
			return nil, errors.New("not enough jpeg_alpha data")
		}
		jpegFlag := buf[0]
		jpegSize := binary.LittleEndian.Uint32(buf[1:5])
		dataSize := binary.LittleEndian.Uint32(buf[5:9])

		buf = buf[9:]

		if uint32(len(buf)) < dataSize || jpegSize > dataSize {
			return nil, errors.New("not enough data to decode jpeg")
		}

		// decode jpeg
		img, err := jpeg.Decode(bytes.NewReader(buf[:jpegSize]))
		if err != nil {
			return nil, fmt.Errorf("failed to decode jpeg: %w", err)
		}

		buf = buf[jpegSize:]

		// apply alpha channel, first convert jpeg to rgba
		rgba := image.NewRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Over)

		// alpha is encoded using lz image format XXXA. Passing rgba image will apply the channel info directly
		_, err = lzImage(buf, nil, rgba)
		if err != nil {
			return nil, err
		}
		if jpegFlag&1 == 0 {
			// need to reverse
			reverseImgRGBA(rgba)
		}

		i.Image = rgba

		return i, nil
	default:
		return nil, fmt.Errorf("unsupported image type %d", i.Type)
	}

	return i, nil
}

func reverseImgRGBA(img *image.RGBA) {
	height := img.Bounds().Dy()
	stride := img.Stride
	newPix := make([]uint8, len(img.Pix))
	for i := 0; i < height; i++ {
		j := height - i - 1
		copy(newPix[stride*i:stride*(i+1)], img.Pix[stride*j:stride*(j+1)])
	}
	img.Pix = newPix
}
