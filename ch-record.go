package spice

import (
	"log"
)

type ChRecord struct {
	cl   *Client
	conn *SpiceConn
}

func (cl *Client) setupRecord(id uint8) (*ChRecord, error) {
	conn, err := cl.conn(ChannelRecord, id, nil)
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
	default:
		log.Printf("spice/record: got message type=%d", typ)
	}
}
