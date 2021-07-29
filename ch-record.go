package spice

import (
	"encoding/binary"
	"log"
)

type ChRecord struct {
	cl   *Client
	conn *SpiceConn
}

const (
	SPICE_MSG_RECORD_START  = 101
	SPICE_MSG_RECORD_STOP   = 102
	SPICE_MSG_RECORD_VOLUME = 103
	SPICE_MSG_RECORD_MUTE   = 104

	SPICE_RECORD_CAP_CELT_0_5_1 = 0
	SPICE_RECORD_CAP_VOLUME     = 1
	SPICE_RECORD_CAP_OPUS       = 2
)

func (cl *Client) setupRecord(id uint8) (*ChRecord, error) {
	conn, err := cl.conn(ChannelRecord, id, caps(SPICE_RECORD_CAP_VOLUME, SPICE_RECORD_CAP_OPUS))
	if err != nil {
		return nil, err
	}
	m := &ChRecord{cl: cl, conn: conn}
	conn.hndlr = m.handle

	go m.conn.ReadLoop()

	return m, nil
}

func (d *ChRecord) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_MSG_RECORD_VOLUME:
		// uint8 nchannels, uint16[]volume
		if len(data) < 1 {
			return
		}
		cnt := int(data[0])
		data = data[1:]
		if len(data) != cnt*2 {
			return // invalid
		}
		vol := make([]uint16, cnt)
		for n := 0; n < cnt; n++ {
			vol[n] = binary.LittleEndian.Uint16(data[:2])
			data = data[2:]
		}
		log.Printf("spice/record: volume information: %v", vol)
	case SPICE_MSG_RECORD_MUTE:
		// uint8 mute
		if len(data) < 1 {
			return
		}
		log.Printf("spice/record: mute information: %d", data[0])
	default:
		log.Printf("spice/record: got message type=%d", typ)
	}
}
