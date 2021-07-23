package spice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SPICE_MSG_MAIN_MIGRATE_BEGIN    = 101
	SPICE_MSG_MAIN_MIGRATE_CANCEL   = 102
	SPICE_MSG_MAIN_INIT             = 103
	SPICE_MSG_MAIN_CHANNELS_LIST    = 104
	SPICE_MSG_MAIN_MOUSE_MODE       = 105
	SPICE_MSG_MAIN_MULTI_MEDIA_TIME = 106

	SPICE_MSG_MAIN_AGENT_CONNECTED    = 107
	SPICE_MSG_MAIN_AGENT_DISCONNECTED = 108
	SPICE_MSG_MAIN_AGENT_DATA         = 109
	SPICE_MSG_MAIN_AGENT_TOKEN        = 110

	SPICE_MSG_MAIN_MIGRATE_SWITCH_HOST       = 111
	SPICE_MSG_MAIN_MIGRATE_END               = 112
	SPICE_MSG_MAIN_NAME                      = 113
	SPICE_MSG_MAIN_UUID                      = 114
	SPICE_MSG_MAIN_AGENT_CONNECTED_TOKENS    = 115
	SPICE_MSG_MAIN_MIGRATE_BEGIN_SEAMLESS    = 116
	SPICE_MSG_MAIN_MIGRATE_DST_SEAMLESS_ACK  = 117
	SPICE_MSG_MAIN_MIGRATE_DST_SEAMLESS_NACK = 118

	SPICE_MSGC_MAIN_CLIENT_INFO           = 101
	SPICE_MSGC_MAIN_MIGRATE_CONNECTED     = 102
	SPICE_MSGC_MAIN_MIGRATE_CONNECT_ERROR = 103
	SPICE_MSGC_MAIN_ATTACH_CHANNELS       = 104
	SPICE_MSGC_MAIN_MOUSE_MODE_REQUEST    = 105

	SPICE_MSGC_MAIN_AGENT_START = 106
	SPICE_MSGC_MAIN_AGENT_DATA  = 107
	SPICE_MSGC_MAIN_AGENT_TOKEN = 108

	VD_AGENT_PROTOCOL = 1
)

const (
	SPICE_MAIN_CAP_SEMI_SEAMLESS_MIGRATE = iota
	SPICE_MAIN_CAP_NAME_AND_UUID
	SPICE_MAIN_CAP_AGENT_CONNECTED_TOKENS
	SPICE_MAIN_CAP_SEAMLESS_MIGRATE
)

const (
	VD_AGENT_MOUSE_STATE = iota + 1
	VD_AGENT_MONITORS_CONFIG
	VD_AGENT_REPLY
	VD_AGENT_CLIPBOARD
	VD_AGENT_DISPLAY_CONFIG
	VD_AGENT_ANNOUNCE_CAPABILITIES
	VD_AGENT_CLIPBOARD_GRAB
	VD_AGENT_CLIPBOARD_REQUEST
	VD_AGENT_CLIPBOARD_RELEASE
	VD_AGENT_FILE_XFER_START
	VD_AGENT_FILE_XFER_STATUS
	VD_AGENT_FILE_XFER_DATA
	VD_AGENT_CLIENT_DISCONNECTED
	VD_AGENT_MAX_CLIPBOARD
	VD_AGENT_AUDIO_VOLUME_SYNC
	VD_AGENT_GRAPHICS_DEVICE_INFO
)

const (
	VD_AGENT_FILE_XFER_STATUS_CAN_SEND_DATA = iota
	VD_AGENT_FILE_XFER_STATUS_CANCELLED
	VD_AGENT_FILE_XFER_STATUS_ERROR
	VD_AGENT_FILE_XFER_STATUS_SUCCESS
	VD_AGENT_FILE_XFER_STATUS_NOT_ENOUGH_SPACE
	VD_AGENT_FILE_XFER_STATUS_SESSION_LOCKED
	VD_AGENT_FILE_XFER_STATUS_VDAGENT_NOT_CONNECTED
	VD_AGENT_FILE_XFER_STATUS_DISABLED
)

