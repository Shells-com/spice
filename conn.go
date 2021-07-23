package spice

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

type SpiceConn struct {
	client *Client
	conn   net.Conn
	serial uint64
	wLock  sync.Mutex
	hndlr  func(typ uint16, data []byte)
	pub    *rsa.PublicKey
	typ    Channel
	id     uint8

	// negociated protocol version
	major uint32
	minor uint32

	commonCaps  []uint32
	channelCaps []uint32
	validCaps   []uint32

	miniHeaders bool

	ackW uint32     // ack window, send acknowledgment for every “window” messages
	ackP uint32     // ack position, once it's == ackW, send ack.
	ackL sync.Mutex // lock for ack
}

func (c *SpiceConn) ReadLoop() {
	// read packets as long as we can
	for {
		err := c.ReadData(func(typ uint16, data []byte) error {
			// this might need to be moved after process?
			doAck := false
			c.ackL.Lock()
			if c.ackW > 0 {
				c.ackP += 1
				if c.ackP >= c.ackW {
					c.ackP = 0
					doAck = true
				}
			}
			c.ackL.Unlock()

			if doAck {
				// send ack
				c.WriteMessage(SPICE_MSGC_ACK)
			}

			return c.process(typ, data)
		})
		if err != nil {
			log.Printf("spice: read failed: %s", err)
			return
		}

	}
}

func (c *SpiceConn) String() string {
	return fmt.Sprintf("%s[%d]", c.typ.String(), c.id)
}

func (c *SpiceConn) process(typ uint16, data []byte) error {
	// process message
	switch typ {
	case SPICE_MSG_SET_ACK:
		c.ackL.Lock()
		defer c.ackL.Unlock()

		if len(data) < 8 {
			// not enough data
			return nil
		}

		gen := binary.LittleEndian.Uint32(data[:4])
		c.ackW = binary.LittleEndian.Uint32(data[4:8])
		c.ackP = 0

		log.Printf("spice: %s connection ack window set to %d (gen=%d)", c.String(), c.ackW, gen)

		// send ack_sync response
		c.WriteMessage(SPICE_MSGC_ACK_SYNC, gen)
	case SPICE_MSG_PING:
		//log.Printf("spice: %s Ping? Pong. Data len=%d", c.String(), len(m.Data))
		if len(data) > 12 {
			data = data[:12]
		}
		// send pong
		c.WriteMessage(SPICE_MSGC_PONG, data)
	case SPICE_MSG_NOTIFY:
		buf := bytes.NewReader(data)
		var ts uint64
		var severity, visibility, what, ln uint32

		binary.Read(buf, binary.LittleEndian, &ts)
		binary.Read(buf, binary.LittleEndian, &severity)
		binary.Read(buf, binary.LittleEndian, &visibility)
		binary.Read(buf, binary.LittleEndian, &what)
		binary.Read(buf, binary.LittleEndian, &ln)

		msg := make([]byte, ln)
		io.ReadFull(buf, msg)

		// example: severity=1 visibility=2 what=0 keyboard channel is insecure
		// severity: INFO|WARN|ERROR
		// visibility: LOW|MEDIUM|HIGH
		// what: error_code/warn_code/info_code

		log.Printf("spice: %s says ts=%d severity=%d visibility=%d what=%d: %s", c.String(), ts, severity, visibility, what, msg)
	case SPICE_MSG_WAIT_FOR_CHANNELS:
		// TODO
		log.Printf("spice: %s got SPICE_MSG_WAIT_FOR_CHANNELS, ignored", c.String())
	case SPICE_MSG_DISCONNECTING:
		log.Printf("spice: %s got SPICE_MSG_DISCONNECTING", c.String())
	default:
		if c.hndlr != nil {
			c.hndlr(typ, data)
		}
	}
	return nil
}

func (c *SpiceConn) Write(buf []byte) (int, error) {
	return c.conn.Write(buf)
}

func (c *SpiceConn) Read(buf []byte) (int, error) {
	return c.conn.Read(buf)
}

func (c *SpiceConn) ReadFull(buf []byte) error {
	_, err := io.ReadFull(c.conn, buf)
	return err
}

func (c *SpiceConn) ReadError() error {
	buf := make([]byte, 4)
	err := c.ReadFull(buf)
	if err != nil {
		return err
	}
	err = SpiceError(binary.LittleEndian.Uint32(buf))
	if err == ErrSpiceLinkOk {
		return nil
	}
	return err
}

