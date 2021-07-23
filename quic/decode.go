package quic

import (
	"errors"
	"fmt"
	"image"
	"io"
	"log"
)

const (
	wmimax           = 6
	rgb32_pixel_pad  = 3
	rgb32_pixel_r    = 0
	rgb32_pixel_g    = 1
	rgb32_pixel_b    = 2
	rgb32_pixel_size = 4
)

type internal struct {
	vers   uint32
	typ    quicImageType
	width  uint32
	height uint32
	nChan  uint32 // number of channels in source image

	// read buffer stuff
	io_word           uint32
	io_next_word      uint32
	io_available_bits int
	rb                io.Reader

	rows_completed uint32

	rgb_state *quicState
	model     *quicModel
	channels  []*quicChannel
}

func (ctx *internal) readWord() error {
	// read one little endian uint32 value from file
	var word [4]byte
	_, err := io.ReadFull(ctx.rb, word[:])
	if err != nil {
		return err
	}
	ctx.io_next_word = uint32(word[0]) | (uint32(word[1]) << 8) | (uint32(word[2]) << 16) | (uint32(word[3]) << 24)
	return nil
}

func (ctx *internal) eatbits(n int) error {
	ctx.io_word = ctx.io_word << n
	delta := ctx.io_available_bits - n
	if delta >= 0 {
		ctx.io_available_bits = delta
		ctx.io_word |= ctx.io_next_word >> ctx.io_available_bits
		return nil
	}
	delta = -1 * delta // abs
	ctx.io_word |= ctx.io_next_word << delta

	err := ctx.readWord()
	if err != nil {
		return err
	}

	ctx.io_available_bits = 32 - delta
	ctx.io_word |= ctx.io_next_word >> ctx.io_available_bits
	return nil
}

func (i *internal) eat32bits() error {
	err := i.eatbits(16)
	if err != nil {
		return err
	}
	return i.eatbits(16)
}

func Decode(r io.Reader) (image.Image, error) {
	// read header
	ctx := &internal{}
	ctx.rb = r
	ctx.readWord()
	ctx.io_word = ctx.io_next_word
	magic := ctx.io_word
	ctx.eat32bits()
	if magic != 0x43495551 { // "QUIC"
		return nil, errors.New("quic: bad magic")
	}

	ctx.vers = ctx.io_word
	ctx.eat32bits()
	ctx.typ = quicImageType(ctx.io_word)
	ctx.eat32bits()
	ctx.width = ctx.io_word
	ctx.eat32bits()
	ctx.height = ctx.io_word
	ctx.eat32bits()

	if ctx.vers != 0 {
		return nil, fmt.Errorf("quic: unsupported version %x", ctx.vers)
	}
	log.Printf("quic: decoding image type=%s size=%d,%d", ctx.typ, ctx.width, ctx.height)

	// init channels
	for i := 0; i < 4; i++ {
		ctx.channels = append(ctx.channels, newChannel(ctx.width, ctx.typ.bpc()))
	}

	ctx.model = newModel(ctx.typ.bpc())
	ctx.rgb_state = newState()

	stride := ctx.typ.Stride()
	if stride == -1 {
		return nil, errors.New("invalid stride, format not supported")
	}
	// quic: decoding image type=RGB32 size=2048,1164

	pixbuf := make([]byte, ctx.width*ctx.height*uint32(stride))
	// fill all with 0xff so we have no alpha by defualt and all white
	for p := range pixbuf {
		pixbuf[p] = 0xff
	}

	err := ctx.decode(pixbuf, ctx.width*uint32(stride))
	if err != nil {
		return nil, err
	}

	switch ctx.typ {
	case QUIC_IMAGE_TYPE_RGB32, QUIC_IMAGE_TYPE_RGB24:
		final := &image.RGBA{
			Pix:    pixbuf,
			Stride: int(ctx.width) * stride,
			Rect:   image.Rectangle{Max: image.Point{X: int(ctx.width), Y: int(ctx.height)}},
		}
		return final, nil
	default:
		return nil, errors.New("TODO add quic format")
	}
}

func (ctx *internal) decode(pixbuf []byte, stride uint32) error {
	switch ctx.typ {
	case QUIC_IMAGE_TYPE_RGB32, QUIC_IMAGE_TYPE_RGB24:
		// 3 bytes on disk, we extract to 4 including 0xff alpha
		ctx.nChan = 3
		ctx.channels[0].correlate_row_zero = 0
		ctx.channels[1].correlate_row_zero = 0
		ctx.channels[2].correlate_row_zero = 0
		ctx.quic_rgb32_uncompress_row0(pixbuf[:stride])

		ctx.rows_completed++

		for row := uint32(1); row < ctx.height; row++ {
			prev := pixbuf
			pixbuf = pixbuf[stride:]
			ctx.channels[0].correlate_row_zero = ctx.channels[0].correlate_row[0]
			ctx.channels[1].correlate_row_zero = ctx.channels[1].correlate_row[0]
			ctx.channels[2].correlate_row_zero = ctx.channels[2].correlate_row[0]
			if err := ctx.quic_rgb32_uncompress_row(prev[:stride], pixbuf[:stride]); err != nil {
				return err
			}
			ctx.rows_completed++
		}
	default:
		return errors.New("TODO not supported image format")
	}
	return nil
}

