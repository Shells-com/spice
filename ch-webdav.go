package spice

import (
	"log"
)

type SpiceWebdav struct {
	cl   *Client
	conn *SpiceConn
}

func (cl *Client) setupWebdav(id uint8) (*SpiceWebdav, error) {
	conn, err := cl.conn(ChannelWebdav, id, nil)
	if err != nil {
		return nil, err
	}
	m := &SpiceWebdav{cl: cl, conn: conn}
	conn.hndlr = m.handle

	go m.conn.ReadLoop()

	return m, nil
}

func (d *SpiceWebdav) handle(typ uint16, data []byte) {
	switch typ {
	default:
		log.Printf("spice/webdav: got message type=%d", typ)
	}
}