const (
	// caps from server: 36327 =   1000110111100111
	// caps from server (win32): 1719 = 11010110111
	VD_AGENT_CAP_MOUSE_STATE               uint32 = iota // X
	VD_AGENT_CAP_MONITORS_CONFIG                         // X
	VD_AGENT_CAP_REPLY                                   // X
	VD_AGENT_CAP_CLIPBOARD                               // (unused)
	VD_AGENT_CAP_DISPLAY_CONFIG                          // (unused) win only
	VD_AGENT_CAP_CLIPBOARD_BY_DEMAND                     // X
	VD_AGENT_CAP_CLIPBOARD_SELECTION                     // X linux only
	VD_AGENT_CAP_SPARSE_MONITORS_CONFIG                  // X
	VD_AGENT_CAP_GUEST_LINEEND_LF                        // X
	VD_AGENT_CAP_GUEST_LINEEND_CRLF                      // (windows only?)
	VD_AGENT_CAP_MAX_CLIPBOARD                           // X
	VD_AGENT_CAP_AUDIO_VOLUME_SYNC                       // X
	VD_AGENT_CAP_MONITORS_CONFIG_POSITION                // (unused)
	VD_AGENT_CAP_FILE_XFER_DISABLED                      // (unused)
	VD_AGENT_CAP_FILE_XFER_DETAILED_ERRORS               // (unused)
	VD_AGENT_CAP_GRAPHICS_DEVICE_INFO                    // X
	VD_AGENT_CAP_CLIPBOARD_NO_RELEASE_ON_REGRAB
	VD_AGENT_CAP_CLIPBOARD_GRAB_SERIAL
)

type SpiceClipboardSelection uint8

const (
	VD_AGENT_CLIPBOARD_SELECTION_CLIPBOARD SpiceClipboardSelection = iota
	VD_AGENT_CLIPBOARD_SELECTION_PRIMARY
	VD_AGENT_CLIPBOARD_SELECTION_SECONDARY
)

type SpiceClipboardFormat uint32

const (
	VD_AGENT_CLIPBOARD_NONE SpiceClipboardFormat = iota
	VD_AGENT_CLIPBOARD_UTF8_TEXT
	VD_AGENT_CLIPBOARD_IMAGE_PNG
	VD_AGENT_CLIPBOARD_IMAGE_BMP
	VD_AGENT_CLIPBOARD_IMAGE_TIFF
	VD_AGENT_CLIPBOARD_IMAGE_JPG
)

const VD_AGENT_SERVER_TOKEN_AMOUNT = 10
const VD_AGENT_MAX_DATA_SIZE = 2048

type SpiceChannelInfo struct {
	typ Channel
	id  uint8
}

type SpiceMonitor struct {
	Height uint32
	Width  uint32
	Depth  uint32
	X      uint32
	Y      uint32
}

// ClipboardData identifies one specific request
type ClipboardData struct {
	selection  SpiceClipboardSelection
	formatType SpiceClipboardFormat
	data       []byte
}

type ChMain struct {
	cl    *Client
	conn  *SpiceConn
	ready chan struct{}
	rOnce sync.Once

	mouseModes   uint32 // available mouse modes mask
	mouseMode    uint32
	agent        uint32 // 1 if agent
	agentTokens  uint32 // agent tokens count
	ramHint      uint32 // hint for ram
	serverTokens uint32 // server tokens count
	agentCaps    uint32 // caps returned by agent (1719 on windows)

	channels []SpiceChannelInfo

	// for vdagent
	vdq [][]byte // vd queue
	vdl sync.Mutex
	vdc *sync.Cond
	vdb []byte // read buffer

	// clipboard (remote→local)
	clipboardCh chan *ClipboardData
	clipboardLk sync.Mutex
}

func (cl *Client) setupMain() error {
	m := &ChMain{cl: cl, ready: make(chan struct{}), serverTokens: VD_AGENT_SERVER_TOKEN_AMOUNT}

	// establish connection to main channel
	conn, err := cl.conn(ChannelMain, 0, caps(SPICE_MAIN_CAP_AGENT_CONNECTED_TOKENS))
	if err != nil {
		return err
	}

	m.conn = conn
	conn.hndlr = m.handle
	cl.main = m
	cl.driver.SetMainTarget(m)
	go m.conn.ReadLoop()
	go m.vdQueue()

	<-m.ready
	return nil
}