func (ctx *internal) quic_rgb32_uncompress_row0(cur_row []byte) {
	bpc := uint32(8)
	bpc_mask := uint32(0xff)
	var pos uint32
	width := ctx.width

	for (wmimax > ctx.rgb_state.wmidx) && (ctx.rgb_state.wmileft <= width) {
		if ctx.rgb_state.wmileft > 0 {
			ctx.quic_rgb32_uncompress_row0_seg(pos, cur_row, pos+ctx.rgb_state.wmileft, bppmask[ctx.rgb_state.wmidx], bpc, bpc_mask)
			pos += ctx.rgb_state.wmileft
			width -= ctx.rgb_state.wmileft
		}
		ctx.rgb_state.wmidx++
		ctx.rgb_state.setWmTrigger()
		ctx.rgb_state.wmileft = defWminext
	}

	if width != 0 {
		ctx.quic_rgb32_uncompress_row0_seg(pos, cur_row, pos+width,
			bppmask[ctx.rgb_state.wmidx], bpc, bpc_mask)
		if wmimax > ctx.rgb_state.wmidx {
			ctx.rgb_state.wmileft -= width
		}
	}
}

func (ctx *internal) quic_rgb32_uncompress_row0_seg(i uint32, cur_row []byte, end, waitmask, bpc, bpc_mask uint32) {
	var stopidx uint32

	if i == 0 {
		cur_row[rgb32_pixel_pad] = 0xff
		c := uint32(0)
		for {
			rc, cwlen := family_8bpc.golombDecoding(ctx.channels[c].buckets_ptrs[ctx.channels[c].correlate_row[0]].bestcode, ctx.io_word)
			ctx.channels[c].correlate_row[0] = rc
			cur_row[c] = byte(family_8bpc.xlatL2U[rc] & 0xFF)
			ctx.eatbits(cwlen)

			c += 1
			if c >= ctx.nChan {
				break
			}
		}

		if ctx.rgb_state.waitcnt != 0 {
			ctx.rgb_state.waitcnt -= 1
		} else {
			ctx.rgb_state.waitcnt = (ctx.rgb_state.tabrand() & waitmask)
			c = 0
			for {
				ctx.channels[c].buckets_ptrs[ctx.channels[c].correlate_row_zero].updateModel(family_8bpc, ctx.rgb_state, uint32(ctx.channels[c].correlate_row[0]), bpc)

				c += 1
				if c >= ctx.nChan {
					break
				}
			}
		}
		i += 1
		stopidx = i + ctx.rgb_state.waitcnt

	} else {
		stopidx = i + ctx.rgb_state.waitcnt
	}

	for stopidx < end {
		for ; i <= stopidx; i += 1 {
			cur_row[(i*rgb32_pixel_size)+rgb32_pixel_pad] = 0xff
			c := uint32(0)
			for {
				rc, cwlen := family_8bpc.golombDecoding(ctx.channels[c].buckets_ptrs[ctx.channels[c].correlate_row[i-1]].bestcode, ctx.io_word)
				ctx.channels[c].correlate_row[i] = byte(rc)
				cur_row[(i*rgb32_pixel_size)+c] = byte((family_8bpc.xlatL2U[rc] + uint32(cur_row[((i-1)*rgb32_pixel_size)+c])) & bpc_mask)
				ctx.eatbits(cwlen)

				c += 1
				if c >= ctx.nChan {
					break
				}
			}
		}
		c := uint32(0)
		for {
			ctx.channels[c].buckets_ptrs[ctx.channels[c].correlate_row[stopidx-1]].updateModel(family_8bpc, ctx.rgb_state, uint32(ctx.channels[c].correlate_row[stopidx]), bpc)
			c += 1
			if c >= ctx.nChan {
				break
			}
		}
		stopidx = i + (ctx.rgb_state.tabrand() & waitmask)

	}

	for ; i < end; i += 1 {
		cur_row[(i*rgb32_pixel_size)+rgb32_pixel_pad] = 0xff
		c := uint32(0)
		for {
			rc, cwlen := family_8bpc.golombDecoding(ctx.channels[c].buckets_ptrs[ctx.channels[c].correlate_row[i-1]].bestcode, ctx.io_word)
			ctx.channels[c].correlate_row[i] = rc
			cur_row[(i*rgb32_pixel_size)+c] = byte((family_8bpc.xlatL2U[rc] + uint32(cur_row[((i-1)*rgb32_pixel_size)+c])) & bpc_mask)
			ctx.eatbits(cwlen)

			c += 1
			if c >= ctx.nChan {
				break
			}
		}
	}
	ctx.rgb_state.waitcnt = stopidx - end
}