func (c *SpiceConn) ReadData(cb func(typ uint16, data []byte) error) error {

	if c.miniHeaders {
		// only type & size
		var typ uint16
		var size uint32
		err := binary.Read(c.conn, binary.LittleEndian, &typ)
		if err != nil {
			return err
		}
		err = binary.Read(c.conn, binary.LittleEndian, &size)
		if err != nil {
			return err
		}

		if size > 10*1024*1024 {
			return errors.New("size too large, limited to 10MB")
		}

		buf := make([]byte, size)
		if err = c.ReadFull(buf); err != nil {
			return err
		}
		return cb(typ, buf)
	}

	var size, subList uint32
	var typ uint16
	var serial uint64

	err := binary.Read(c.conn, binary.LittleEndian, &serial)
	if err != nil {
		return err
	}
	binary.Read(c.conn, binary.LittleEndian, &typ)
	binary.Read(c.conn, binary.LittleEndian, &size)
	binary.Read(c.conn, binary.LittleEndian, &subList)

	//log.Printf("spice: read data serial=%d type=%d size=%d subList=%d", d.Serial, d.Message.Type, size, subList)

	if size > 10*1024*1024 {
		return errors.New("size too large, limited to 10MB")
	}

	buf := make([]byte, size)
	if err := c.ReadFull(buf); err != nil {
		return err
	}

	if subList == 0 {
		// simple
		return cb(typ, buf)
	}

	// ok we have to deal with sublist and all that crap. But first, let's set the msg
	mainBuf := buf[:subList]

	subCnt := binary.LittleEndian.Uint16(buf[subList : subList+2])

	// TODO check all values against going out of bound of the slice
	for i := uint16(0); i < subCnt; i++ {
		offt := subList + 2 + (uint32(i) * 4)
		offt = binary.LittleEndian.Uint32(buf[offt : offt+4])

		size := binary.LittleEndian.Uint32(buf[offt+2 : offt+6])

		subTyp := binary.LittleEndian.Uint16(buf[offt : offt+2])
		subDat := buf[offt+6 : offt+6+size]

		if err := cb(subTyp, subDat); err != nil {
			return err
		}
	}
	return cb(typ, mainBuf)
}

func (c *SpiceConn) WriteMessage(typ uint16, data ...interface{}) error {
	var buf []byte

	for _, subdata := range data {
		switch v := subdata.(type) {
		case []byte:
			if buf == nil {
				buf = v
			} else {
				buf = append(buf, v...)
			}
		default:
			w := &bytes.Buffer{}
			err := binary.Write(w, binary.LittleEndian, subdata)
			if err != nil {
				return err
			}
			if buf == nil {
				buf = w.Bytes()
			} else {
				buf = append(buf, w.Bytes()...)
			}
		}
	}
	c.wLock.Lock()
	defer c.wLock.Unlock()

	if c.miniHeaders {
		binary.Write(c.conn, binary.LittleEndian, typ)
		binary.Write(c.conn, binary.LittleEndian, uint32(len(buf)))
		_, err := c.conn.Write(buf)
		return err
	}

	// easy
	hdr := &bytes.Buffer{}
	serial := atomic.AddUint64(&c.serial, 1)

	binary.Write(hdr, binary.LittleEndian, serial)
	binary.Write(hdr, binary.LittleEndian, typ)
	binary.Write(hdr, binary.LittleEndian, uint32(len(buf)))
	binary.Write(hdr, binary.LittleEndian, uint32(len(buf)))

	_, err := c.Write(hdr.Bytes())
	if err != nil {
		return err
	}
	_, err = c.Write(buf)
	return err
}

func (c *SpiceConn) handshake(typ Channel, chId uint8, channelCaps []uint32) error {
	c.typ = typ
	c.id = chId
	err := c.sendSpiceLinkMess(typ, chId, channelCaps)
	if err != nil {
		return err
	}
	err = c.readSpiceLinkReply()
	if err != nil {
		return err
	}

	cnt := len(c.channelCaps)
	if cnt2 := len(channelCaps); cnt2 < cnt {
		cnt = cnt2
	}

	if cnt > 0 {
		c.validCaps = make([]uint32, cnt)
		for i := 0; i < cnt; i++ {
			c.validCaps[i] = channelCaps[i] & c.channelCaps[i]
		}
	}
	log.Printf("spice: %s channel req_caps=%v caps=%v valid_caps=%v", c.String(), channelCaps, c.channelCaps, c.validCaps)

	// encrypt password
	ciphertext, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, c.pub, []byte(c.client.password), nil)
	if err != nil {
		return err
	}

	c.Write(ciphertext)
	return c.ReadError()
}

