package spice

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"log"
)

const (
	SPICE_MSG_DISPLAY_MODE      = 101
	SPICE_MSG_DISPLAY_MARK      = 102
	SPICE_MSG_DISPLAY_RESET     = 103
	SPICE_MSG_DISPLAY_COPY_BITS = 104

	SPICE_MSG_DISPLAY_INVAL_LIST         = 105
	SPICE_MSG_DISPLAY_INVAL_ALL_PIXMAPS  = 106
	SPICE_MSG_DISPLAY_INVAL_PALETTE      = 107
	SPICE_MSG_DISPLAY_INVAL_ALL_PALETTES = 108

	SPICE_MSG_DISPLAY_STREAM_CREATE      = 122
	SPICE_MSG_DISPLAY_STREAM_DATA        = 123
	SPICE_MSG_DISPLAY_STREAM_CLIP        = 124
	SPICE_MSG_DISPLAY_STREAM_DESTROY     = 125
	SPICE_MSG_DISPLAY_STREAM_DESTROY_ALL = 126

	SPICE_MSG_DISPLAY_DRAW_FILL              = 302
	SPICE_MSG_DISPLAY_DRAW_OPAQUE            = 303
	SPICE_MSG_DISPLAY_DRAW_COPY              = 304
	SPICE_MSG_DISPLAY_DRAW_BLEND             = 305
	SPICE_MSG_DISPLAY_DRAW_BLACKNESS         = 306
	SPICE_MSG_DISPLAY_DRAW_WHITENESS         = 307
	SPICE_MSG_DISPLAY_DRAW_INVERS            = 308
	SPICE_MSG_DISPLAY_DRAW_ROP3              = 309
	SPICE_MSG_DISPLAY_DRAW_STROKE            = 310
	SPICE_MSG_DISPLAY_DRAW_TEXT              = 311
	SPICE_MSG_DISPLAY_DRAW_TRANSPARENT       = 312
	SPICE_MSG_DISPLAY_DRAW_ALPHA_BLEND       = 313
	SPICE_MSG_DISPLAY_SURFACE_CREATE         = 314
	SPICE_MSG_DISPLAY_SURFACE_DESTROY        = 315
	SPICE_MSG_DISPLAY_STREAM_DATA_SIZED      = 316
	SPICE_MSG_DISPLAY_MONITORS_CONFIG        = 317
	SPICE_MSG_DISPLAY_DRAW_COMPOSITE         = 318
	SPICE_MSG_DISPLAY_STREAM_ACTIVATE_REPORT = 319
	SPICE_MSG_DISPLAY_GL_SCANOUT_UNIX        = 320
	SPICE_MSG_DISPLAY_GL_DRAW                = 321
	SPICE_MSG_DISPLAY_QUALITY_INDICATOR      = 322

	SPICE_MSGC_DISPLAY_INIT                       = 101
	SPICE_MSGC_DISPLAY_STREAM_REPORT              = 102
	SPICE_MSGC_DISPLAY_PREFERRED_COMPRESSION      = 103
	SPICE_MSGC_DISPLAY_GL_DRAW_DONE               = 104
	SPICE_MSGC_DISPLAY_PREFERRED_VIDEO_CODEC_TYPE = 105
)

const (
	// 4178 = 1000001010010
	SPICE_DISPLAY_CAP_SIZED_STREAM    uint32 = iota
	SPICE_DISPLAY_CAP_MONITORS_CONFIG        // X
	SPICE_DISPLAY_CAP_COMPOSITE
	SPICE_DISPLAY_CAP_A8_SURFACE
	SPICE_DISPLAY_CAP_STREAM_REPORT // X
	SPICE_DISPLAY_CAP_LZ4_COMPRESSION
	SPICE_DISPLAY_CAP_PREF_COMPRESSION // X
	SPICE_DISPLAY_CAP_GL_SCANOUT
	SPICE_DISPLAY_CAP_MULTI_CODEC
	SPICE_DISPLAY_CAP_CODEC_MJPEG
	SPICE_DISPLAY_CAP_CODEC_VP8
	SPICE_DISPLAY_CAP_CODEC_H264
	SPICE_DISPLAY_CAP_PREF_VIDEO_CODEC_TYPE // X
	SPICE_DISPLAY_CAP_CODEC_VP9
	SPICE_DISPLAY_CAP_CODEC_H265
)

