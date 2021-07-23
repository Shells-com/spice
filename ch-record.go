package spice

import (
	"log"
)

type SpiceRecord struct {
	cl   *Client
	conn *SpiceConn
}

func (cl *Client) setupRecord(id uint8) (*SpiceRecord, error) {
	conn, err := cl.conn(ChannelRecord, id, nil)
	if err != nil {
		return nil, err
	}
	m := &SpiceRecord{cl: cl, conn: conn}
	conn.hndlr = m.handle

	go m.conn.ReadLoop()

	return m, nil
}

func (d *SpiceRecord) handle(typ uint16, data []byte) {
	switch typ {
	default:
		log.Printf("spice/record: got message type=%d", typ)
	}
}
