package spice

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
)

func bitmapImage(data []byte) (image.Image, error) {
	if len(data) < 2 {
		return nil, errors.New("not enough data for bitmap image")
	}

	format := BitmapImageType(data[0])
	flags := data[1] // 1=PAL_CACHE_ME, 2=PAL_FROM_CACHE, 4=TOP_DOWN,

	var headerLen int
	if flags&2 == 2 {
		headerLen = 22
	} else {
		headerLen = 18
	}

	if len(data) < headerLen {
		return nil, errors.New("not enough data for bitmap image")
	}

	width := binary.LittleEndian.Uint32(data[2:6])
	height := binary.LittleEndian.Uint32(data[6:10])
	stride := binary.LittleEndian.Uint32(data[10:14])

	var palId uint64
	var palPtr uint32
	if flags&2 == 2 { // PAL_FROM_CACHE
		// palette_id
		palId = binary.LittleEndian.Uint64(data[14:22])
	} else {
		// ptr to palette
		palPtr = binary.LittleEndian.Uint32(data[14:18])
	}

	data = data[headerLen:]

	//log.Printf("bitmap image, size=%dx%d stride=%d flags=%d format=%d palId=%d palPtr=%d len=%d", width, height, stride, flags, format, palId, palPtr, len(data))
	_, _ = palId, palPtr

	// TODO fix BITMAP_IMAGE_TYPE_32BIT
	switch format {
	case BITMAP_IMAGE_TYPE_32BIT, BITMAP_IMAGE_TYPE_RGBA:
		// easy enough
		ln := int(height * stride)
		if len(data) < ln {
			return nil, errors.New("not enough data for image")
		}
		img := &image.RGBA{Pix: data[:ln], Stride: int(stride), Rect: image.Rect(0, 0, int(width), int(height))}

		if flags&4 == 0 {
			// reverse image
			reverseImgRGBA(img)
		}
		return img, nil
	default:
		return nil, fmt.Errorf("unsupported bitmap image format=%d size=%d,%d stride=%d", format, width, height, stride)
	}
}
