// Package spice implements a client for the SPICE remote desktop protocol
// commonly used with QEMU and libvirt virtualization systems.
package spice

import (
	"image"
	"log"
	"net"
	"sync"
	"time"
)

// Connector is an interface that can establish network connections to a SPICE server
// with optional compression for the display channel
type Connector interface {
	SpiceConnect(compress bool) (net.Conn, error)
}

// Driver is the interface that must be implemented by clients to handle
// display updates, input events, cursor changes and clipboard operations
type Driver interface {
	// DisplayInit initializes the display with the given image
	DisplayInit(image.Image)
	// DisplayRefresh triggers a refresh of the display
	DisplayRefresh()
	// SetEventsTarget sets the input events channel for sending user input
	SetEventsTarget(*ChInputs)
	// SetMainTarget sets the main channel for server communication
	SetMainTarget(*ChMain)
	// SetCursor updates the cursor image and position
	SetCursor(img image.Image, x, y uint16)

	// Clipboard related methods
	// ClipboardGrabbed is called when the server grabs the clipboard
	ClipboardGrabbed(selection SpiceClipboardSelection, clipboardTypes []SpiceClipboardFormat)
	// ClipboardFetch retrieves clipboard data from the client
	ClipboardFetch(selection SpiceClipboardSelection, clType SpiceClipboardFormat) ([]byte, error)
	// ClipboardRelease is called when the server releases the clipboard
	ClipboardRelease(selection SpiceClipboardSelection)
}

// Client represents a SPICE protocol client connection
// It manages all channel connections and coordinates communication
type Client struct {
	c        Connector   // Network connection provider
	driver   Driver      // Implementation for handling display/input
	password string      // Password for SPICE authentication
	session  uint32      // SPICE connection ID
	displays uint32      // Number of displays available
	Debug    *log.Logger // Optional logger for debug information

	// Channel handlers for different SPICE channels
	main     *ChMain      // Main channel for connection management
	playback *ChPlayback  // Audio playback channel
	record   *ChRecord    // Audio recording channel
	webdav   *SpiceWebdav // WebDAV channel for file transfers

	// Media time synchronization
	mmTime  uint32       // Media time in milliseconds from server
	mmStamp time.Time    // Local timestamp when mmTime was received
	mmLock  sync.RWMutex // Lock for media time access
}

// New creates a new SPICE client and establishes connection to all available channels
// It requires a Connector for network access, a Driver for GUI interaction,
// and the password for SPICE authentication
func New(c Connector, driver Driver, password string) (*Client, error) {
	cl := &Client{c: c, driver: driver, password: password}

	// First establish the main channel connection
	err := cl.setupMain()
	if err != nil {
		return nil, err
	}

	// Connect to all available channels in parallel
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
				cl.webdav, _ = cl.setupWebdav(id)
			}(ch.id)
		case ChannelUsbRedir:
			log.Printf("spice: USB supported, device #%d", ch.id)
			// Do nothing - USB support is not yet implemented
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

// GetFileTransfer returns the WebDAV file transfer interface if available
func (client *Client) GetFileTransfer() *SpiceWebdav {
	return client.webdav
}
