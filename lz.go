package spice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
)

func lzImage(buf []byte, palette []color.RGBA, img *image.RGBA) (image.Image, error) {
	if len(buf) < 28 {
		// too small
		return nil, errors.New("not enough data for lzImage")
	}

	// check magic
	if string(buf[:4]) != "  ZL" {
		return nil, errors.New("invalid magic for LZ image")
	}

	// 00000000  20 20 5a 4c 00 01 00 01  00 00 00 09 00 00 02 d2  |  ZL............|
	// 00000010  00 00 01 11 00 00 0b 48  00 00 00 01              |.......H....|
	// 2020/11/14 03:08:46 spice/lz: decoding image version=16777472 type=150994944 3523346432x285278208 stride=1208680448 top_down=16777216

	// followed by the following uint32 values: type, width, height, stride, top_down
	vers := binary.BigEndian.Uint32(buf[4:8])
	typ := LzImageType(binary.BigEndian.Uint32(buf[8:12]))
	width := binary.BigEndian.Uint32(buf[12:16])
	height := binary.BigEndian.Uint32(buf[16:20])
	stride := binary.BigEndian.Uint32(buf[20:24])
	top_down := binary.BigEndian.Uint32(buf[24:28])

	//log.Printf("spice/lz: decoding image version=%d type=%s %dx%d stride=%d top_down=%d", vers, typ, width, height, stride, top_down)
	_ = vers // avoid unused error when log is commented

	// build image directly so we guarantee the right stride
	if img == nil {
		img = &image.RGBA{Pix: make([]byte, stride*height), Stride: int(stride), Rect: image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{int(width), int(height)}}}
	}

	lzBuf := bytes.NewReader(buf[28:])

	switch typ {
	case LZ_IMAGE_TYPE_RGB32, LZ_IMAGE_TYPE_RGBA:
		err := lzDecompress(lzBuf, img, LZ_IMAGE_TYPE_RGB32, typ != LZ_IMAGE_TYPE_RGBA, palette, false)
		if err != nil {
			return nil, err
		}
		if typ == LZ_IMAGE_TYPE_RGBA && err == nil {
			// also read alpha layer (format XXXA)
			err = lzDecompress(lzBuf, img, LZ_IMAGE_TYPE_XXXA, false, nil, true)
			if err != nil {
				return nil, err
			}
		}
	case LZ_IMAGE_TYPE_XXXA:
		// alpha layer only
		err := lzDecompress(lzBuf, img, typ, false, nil, true)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("lz: unsupported type %s", typ)
	}

	if lzBuf.Len() > 0 {
		log.Printf("WARNING: lz didn't read the whole buffer, data is being lost")
	}

	if top_down == 0 {
		reverseImgRGBA(img)
	}

	return img, nil
}