func (m *ChMain) handle(typ uint16, data []byte) {
	// handler

	switch typ {
	case SPICE_MSG_MAIN_INIT:
		// this is a initial msg sent from main

		var agentTokens, mmTime uint32
		now := time.Now()

		buf := bytes.NewReader(data)
		binary.Read(buf, binary.LittleEndian, &m.cl.session)
		binary.Read(buf, binary.LittleEndian, &m.cl.displays)
		binary.Read(buf, binary.LittleEndian, &m.mouseModes)
		binary.Read(buf, binary.LittleEndian, &m.mouseMode)
		binary.Read(buf, binary.LittleEndian, &m.agent)
		binary.Read(buf, binary.LittleEndian, &agentTokens)
		binary.Read(buf, binary.LittleEndian, &mmTime)
		binary.Read(buf, binary.LittleEndian, &m.ramHint)

		atomic.StoreUint32(&m.agentTokens, agentTokens)

		log.Printf("spice/main: got MAIN_INIT: sessionID=%d displays=%d mouseModes=%d mouseMode=%d agent=%d agentTokens=%d mmTime=%d ramHint=%d", m.cl.session, m.cl.displays, m.mouseModes, m.mouseMode, m.agent, m.agentTokens, mmTime, m.ramHint)

		m.cl.mmLock.Lock()
		m.cl.mmTime = mmTime
		m.cl.mmStamp = now
		m.cl.mmLock.Unlock()

		if m.mouseModes&SPICE_MOUSE_MODE_CLIENT == SPICE_MOUSE_MODE_CLIENT && m.mouseMode != SPICE_MOUSE_MODE_CLIENT {
			// set mouse mode to client
			m.MouseModeRequest(SPICE_MOUSE_MODE_CLIENT)
		}

		// send SPICE_MSGC_MAIN_ATTACH_CHANNELS to receive channels list
		m.conn.WriteMessage(SPICE_MSGC_MAIN_ATTACH_CHANNELS)

		// if agent, initialize

		if m.agent != 0 {
			m.agentInit()
		}

	case SPICE_MSG_MAIN_CHANNELS_LIST:
		// got the list of channels
		buf := bytes.NewReader(data)
		var cnt uint32
		binary.Read(buf, binary.LittleEndian, &cnt)

		var list []SpiceChannelInfo

		for i := uint32(0); i < cnt; i++ {
			var typ Channel
			var id uint8
			binary.Read(buf, binary.LittleEndian, &typ)
			binary.Read(buf, binary.LittleEndian, &id)
			//log.Printf("spice/main: found spice channel type=%s id=%d", typ, id)
			list = append(list, SpiceChannelInfo{typ: typ, id: id})
		}

		m.channels = list

		m.rOnce.Do(func() { close(m.ready) })
	case SPICE_MSG_MAIN_MOUSE_MODE:
		if len(data) < 4 {
			return
		}
		// set mouse mode
		supported := binary.LittleEndian.Uint16(data[:2])
		current := binary.LittleEndian.Uint16(data[2:4])

		m.mouseMode = uint32(current)
		m.mouseModes = uint32(supported)

		if m.mouseModes&SPICE_MOUSE_MODE_CLIENT == SPICE_MOUSE_MODE_CLIENT && m.mouseMode != SPICE_MOUSE_MODE_CLIENT {
			// set mouse mode to client
			m.MouseModeRequest(SPICE_MOUSE_MODE_CLIENT)
		}

		log.Printf("spice/main: mouse mode set to %d out of %d", current, supported)
	case SPICE_MSG_MAIN_MULTI_MEDIA_TIME:
		if len(data) != 4 {
			return
		}
		mmTime := binary.LittleEndian.Uint32(data[:4])
		now := time.Now()
		log.Printf("spice/main: received multimedia time update: %d", mmTime)
		m.cl.mmLock.Lock()
		m.cl.mmTime = mmTime
		m.cl.mmStamp = now
		m.cl.mmLock.Unlock()
	case SPICE_MSG_MAIN_AGENT_CONNECTED:
		m.agent = 1
		m.agentInit()
	case SPICE_MSG_MAIN_AGENT_DISCONNECTED:
		m.agent = 0
	case SPICE_MSG_MAIN_AGENT_DATA:
		m.agentHandler(data)
	case SPICE_MSG_MAIN_AGENT_TOKEN:
		if len(data) != 4 {
			return
		}
		m.updateAgentToken(binary.LittleEndian.Uint32(data[:4]))
	default:
		log.Printf("spice/main: got message type=%d", typ)
	}
}

