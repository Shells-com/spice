package quic

import "fmt"

type quicImageType uint32

const (
	QUIC_IMAGE_TYPE_INVALID quicImageType = iota
	QUIC_IMAGE_TYPE_GRAY
	QUIC_IMAGE_TYPE_RGB16
	QUIC_IMAGE_TYPE_RGB24
	QUIC_IMAGE_TYPE_RGB32
	QUIC_IMAGE_TYPE_RGBA
)

func (t quicImageType) bpc() uint32 {
	switch t {
	case QUIC_IMAGE_TYPE_GRAY:
		return 8
	case QUIC_IMAGE_TYPE_RGB16:
		return 5
	case QUIC_IMAGE_TYPE_RGB24:
		return 8
	case QUIC_IMAGE_TYPE_RGB32:
		return 8
	case QUIC_IMAGE_TYPE_RGBA:
		return 8
	default:
		// invalid
		return 0
	}
}

func (t quicImageType) String() string {
	switch t {
	case QUIC_IMAGE_TYPE_INVALID:
		return "INVALID"
	case QUIC_IMAGE_TYPE_GRAY:
		return "GRAY"
	case QUIC_IMAGE_TYPE_RGB16:
		return "RGB16"
	case QUIC_IMAGE_TYPE_RGB24:
		return "RGB24"
	case QUIC_IMAGE_TYPE_RGB32:
		return "RGB32"
	case QUIC_IMAGE_TYPE_RGBA:
		return "RGBA"
	default:
		return fmt.Sprintf("quicImageType(%d)", t)
	}
}

func (t quicImageType) Stride() int {
	switch t {
	case QUIC_IMAGE_TYPE_INVALID:
		return -1
	case QUIC_IMAGE_TYPE_GRAY:
		return 2
	case QUIC_IMAGE_TYPE_RGB16:
		return 2
	case QUIC_IMAGE_TYPE_RGB24:
		return 4
	case QUIC_IMAGE_TYPE_RGB32:
		return 4
	case QUIC_IMAGE_TYPE_RGBA:
		return 4
	default:
		return -1
	}
}
