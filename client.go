package spice

import (
	"image"
	"log"
	"net"
	"sync"
	"time"
)

// SpiceConnector is a type of object that can create connections to a spice server
type Connector interface {
	SpiceConnect(compress bool) (net.Conn, error)
}

type Driver interface {
	DisplayInit(image.Image)
	DisplayRefresh()
	SetEventsTarget(*ChInputs)
	SetMainTarget(*ChMain)
	SetCursor(img image.Image, x, y uint16)

	// clipboard related
	ClipboardGrabbed(selection SpiceClipboardSelection, clipboardTypes []SpiceClipboardFormat)
	ClipboardFetch(selection SpiceClipboardSelection, clType SpiceClipboardFormat) ([]byte, error)
	ClipboardRelease(selection SpiceClipboardSelection)
}

type Client struct {
	c        Connector
	driver   Driver
	password string
	session  uint32 // connection_id
	displays uint32 // number of displays
	Debug    *log.Logger

	main     *ChMain
	playback *ChPlayback
	record   *ChRecord

	mmTime  uint32    // mmTime as of start
	mmStamp time.Time // mmStamp is the time at which mmTime was set
	mmLock  sync.RWMutex
}

func New(c Connector, driver Driver, password string) (*Client, error) {
	cl := &Client{c: c, driver: driver, password: password}

	// connection has been established
	err := cl.setupMain()
	if err != nil {
		return nil, err
	}

	// connect channels
	var wg sync.WaitGroup
	for _, ch := range cl.main.channels {
		switch ch.typ {
		case ChannelDisplay:
			if ch.id > 0 {
				// TODO handle multiple screens
				continue
			}
			wg.Add(1)
			go func(id uint8) {
				defer wg.Done()
				cl.setupDisplay(id)
			}(ch.id)
		case ChannelInputs:
			wg.Add(1)
			go func(id uint8) {
				defer wg.Done()
				cl.setupInputs(id)
			}(ch.id)
		case ChannelCursor:
			if ch.id > 0 {
				// TODO handle multiple screens
				continue
			}
			wg.Add(1)
			go func(id uint8) {
				defer wg.Done()
				cl.setupCursor(id)
			}(ch.id)
		case ChannelPlayback:
			wg.Add(1)
			go func(id uint8) {
				defer wg.Done()
				cl.playback, _ = cl.setupPlayback(id)
			}(ch.id)
		case ChannelRecord:
			wg.Add(1)
			go func(id uint8) {
				defer wg.Done()
				cl.record, _ = cl.setupRecord(id)
			}(ch.id)
		case ChannelWebdav:
			wg.Add(1)
			go func(id uint8) {
				defer wg.Done()
				cl.setupWebdav(id)
			}(ch.id)
		case ChannelUsbRedir:
			log.Printf("spice: USB supported, device #%d", ch.id)
			// do nothing, this is useful elsewhere
		default:
			log.Printf("spice: could not connect to channel %s[%d]: unknown type", ch.typ, ch.id)
		}
	}
	wg.Wait()

	return cl, nil
}

func (client *Client) conn(typ Channel, chId uint8, channelCaps []uint32) (*SpiceConn, error) {
	compress := false
	if typ == ChannelDisplay {
		// we want to compress that
		compress = true
	}
	cnx, err := client.c.SpiceConnect(compress)
	if err != nil {
		return nil, err
	}
	conn := &SpiceConn{client: client, conn: cnx}

	if err := conn.handshake(typ, chId, channelCaps); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (client *Client) MediaTime() uint32 {
	client.mmLock.RLock()
	defer client.mmLock.RUnlock()

	// compute current media time
	return client.mmTime + uint32(time.Since(client.mmStamp)/time.Millisecond)
}

func (client *Client) MediaTill(t uint32) time.Duration {
	client.mmLock.RLock()
	defer client.mmLock.RUnlock()

	// calculate time until reaching the specified media time.
	// "t" is assumed to be > mmTime, but actually it'll work either way
	tOfft := (time.Duration(t) - time.Duration(client.mmTime)) * time.Millisecond
	return time.Until(client.mmStamp.Add(tOfft))
}

func (client *Client) UpdateView(w, h int) {
	if m := client.main; m != nil {
		m.MonitorConfig(0, []SpiceMonitor{SpiceMonitor{Width: uint32(w), Height: uint32(h), Depth: 32}})
	}
}

func (client *Client) ToggleMute() {
	client.playback.mute = !client.playback.mute
}

func (client *Client) SetMute(muted bool) {
	client.playback.mute = muted
}

func (client *Client) GetMute() bool {
	return client.playback.mute
}