func (c *SpiceConn) sendSpiceLinkMess(typ Channel, chId uint8, channelCaps []uint32) error {
	// generate a SpiceLinkMess packet and send
	pkt := &bytes.Buffer{}

	commonCaps := caps(SPICE_COMMON_CAP_MINI_HEADER)

	binary.Write(pkt, binary.LittleEndian, c.client.session)
	binary.Write(pkt, binary.LittleEndian, typ)
	binary.Write(pkt, binary.LittleEndian, chId)
	binary.Write(pkt, binary.LittleEndian, uint32(len(commonCaps)))  // num_common_caps
	binary.Write(pkt, binary.LittleEndian, uint32(len(channelCaps))) // num_channel_caps
	binary.Write(pkt, binary.LittleEndian, uint32(18))               // caps_offset

	for _, c := range commonCaps {
		binary.Write(pkt, binary.LittleEndian, c)
	}
	for _, c := range channelCaps {
		binary.Write(pkt, binary.LittleEndian, c)
	}

	buf := pkt.Bytes()

	pkt = &bytes.Buffer{}

	pkt.Write([]byte(SPICE_MAGIC))
	binary.Write(pkt, binary.LittleEndian, uint32(SPICE_VERSION_MAJOR))
	binary.Write(pkt, binary.LittleEndian, uint32(SPICE_VERSION_MINOR))
	binary.Write(pkt, binary.LittleEndian, uint32(len(buf)))
	pkt.Write(buf)

	// write
	_, err := pkt.WriteTo(c.conn)
	return err
}

func (c *SpiceConn) readSpiceLinkReply() error {
	hdr := make([]byte, 16)
	_, err := io.ReadFull(c.conn, hdr)
	if err != nil {
		return err
	}

	// hdr = magic + major_version + minor_version + size
	if string(hdr[:4]) != SPICE_MAGIC {
		return errors.New("invalid magic")
	}

	c.major = binary.LittleEndian.Uint32(hdr[4:8])
	c.minor = binary.LittleEndian.Uint32(hdr[8:12])
	size := binary.LittleEndian.Uint32(hdr[12:16])

	if size > 512 {
		return errors.New("SpiceLinkReply packet too large")
	}

	//log.Printf("spice: connected to server running Spice protocol version %d.%d", c.major, c.minor)

	pkt := make([]byte, size)
	_, err = io.ReadFull(c.conn, pkt)
	if err != nil {
		return err
	}

	//log.Printf("received data=\n%s", hex.Dump(pkt))

	r := bytes.NewReader(pkt)
	var spiceErr SpiceError
	binary.Read(r, binary.LittleEndian, &spiceErr)

	if spiceErr != ErrSpiceLinkOk {
		return fmt.Errorf("error in SpiceLinkReply packet: %w", spiceErr)
	}

	// 1024 bit RSA public key in X.509 SubjectPublicKeyInfo format
	pubKey := make([]byte, SPICE_TICKET_PUBKEY_BYTES)
	_, err = io.ReadFull(r, pubKey)
	if err != nil {
		return err
	}

	pk, err := x509.ParsePKIXPublicKey(pubKey)
	if err != nil {
		return err
	}
	if pk2, ok := pk.(*rsa.PublicKey); ok {
		c.pub = pk2
	} else {
		return errors.New("invalid public key")
	}

	var commonCaps, channelCaps, capsOffset uint32
	binary.Read(r, binary.LittleEndian, &commonCaps)
	binary.Read(r, binary.LittleEndian, &channelCaps)
	binary.Read(r, binary.LittleEndian, &capsOffset)

	_ = capsOffset

	for i := uint32(0); i < commonCaps; i++ {
		var v uint32
		binary.Read(r, binary.LittleEndian, &v)
		c.commonCaps = append(c.commonCaps, v)
	}
	for i := uint32(0); i < channelCaps; i++ {
		var v uint32
		binary.Read(r, binary.LittleEndian, &v)
		c.channelCaps = append(c.channelCaps, v)
	}

	if len(c.commonCaps) > 0 && c.commonCaps[0]&(1<<SPICE_COMMON_CAP_MINI_HEADER) == (1<<SPICE_COMMON_CAP_MINI_HEADER) {
		c.miniHeaders = true
	}

	// common caps= 0xb, channel caps=0x9 ... ... ???

	return nil
}

func (c *SpiceConn) Close() error {
	return c.conn.Close()
}

func (c *SpiceConn) writeLoop(ch chan *spicePacket) {
	for pkt := range ch {
		c.WriteMessage(pkt.typ, pkt.data...)
	}
}