func (m *ChMain) updateAgentToken(amount uint32) {
	atomic.AddUint32(&m.agentTokens, amount)
	m.vdc.Signal()
}

func (m *ChMain) MouseModeRequest(mod uint32) error {
	// mode value
	// note: client mode is likely the best :)
	buf := make([]byte, 4)

	binary.LittleEndian.PutUint32(buf, mod)
	return m.conn.WriteMessage(SPICE_MSGC_MAIN_MOUSE_MODE_REQUEST, buf)
}

func (m *ChMain) agentInit() error {
	log.Printf("spice/main: attempting to initate agent link")
	// trigger connection to agent
	m.conn.WriteMessage(SPICE_MSGC_MAIN_AGENT_START, uint32(VD_AGENT_SERVER_TOKEN_AMOUNT))

	// send agent announce caps
	return m.AgentWrite(
		VD_AGENT_ANNOUNCE_CAPABILITIES,
		uint32(1),
		caps(
			//VD_AGENT_CAP_MOUSE_STATE,
			VD_AGENT_CAP_MONITORS_CONFIG,
			//VD_AGENT_CAP_REPLY,
			VD_AGENT_CAP_CLIPBOARD_BY_DEMAND,
			VD_AGENT_CAP_CLIPBOARD_SELECTION,
			VD_AGENT_CAP_CLIPBOARD_GRAB_SERIAL,
		),
	)
}

func (m *ChMain) MonitorConfig(flags uint32, mons []SpiceMonitor) error {
	return m.AgentWrite(
		VD_AGENT_MONITORS_CONFIG,
		uint32(len(mons)),
		flags,
		mons,
	)
}

func (m *ChMain) DisplayConfig(flags, depth uint32) error {
	// Flags: disable_wallpaper, disable_font_smooth, disable_animation, set_color_depth
	return m.AgentWrite(
		VD_AGENT_DISPLAY_CONFIG,
		flags,
		depth,
	)
}

func (m *ChMain) AgentWrite(typ uint32, data ...interface{}) error {
	buf := make([]byte, 20) // uint32(VD_AGENT_PROTOCOL), uint32(typ), uint64(opaque), uint32(len(buf)-16)
	var opaque uint64

	// generate buffer
	for _, d := range data {
		switch v := d.(type) {
		case []byte:
			if buf == nil {
				buf = v
			} else {
				buf = append(buf, v...)
			}
		case uint64:
			if len(data) == 1 {
				opaque = v
				break
			}
			tmp := &bytes.Buffer{}
			binary.Write(tmp, binary.LittleEndian, d)
			if buf == nil {
				buf = tmp.Bytes()
			} else {
				buf = append(buf, tmp.Bytes()...)
			}
		default:
			tmp := &bytes.Buffer{}
			binary.Write(tmp, binary.LittleEndian, d)
			if buf == nil {
				buf = tmp.Bytes()
			} else {
				buf = append(buf, tmp.Bytes()...)
			}
		}
	}

	// fill buf values
	binary.LittleEndian.PutUint32(buf[:4], VD_AGENT_PROTOCOL)
	binary.LittleEndian.PutUint32(buf[4:8], typ)
	binary.LittleEndian.PutUint64(buf[8:16], opaque)
	binary.LittleEndian.PutUint32(buf[16:20], uint32(len(buf)-20))

	// queue buffer for sending & poke sending
	m.vdl.Lock()
	defer m.vdl.Unlock()

	m.vdq = append(m.vdq, buf)
	m.vdc.Signal()
	return nil
}