// Functions for handling SPICE_IMAGE_TYPE_LZ_RGB
// Adapted from lz.js, itself adapted from lz.c
// See: https://gitlab.freedesktop.org/spice/spice-html5/-/blob/master/src/lz.js
// Adapted again from lib/images/lz.js function lz_rgb32_decompress:
// and modified/improved.
func lzDecompress(in io.ByteReader, out *image.RGBA, typ LzImageType, defaultAlpha bool, palette []color.RGBA, opaque bool) error {
	var op uint32 // output position
	outBuf := out.Pix
	outBufLen := uint32(len(outBuf) / 4)

	for {
		if op >= outBufLen {
			break
		}
		ctrl, err := in.ReadByte()
		if err != nil {
			return err
		}

		ref := op
		ln := uint32(ctrl >> 5)
		ofs := uint32(ctrl&0x1f) << 8
		op4 := op * 4

		//log.Printf("op=%d ctrl=0x%x len=%d ofs=0x%x", op, ctrl, ln, ofs)

		if ctrl >= 0x20 { // equivalent to if ln>0
			ln -= 1
			if ln == 7-1 { // if ln has its maximum value
				for {
					code, err := in.ReadByte()
					if err != nil {
						return err
					}
					ln += uint32(code)
					if code != 0xff {
						break
					}
				}
			}
			code, err := in.ReadByte()
			if err != nil {
				return err
			}
			ofs += uint32(code)
			//log.Printf("ofs += 0x%x", code)

			if ofs == 0x1fff { // if max value, next 2 bytes are added (big endian)
				code, err = in.ReadByte()
				if err != nil {
					return err
				}
				ofs += uint32(code) << 8
				code, err = in.ReadByte()
				if err != nil {
					return err
				}
				ofs += uint32(code)
			}
			ln += 1
			if typ == LZ_IMAGE_TYPE_XXXA || palette != nil {
				ln += 2
			}
			ofs += 1

			//log.Printf("status ln=%d ofs=%d op=%d/%d ref=%d", ln, ofs, op, outBufLen, ref)

			// CAST_PLT_DISTANCE ofs and len
			switch typ {
			case LZ_IMAGE_TYPE_PLT4_LE, LZ_IMAGE_TYPE_PLT4_BE:
				ofs *= 2
				ln *= 2
			case LZ_IMAGE_TYPE_PLT1_BE, LZ_IMAGE_TYPE_PLT1_LE:
				ofs *= 8
				ln *= 8
			}

			if ofs > ref {
				return fmt.Errorf("lz: back reference pointing to before start of data (ref(%d) -= %d)", ref, ofs)
			}
			ref -= ofs
			if ref == op-1 {
				//plt4/1 what?
				b := ref // this "b" var seems useless

				for ; ln > 0; ln -= 1 {
					op4 = op * 4
					// COPY_PIXEL
					if typ == LZ_IMAGE_TYPE_XXXA {
						if opaque {
							outBuf[op4+3] = 0xff
						} else {
							outBuf[op4+3] = outBuf[b*4+3]
						}
					} else {
						for i := uint32(0); i < 4; i += 1 {
							outBuf[op4+i] = outBuf[b*4+i]
						}
					}
					op += 1
				}
			} else {
				for ; ln > 0; ln -= 1 {
					//COPY_REF_PIXEL
					op4 = op * 4
					if typ == LZ_IMAGE_TYPE_XXXA {
						if opaque {
							outBuf[op4+3] = 0xff
						} else {
							outBuf[op4+3] = outBuf[ref*4+3]
						}
					} else {
						for i := uint32(0); i < 4; i += 1 {
							outBuf[op4+i] = outBuf[ref*4+i]
						}
					}
					op += 1
					ref += 1
				}
			}
		} else {
			//COPY_COMP_PIXEL
			ctrl += 1

			if typ == LZ_IMAGE_TYPE_XXXA {
				code, err := in.ReadByte()
				if err != nil {
					return err
				}

				if opaque {
					outBuf[op4+3] = 0xff
				} else {
					outBuf[op4+3] = code
				}
			} else if palette != nil {
				switch typ {
				case LZ_IMAGE_TYPE_PLT1_LE:
					// TODO
					return errors.New("TODO LZ_IMAGE_TYPE_PLT1_LE 1")
				case LZ_IMAGE_TYPE_PLT1_BE:
					// TODO
					return errors.New("TODO LZ_IMAGE_TYPE_PLT1_BE 1")
				case LZ_IMAGE_TYPE_PLT4_LE:
					// TODO
					return errors.New("TODO LZ_IMAGE_TYPE_PLT4_LE 1")
				case LZ_IMAGE_TYPE_PLT4_BE:
					// TODO
					return errors.New("TODO LZ_IMAGE_TYPE_PLT4_BE 1")
				case LZ_IMAGE_TYPE_PLT8:
					posPal, err := in.ReadByte()
					if err != nil {
						return err
					}
					copyPixel(op4, palette[posPal], outBuf)
					if defaultAlpha {
						outBuf[op4+3] = 0xff
					}
				}
			} else {
				code, err := in.ReadByte()
				if err != nil {
					return err
				}
				outBuf[op4+2] = code
				code, err = in.ReadByte()
				if err != nil {
					return err
				}
				outBuf[op4+1] = code
				code, err = in.ReadByte()
				if err != nil {
					return err
				}
				outBuf[op4+0] = code

				if defaultAlpha {
					outBuf[op4+3] = 0xff
				}
			}
			op += 1

			// for (--ctrl; ctrl; ctrl--) {
			for i := ctrl - 1; i > 0; i -= 1 {
				//COPY_COMP_PIXEL
				op4 = op * 4 // faster?
				if typ == LZ_IMAGE_TYPE_XXXA {
					code, err := in.ReadByte()
					if err != nil {
						return err
					}
					if opaque {
						outBuf[op4+3] = 0xff
					} else {
						outBuf[op4+3] = code
					}
				} else if palette != nil {
					switch typ {
					case LZ_IMAGE_TYPE_PLT1_LE:
						return errors.New("TODO LZ_IMAGE_TYPE_PLT1_LE 2")
					case LZ_IMAGE_TYPE_PLT1_BE:
						return errors.New("TODO LZ_IMAGE_TYPE_PLT1_BE 2")
					case LZ_IMAGE_TYPE_PLT4_LE:
						return errors.New("TODO LZ_IMAGE_TYPE_PLT4_LE 2")
					case LZ_IMAGE_TYPE_PLT4_BE:
						return errors.New("TODO LZ_IMAGE_TYPE_PLT4_BE 2")
					case LZ_IMAGE_TYPE_PLT8:
						return errors.New("TODO LZ_IMAGE_TYPE_PLT8 2")
					}
				} else {
					code, err := in.ReadByte()
					if err != nil {
						return err
					}
					outBuf[op4+2] = code
					code, err = in.ReadByte()
					if err != nil {
						return err
					}
					outBuf[op4+1] = code
					code, err = in.ReadByte()
					if err != nil {
						return err
					}
					outBuf[op4+0] = code

					if defaultAlpha {
						outBuf[op4+3] = 0xff
					}
				}
				op += 1
			}
		}
	}

	return nil
}

func copyPixel(op4 uint32, col color.RGBA, outBuf []byte) {
	outBuf[op4+0] = col.R
	outBuf[op4+1] = col.G
	outBuf[op4+2] = col.B
}