const (
	SPICE_IMAGE_COMPRESSION_INVALID uint8 = iota
	SPICE_IMAGE_COMPRESSION_OFF
	SPICE_IMAGE_COMPRESSION_AUTO_GLZ
	SPICE_IMAGE_COMPRESSION_AUTO_LZ
	SPICE_IMAGE_COMPRESSION_QUIC
	SPICE_IMAGE_COMPRESSION_GLZ
	SPICE_IMAGE_COMPRESSION_LZ
	SPICE_IMAGE_COMPRESSION_LZ4
)

type SpiceDisplay struct {
	cl   *Client
	conn *SpiceConn

	display draw.Image
}

func (cl *Client) setupDisplay(id uint8) (*SpiceDisplay, error) {
	conn, err := cl.conn(ChannelDisplay, id, caps(
		SPICE_DISPLAY_CAP_SIZED_STREAM,
		SPICE_DISPLAY_CAP_STREAM_REPORT,
		SPICE_DISPLAY_CAP_MONITORS_CONFIG,
		SPICE_DISPLAY_CAP_MULTI_CODEC,
		SPICE_DISPLAY_CAP_LZ4_COMPRESSION,
		SPICE_DISPLAY_CAP_PREF_COMPRESSION,
	))
	if err != nil {
		return nil, err
	}
	m := &SpiceDisplay{cl: cl, conn: conn}
	conn.hndlr = m.handle

	go m.conn.ReadLoop()

	// enable image caching and global dictionary compression
	m.conn.WriteMessage(SPICE_MSGC_DISPLAY_INIT, make([]byte, 14))

	// say we like LZ
	m.conn.WriteMessage(SPICE_MSGC_DISPLAY_PREFERRED_COMPRESSION, []byte{SPICE_IMAGE_COMPRESSION_AUTO_LZ})

	// say we like MJPEG & VP8
	// SPICE_MSGC_DISPLAY_PREFERRED_VIDEO_CODEC_TYPE
	// 1=MJPEG 2=VP8 3=H264 4=VP9 5=H265
	m.conn.WriteMessage(SPICE_MSGC_DISPLAY_PREFERRED_VIDEO_CODEC_TYPE, uint8(2), uint8(1), uint8(2))

	return m, nil
}

func (d *SpiceDisplay) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_MSG_DISPLAY_MARK:
		log.Printf("spice/display: MARK!")

		d.cl.driver.DisplayInit(d.display)
	case SPICE_MSG_DISPLAY_INVAL_ALL_PALETTES:
		log.Printf("spice/display: TODO invalidate all palettes")
	case SPICE_MSG_DISPLAY_DRAW_FILL:
		d.handleDrawFill(data)
	case SPICE_MSG_DISPLAY_DRAW_COPY:
		d.handleDrawCopy(data)
	case SPICE_MSG_DISPLAY_SURFACE_CREATE:
		if len(data) < 20 {
			log.Printf("spice/display: surface create packet too short")
			return
		}
		sid := binary.LittleEndian.Uint32(data[:4])
		width := binary.LittleEndian.Uint32(data[4:8])
		height := binary.LittleEndian.Uint32(data[8:12])
		fmt := binary.LittleEndian.Uint32(data[12:16])
		flags := binary.LittleEndian.Uint32(data[16:20])

		d.initSurface(sid, width, height, fmt, flags)
		log.Printf("spice/display: surface create, id=%d %dx%d fmt=%d flags=%d", sid, width, height, fmt, flags)
	case SPICE_MSG_DISPLAY_SURFACE_DESTROY:
		// we don't really care but...
		log.Printf("spice/display: TODO surface destroy")
	case SPICE_MSG_DISPLAY_MONITORS_CONFIG:
		if len(data) < 4 {
			// too small
			return
		}
		cnt := int(binary.LittleEndian.Uint16(data[:2]))
		max := binary.LittleEndian.Uint16(data[2:4])

		if len(data) < 4+(cnt*28) {
			// too small
			return
		}

		log.Printf("spice/display: SPICE_MSG_DISPLAY_MONITORS_CONFIG has %d/%d heads", cnt, max)
		for i := 0; i < cnt; i++ {
			info := data[4+(i*28) : 4+((i+1)*28)]
			monitorId := binary.LittleEndian.Uint32(info[:4])
			surfaceId := binary.LittleEndian.Uint32(info[4:8])
			width := binary.LittleEndian.Uint32(info[8:12])
			height := binary.LittleEndian.Uint32(info[12:16])
			x := binary.LittleEndian.Uint32(info[16:20])
			y := binary.LittleEndian.Uint32(info[20:24])
			flags := binary.LittleEndian.Uint32(info[24:28])

			log.Printf("spice/display: found monitor #%d (surface %d): %dx%d pos=%d,%d flags=%d", monitorId, surfaceId, width, height, x, y, flags)
		}
	default:
		log.Printf("spice/display: got message type=%d", typ)
	}
}

