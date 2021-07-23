package spice

import (
	"encoding/binary"
	"errors"
	"image"
	"log"
)

const (
	SPICE_MSG_CURSOR_INIT      = 101
	SPICE_MSG_CURSOR_RESET     = 102
	SPICE_MSG_CURSOR_SET       = 103
	SPICE_MSG_CURSOR_MOVE      = 104
	SPICE_MSG_CURSOR_HIDE      = 105
	SPICE_MSG_CURSOR_TRAIL     = 106
	SPICE_MSG_CURSOR_INVAL_ONE = 107
	SPICE_MSG_CURSOR_INVAL_ALL = 108

	SPICE_CURSOR_TYPE_ALPHA   = 0
	SPICE_CURSOR_TYPE_MONO    = 1
	SPICE_CURSOR_TYPE_COLOR4  = 2
	SPICE_CURSOR_TYPE_COLOR8  = 3
	SPICE_CURSOR_TYPE_COLOR16 = 4
	SPICE_CURSOR_TYPE_COLOR24 = 5
	SPICE_CURSOR_TYPE_COLOR32 = 6
)

type ChCursor struct {
	cl   *Client
	conn *SpiceConn
}

type cursorInfo struct {
	im            image.Image
	unique        uint64
	typ           uint8
	width, height uint16
	hotX, hotY    uint16
}

func (cl *Client) setupCursor(id uint8) (*ChCursor, error) {
	conn, err := cl.conn(ChannelCursor, id, nil)
	if err != nil {
		return nil, err
	}
	m := &ChCursor{cl: cl, conn: conn}
	conn.hndlr = m.handle

	go m.conn.ReadLoop()

	return m, nil
}

func (d *ChCursor) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_MSG_CURSOR_INIT:
		// Point16 position uint16 trail_length uint16 trail_frequency uint8 visible Cursor cursor
		if len(data) < 11 {
			// too short
			return
		}
		posX := binary.LittleEndian.Uint16(data[:2])
		posY := binary.LittleEndian.Uint16(data[2:4])
		tLen := binary.LittleEndian.Uint16(data[4:6])
		tFreq := binary.LittleEndian.Uint16(data[6:8])
		vis := data[8]

		if l := d.cl.Debug; l != nil {
			l.Printf("spice/cursor: init %d,%d trail=%d,%d visible=%d", posX, posY, tLen, tFreq, vis)
		}

		if vis == 0 {
			// invisible cursor
			d.cl.driver.SetCursor(image.NewRGBA(image.Rectangle{Max: image.Point{16, 16}}), 0, 0)
			return
		}

		cur, err := d.decodeCursor(data[9:])
		if err != nil {
			log.Printf("spice/cursor: failed to read cursor: %s", err)
		} else if cur != nil {
			d.cl.driver.SetCursor(cur.im, cur.hotX, cur.hotY)
		} else {
			d.cl.driver.SetCursor(nil, 0, 0)
		}
	case SPICE_MSG_CURSOR_RESET:
		// empty
		d.cl.driver.SetCursor(nil, 0, 0)
	case SPICE_MSG_CURSOR_MOVE:
		// ignore
	case SPICE_MSG_CURSOR_SET:
		if len(data) < 7 {
			// too short
			return
		}
		// Point16 position uint8 visible Cursor cursor
		posX := binary.LittleEndian.Uint16(data[:2])
		posY := binary.LittleEndian.Uint16(data[2:4])
		vis := data[4]

		if l := d.cl.Debug; l != nil {
			l.Printf("spice/cursor: set %d,%d vis=%d len=%d", posX, posY, vis, len(data))
		}

		if vis == 0 {
			// invisible cursor
			d.cl.driver.SetCursor(image.NewRGBA(image.Rectangle{Max: image.Point{16, 16}}), 0, 0)
			return
		}

		cur, err := d.decodeCursor(data[5:])
		if err != nil {
			log.Printf("spice/cursor: failed to read cursor: %s", err)
		} else if cur == nil {
			d.cl.driver.SetCursor(nil, 0, 0)
		} else {
			d.cl.driver.SetCursor(cur.im, cur.hotX, cur.hotY)
		}
	case SPICE_MSG_CURSOR_HIDE:
		d.cl.driver.SetCursor(nil, 0, 0)
	case SPICE_MSG_CURSOR_INVAL_ALL:
		// TODO clear cache
	default:
		log.Printf("spice/cursor: got message type=%d", typ)
	}
}

func (d *ChCursor) decodeCursor(data []byte) (*cursorInfo, error) {
	flags := binary.LittleEndian.Uint16(data[:2])
	if flags&1 == 1 {
		// no cursor header ... ?
		return nil, nil
	}
	// flags: 1=NONE, 2=CACHE_ME, 4=FROM_CACHE
	info := &cursorInfo{}

	info.unique = binary.LittleEndian.Uint64(data[2:10]) // unique cursor id, used for cache
	info.typ = data[10]                                  // 0=ALPHA 1=MONO 2=COLOR4 3=COLOR8 4=COLOR16 5=COLOR24 6=COLOR32
	info.width = binary.LittleEndian.Uint16(data[11:13])
	info.height = binary.LittleEndian.Uint16(data[13:15])
	info.hotX = binary.LittleEndian.Uint16(data[15:17])
	info.hotY = binary.LittleEndian.Uint16(data[17:19])

	data = data[19:]

	//l.Printf("spice/cursor: flags=%d unique=%d type=%d size=%d,%d hot=%d,%d rem=%d", flags, unique, typ, width, height, hotX, hotY, len(data))

	switch info.typ {
	case 0: // ALPHA
		// len = 16408-5-17 = 16386. 64*64*4 = 16384
		ln := int(info.width) * int(info.height) * 4
		if len(data) < ln {
			return nil, errors.New("unable to decode cursor: not enough data")
		}

		// reverse red & blue
		for i := 0; i < len(data); i += 4 {
			data[i], data[i+2] = data[i+2], data[i]
		}

		im := image.NewRGBA(image.Rectangle{Max: image.Point{int(info.width), int(info.height)}})
		copy(im.Pix, data)

		info.im = im

		return info, nil
	}

	// TODO
	return nil, nil
}