func (m *ChMain) SendGrabClipboard(selection SpiceClipboardSelection, formatTypes []SpiceClipboardFormat) error {
	if len(formatTypes) == 0 {
		// TODO: send clipboard clear
		return nil
	}

	buf := &bytes.Buffer{}

	if testCap(m.agentCaps, VD_AGENT_CAP_CLIPBOARD_SELECTION) {
		buf.Write([]byte{uint8(selection), 0, 0, 0}) // uint8_t __reserved[sizeof(uint32_t) - 1 * sizeof(uint8_t)]
	} else if selection != VD_AGENT_CLIPBOARD_SELECTION_CLIPBOARD {
		// ignore this because remote only supports Default
		return nil
	}

	log.Printf("spice/main: send grab clipboard command with types %v", formatTypes)

	for _, fmt := range formatTypes {
		binary.Write(buf, binary.LittleEndian, fmt)
	}

	return m.AgentWrite(
		VD_AGENT_CLIPBOARD_GRAB,
		buf.Bytes(),
	)
}

func (m *ChMain) RequestClipboard(selection SpiceClipboardSelection, clipboardType SpiceClipboardFormat) ([]byte, error) {
	log.Printf("spice/main: send request clipboard command with type %d", clipboardType)

	m.clipboardLk.Lock()
	defer m.clipboardLk.Unlock()

	ch := make(chan *ClipboardData, 2)
	m.clipboardCh = ch

	var err error
	if testCap(m.agentCaps, VD_AGENT_CAP_CLIPBOARD_SELECTION) {
		err = m.AgentWrite(VD_AGENT_CLIPBOARD_REQUEST, uint8(selection), uint8(0), uint8(0), uint8(0), uint32(clipboardType))
	} else {
		err = m.AgentWrite(VD_AGENT_CLIPBOARD_REQUEST, uint32(clipboardType))
	}

	if err != nil {
		return nil, err
	}

	// 5secs timeout on read
	t := time.NewTimer(5 * time.Second)

	select {
	case res := <-ch:
		//log.Printf("spice/main: got clipboard data, len=%d", len(res.data))
		return res.data, nil
	case <-t.C:
		return nil, errors.New("timeout while reading")
	}
}

func (m *ChMain) handleIncomingClipboard(selection SpiceClipboardSelection, typ SpiceClipboardFormat, data []byte) {
	//log.Printf("received clipboard %d/%d len=%d", selection, typ, len(data))
	obj := &ClipboardData{
		selection:  selection,
		formatType: typ,
		data:       data,
	}

	select {
	case m.clipboardCh <- obj:
	default:
		// do not lock
	}
}

func (m *ChMain) sendClipboard(selection SpiceClipboardSelection, formatType SpiceClipboardFormat, data []byte) error {
	tmp := &bytes.Buffer{}

	// write selection
	if testCap(m.agentCaps, VD_AGENT_CAP_CLIPBOARD_SELECTION) {
		tmp.Write([]byte{uint8(formatType), 0, 0, 0}) // uint8_t selection + uint8_t __reserved[sizeof(uint32_t) - 1 * sizeof(uint8_t)]
	}

	// write type
	binary.Write(tmp, binary.LittleEndian, uint32(formatType))

	// write data
	tmp.Write(data)

	log.Printf("spice/main: send requested clipboard data with type %d (%d bytes)", formatType, len(data))
	return m.AgentWrite(VD_AGENT_CLIPBOARD, tmp.Bytes())
}