func (d *SpiceDisplay) initSurface(sid, width, height, fmt, flags uint32) {
	switch fmt {
	case 1: // 1_A
		log.Printf("todo")
	case 8: // 8_A
		log.Printf("todo")
	case 16: // 16_555
		log.Printf("todo")
	case 80: // 16_565
		log.Printf("todo")
	case 32, 96: // 32_xRGB, 32_ARGB
		img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
		for p := range img.Pix {
			if p%4 == 3 {
				img.Pix[p] = 0xff
			} else {
				img.Pix[p] = 0
			}
		}

		d.display = img
	}
}

func (d *SpiceDisplay) handleDrawFill(req []byte) {
	// DisplayBase, Brush brush, ropd, QMask

	//log.Printf("spice/display: draw fill data len=%d", len(req))
	r := bytes.NewReader(req)
	// display_base: uint32 surface_id Rect box (x,y,w,h) Clip clip (clip_type type int8 none|rects, if rects → uint32 num_rects, rects[num_rects])
	base := &DisplayBase{}
	err := base.Decode(r)
	if err != nil {
		log.Printf("failed to decode display base: %s", err)
		return
	}

	// brush_type enum8 → 0=NONE 1=SOLID 2=PATTERN
	var brushType uint8
	var brushColor uint32
	binary.Read(r, binary.LittleEndian, &brushType)

	switch brushType {
	case 0: // NONE
		// nothing
		// ...
		return
	case 1: // SOLID
		binary.Read(r, binary.LittleEndian, &brushColor)
	case 2: // PATTERN
		log.Printf("spice/display: pattern draw_fill not implemented")
		return
	}

	// ropd rop_descriptor
	var ropd Ropd
	binary.Read(r, binary.LittleEndian, &ropd)

	// QMask mask @outvar(mask)
	var qmask QMask
	qmask.Decode(r)

	if qmask.ImagePtr != 0 {
		qmask.Image, err = DecodeImage(req[qmask.ImagePtr:])
	}

	// Apply COLOR
	color := color.RGBA{uint8(brushColor), uint8(brushColor >> 8), uint8(brushColor >> 16), uint8(brushColor >> 24)}
	b := base.Box.Rectangle()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d.display.Set(x, y, color)
		}
	}
	d.cl.driver.DisplayRefresh()
}

func (d *SpiceDisplay) handleDrawCopy(req []byte) {
	//log.Printf("spice/display: draw copy data len=%d", len(req))
	r := bytes.NewReader(req)
	// display_base: uint32 surface_id Rect box (x,y,w,h) Clip clip (clip_type type int8 none|rects, if rects → uint32 num_rects, rects[num_rects])
	base := &DisplayBase{}
	err := base.Decode(r)
	if err != nil {
		log.Printf("failed to decode display base: %s", err)
		return
	}
	//log.Printf("display base = %+v", base)

	var imgPtr uint32
	binary.Read(r, binary.LittleEndian, &imgPtr)

	// Rect src_area
	srcArea := &Rect{}
	srcArea.Decode(r)

	// ropd rop_descriptor
	var ropd Ropd
	binary.Read(r, binary.LittleEndian, &ropd)

	// image_scale_mode scale_mode
	var scaleMode ImageScaleMode
	binary.Read(r, binary.LittleEndian, &scaleMode)

	// QMask mask @outvar(mask)
	var qmask QMask
	qmask.Decode(r)

	if qmask.ImagePtr != 0 {
		qmask.Image, err = DecodeImage(req[qmask.ImagePtr:])
	}

	// decode image
	img, err := DecodeImage(req[imgPtr:])
	if err != nil {
		log.Printf("failed to decode image: %s", err)
		return
	}

	// put image on d.display, then refresh canvas
	draw.Draw(d.display, base.Box.Rectangle(), img.Image, image.Point{0, 0}, draw.Over)
	d.cl.driver.DisplayRefresh()
}