func (ctx *internal) quic_rgb32_uncompress_row(prev_row, cur_row []byte) error {
	bpc := uint32(8)
	bpc_mask := uint32(0xff)
	var pos uint32
	width := ctx.width

	for (wmimax > ctx.rgb_state.wmidx) && (ctx.rgb_state.wmileft <= width) {
		if ctx.rgb_state.wmileft > 0 {
			if err := ctx.quic_rgb32_uncompress_row_seg(pos, prev_row, cur_row, pos+ctx.rgb_state.wmileft, bpc, bpc_mask); err != nil {
				return err
			}
			pos += ctx.rgb_state.wmileft
			width -= ctx.rgb_state.wmileft
		}
		ctx.rgb_state.wmidx += 1
		ctx.rgb_state.setWmTrigger()
		ctx.rgb_state.wmileft = defWminext
	}

	if width > 0 {
		if err := ctx.quic_rgb32_uncompress_row_seg(pos, prev_row, cur_row, pos+width, bpc, bpc_mask); err != nil {
			return err
		}
		if wmimax > ctx.rgb_state.wmidx {
			ctx.rgb_state.wmileft -= width
		}
	}
	return nil
}

func (this *internal) quic_rgb32_uncompress_row_seg(i uint32, prev_row, cur_row []byte, end, bpc, bpc_mask uint32) error {
	var waitmask = bppmask[this.rgb_state.wmidx]

	var run_index, stopidx, run_end uint32
	var c uint32
	var err error

	if i == 0 {
		cur_row[rgb32_pixel_pad] = 0xff

		for c = 0; c < this.nChan; c += 1 {
			rc, cwlen := family_8bpc.golombDecoding(this.channels[c].buckets_ptrs[this.channels[c].correlate_row_zero].bestcode, this.io_word)
			this.channels[c].correlate_row[0] = rc
			cur_row[c] = byte((family_8bpc.xlatL2U[this.channels[c].correlate_row[0]] + uint32(prev_row[c])) & bpc_mask)
			this.eatbits(cwlen)
		}

		if this.rgb_state.waitcnt != 0 {
			this.rgb_state.waitcnt -= 1
		} else {
			this.rgb_state.waitcnt = this.rgb_state.tabrand() & waitmask
			c = 0
			for c = 0; c < this.nChan; c += 1 {
				this.channels[c].buckets_ptrs[this.channels[c].correlate_row_zero].updateModel(family_8bpc, this.rgb_state, uint32(this.channels[c].correlate_row[0]), bpc)
			}
		}
		i += 1
		stopidx = i + this.rgb_state.waitcnt
	} else {
		stopidx = i + this.rgb_state.waitcnt
	}
	for {
		var rc = false
		for stopidx < end && !rc {
			for ; i <= stopidx && !rc; i++ {
				var pixel = i * rgb32_pixel_size
				var pixelm1 = (i - 1) * rgb32_pixel_size
				var pixelm2 = (i - 2) * rgb32_pixel_size

				if prev_row[pixelm1+rgb32_pixel_r] == prev_row[pixel+rgb32_pixel_r] && prev_row[pixelm1+rgb32_pixel_g] == prev_row[pixel+rgb32_pixel_g] && prev_row[pixelm1+rgb32_pixel_b] == prev_row[pixel+rgb32_pixel_b] {
					if run_index != i && i > 2 && (cur_row[pixelm1+rgb32_pixel_r] == cur_row[pixelm2+rgb32_pixel_r] && cur_row[pixelm1+rgb32_pixel_g] == cur_row[pixelm2+rgb32_pixel_g] && cur_row[pixelm1+rgb32_pixel_b] == cur_row[pixelm2+rgb32_pixel_b]) {
						/* do run */
						this.rgb_state.waitcnt = stopidx - i
						run_index = i
						run_end, err = this.decode_run(this.rgb_state)
						if err != nil {
							return err
						}
						run_end += i

						for ; i < run_end; i++ {
							var pixel = i * rgb32_pixel_size
							var pixelm1 = (i - 1) * rgb32_pixel_size
							cur_row[pixel+rgb32_pixel_pad] = 0xff
							cur_row[pixel+rgb32_pixel_r] = cur_row[pixelm1+rgb32_pixel_r]
							cur_row[pixel+rgb32_pixel_g] = cur_row[pixelm1+rgb32_pixel_g]
							cur_row[pixel+rgb32_pixel_b] = cur_row[pixelm1+rgb32_pixel_b]
						}

						if i == end {
							return nil
						} else {
							stopidx = i + this.rgb_state.waitcnt
							rc = true
							break
						}
					}
				}

				c = 0
				cur_row[pixel+rgb32_pixel_pad] = 0xff
				for c = 0; c < this.nChan; c += 1 {
					var cc = this.channels[c]
					var cr = cc.correlate_row

					rc, cwlen := family_8bpc.golombDecoding(cc.buckets_ptrs[cr[i-1]].bestcode, this.io_word)
					cr[i] = rc
					cur_row[pixel+c] = byte((family_8bpc.xlatL2U[rc] + ((uint32(cur_row[pixelm1+c]) + uint32(prev_row[pixel+c])) >> 1)) & bpc_mask)
					this.eatbits(cwlen)
				}
			}
			if rc {
				break
			}

			c = 0
			for c = 0; c < this.nChan; c += 1 {
				this.channels[c].buckets_ptrs[this.channels[c].correlate_row[stopidx-1]].updateModel(family_8bpc, this.rgb_state, uint32(this.channels[c].correlate_row[stopidx]), bpc)
			}

			stopidx = i + (this.rgb_state.tabrand() & waitmask)
		}

		for ; i < end && !rc; i++ {
			var pixel = i * rgb32_pixel_size
			var pixelm1 = (i - 1) * rgb32_pixel_size
			var pixelm2 = (i - 2) * rgb32_pixel_size

			if prev_row[pixelm1+rgb32_pixel_r] == prev_row[pixel+rgb32_pixel_r] && prev_row[pixelm1+rgb32_pixel_g] == prev_row[pixel+rgb32_pixel_g] && prev_row[pixelm1+rgb32_pixel_b] == prev_row[pixel+rgb32_pixel_b] {
				if run_index != i && i > 2 && (cur_row[pixelm1+rgb32_pixel_r] == cur_row[pixelm2+rgb32_pixel_r] && cur_row[pixelm1+rgb32_pixel_g] == cur_row[pixelm2+rgb32_pixel_g] && cur_row[pixelm1+rgb32_pixel_b] == cur_row[pixelm2+rgb32_pixel_b]) {
					/* do run */
					this.rgb_state.waitcnt = stopidx - i
					run_index = i
					run_end, err = this.decode_run(this.rgb_state)
					if err != nil {
						return err
					}
					run_end += i

					for ; i < run_end; i++ {
						var pixel = i * rgb32_pixel_size
						var pixelm1 = (i - 1) * rgb32_pixel_size
						cur_row[pixel+rgb32_pixel_pad] = 0xff
						cur_row[pixel+rgb32_pixel_r] = cur_row[pixelm1+rgb32_pixel_r]
						cur_row[pixel+rgb32_pixel_g] = cur_row[pixelm1+rgb32_pixel_g]
						cur_row[pixel+rgb32_pixel_b] = cur_row[pixelm1+rgb32_pixel_b]
					}

					if i == end {
						return nil
					} else {
						stopidx = i + this.rgb_state.waitcnt
						rc = true
						break
					}
				}
			}

			cur_row[pixel+rgb32_pixel_pad] = 0xff
			c = 0

			for c = 0; c < this.nChan; c += 1 {
				rc, cwlen := family_8bpc.golombDecoding(this.channels[c].buckets_ptrs[this.channels[c].correlate_row[i-1]].bestcode, this.io_word)
				this.channels[c].correlate_row[i] = rc
				cur_row[pixel+c] = byte((family_8bpc.xlatL2U[rc] + ((uint32(cur_row[pixelm1+c]) + uint32(prev_row[pixel+c])) >> 1)) & bpc_mask)
				this.eatbits(cwlen)
			}
		}

		if !rc {
			this.rgb_state.waitcnt = stopidx - end
			return nil
		}
	}
}

func (ctx *internal) decode_run(state *quicState) (uint32, error) {
	var runlen uint32

	for {
		var x = (^(ctx.io_word >> 24) >> 0) & 0xff
		var temp = int(zeroLUT[x])

		for hits := 1; hits <= temp; hits++ {
			runlen += uint32(state.melcorder)

			if state.melcstate < 32 {
				state.melcstate += 1
				state.melclen = uint32(quicJ[state.melcstate])
				state.melcorder = (1 << state.melclen)
			}
		}
		if temp != 8 {
			if err := ctx.eatbits(temp + 1); err != nil {
				return 0, err
			}

			break
		}
		if err := ctx.eatbits(8); err != nil {
			return 0, err
		}
	}

	if state.melclen != 0 {
		runlen += ctx.io_word >> (32 - state.melclen)
		ctx.eatbits(int(state.melclen))
	}

	if state.melcstate != 0 {
		state.melcstate -= 1
		state.melclen = uint32(quicJ[state.melcstate])
		state.melcorder = 1 << state.melclen
	}

	return runlen, nil
}