func (m *ChMain) agentHandler(data []byte) {
	// we received some data
	if len(data) < 20 {
		log.Printf("spice/main: dropping truncated message from agent")
		return
	}

	if m.vdb != nil {
		data = append(m.vdb, data...)
		m.vdb = nil
	}

	m.serverTokens--
	if m.serverTokens == 0 {
		log.Println("spice/main: server token pool is empty, send more tokens.")
		m.conn.WriteMessage(SPICE_MSGC_MAIN_AGENT_TOKEN, uint32(VD_AGENT_SERVER_TOKEN_AMOUNT))
		m.serverTokens += 10
	}

	proto := binary.LittleEndian.Uint32(data[:4])
	typ := binary.LittleEndian.Uint32(data[4:8])
	opaque := binary.LittleEndian.Uint64(data[8:16])
	size := binary.LittleEndian.Uint32(data[16:20])
	if proto != VD_AGENT_PROTOCOL {
		log.Printf("spice/main: dropping unknown protocol %d message from agent", proto)
		return
	}
	if len(data) < int(size) {
		log.Printf("spice/main: waiting for more data from agent...")
		m.vdb = data
		return
	}

	data = data[20:]
	data = data[:size] // just in case

	switch typ {
	case VD_AGENT_ANNOUNCE_CAPABILITIES:
		data = data[4:] // skip uint32_t  request - should be zero
		// read capabilities!
		cnt := len(data) / 4
		c := make([]uint32, cnt)
		for i := 0; i < cnt; i++ {
			c[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
		}
		if len(c) > 0 {
			m.agentCaps = c[0]
		}
		//log.Printf("DATA = %s", hex.Dump(data))
		log.Printf("spice/main: received capabilities from agent: %v", c)
	case VD_AGENT_CLIPBOARD:
		// got clipboard
		selection := VD_AGENT_CLIPBOARD_SELECTION_CLIPBOARD
		if testCap(m.agentCaps, VD_AGENT_CAP_CLIPBOARD_SELECTION) {
			selection = SpiceClipboardSelection(data[0])
			data = data[4:] // uint8_t __reserved[sizeof(uint32_t) - 1 * sizeof(uint8_t)];
		}
		typ := SpiceClipboardFormat(binary.LittleEndian.Uint32(data[:4]))
		data = data[4:]
		m.handleIncomingClipboard(selection, typ, data)
	case VD_AGENT_CLIPBOARD_GRAB: // remote is claiming ownership on the clipboard
		selection := VD_AGENT_CLIPBOARD_SELECTION_CLIPBOARD
		if testCap(m.agentCaps, VD_AGENT_CAP_CLIPBOARD_SELECTION) {
			selection = SpiceClipboardSelection(data[0])
			data = data[4:] // uint8_t __reserved[sizeof(uint32_t) - 1 * sizeof(uint8_t)];
		}
		cnt := len(data) / 4
		c := make([]SpiceClipboardFormat, cnt)
		for i := 0; i < cnt; i++ {
			c[i] = SpiceClipboardFormat(binary.LittleEndian.Uint32(data[i*4 : i*4+4]))
		}
		// TODO this should call the handler and inform of the available types, choosing a type should be up to the handler
		m.cl.driver.ClipboardGrabbed(selection, c)
	case VD_AGENT_CLIPBOARD_REQUEST:
		// send our clipboard
		selection := VD_AGENT_CLIPBOARD_SELECTION_CLIPBOARD
		if testCap(m.agentCaps, VD_AGENT_CAP_CLIPBOARD_SELECTION) {
			selection = SpiceClipboardSelection(data[0])
			data = data[4:]
		}
		typ := SpiceClipboardFormat(binary.LittleEndian.Uint32(data[:4]))
		log.Printf("spice/main: fetching clipboard %d/%d", selection, typ)
		data, err := m.cl.driver.ClipboardFetch(selection, typ)
		if err != nil {
			log.Printf("spice/main: failed to fetch clipboard: %s", err)
		} else {
			// send clipboard data
			m.sendClipboard(selection, typ, data)
		}
	case VD_AGENT_CLIPBOARD_RELEASE:
		// release when clipboard is empty
		selection := VD_AGENT_CLIPBOARD_SELECTION_CLIPBOARD
		if testCap(m.agentCaps, VD_AGENT_CAP_CLIPBOARD_SELECTION) {
			selection = SpiceClipboardSelection(data[0])
		}
		m.cl.driver.ClipboardRelease(selection)
	default:
		log.Printf("spice/main: unhandled packet type=%d opaque=%d size=%d from agent", typ, opaque, size)
	}
}

func (m *ChMain) vdQueue() {
	m.vdc = sync.NewCond(&m.vdl)
	m.vdl.Lock()

	for {
		if len(m.vdq) == 0 {
			m.vdc.Wait()
			continue
		}

		if atomic.LoadUint32(&m.agentTokens) == 0 {
			// not enough tokens, wait
			m.vdc.Wait()
			continue
		}

		// send one buf
		buf := m.vdq[0]
		if len(buf) <= VD_AGENT_MAX_DATA_SIZE {
			// skip to next item
			m.vdq = m.vdq[1:]
		} else {
			// cut
			buf = buf[:VD_AGENT_MAX_DATA_SIZE]
			m.vdq[0] = m.vdq[0][VD_AGENT_MAX_DATA_SIZE:]
		}

		// write buf
		m.conn.WriteMessage(
			SPICE_MSGC_MAIN_AGENT_DATA,
			buf,
		)
	}
}
