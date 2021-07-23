package spice

import (
	"encoding/binary"
	"log"
)

const (
	SPICE_MSGC_INPUTS_KEY_DOWN      = 101
	SPICE_MSGC_INPUTS_KEY_UP        = 102
	SPICE_MSGC_INPUTS_KEY_MODIFIERS = 103

	SPICE_MSGC_INPUTS_MOUSE_MOTION   = 111
	SPICE_MSGC_INPUTS_MOUSE_POSITION = 112
	SPICE_MSGC_INPUTS_MOUSE_PRESS    = 113
	SPICE_MSGC_INPUTS_MOUSE_RELEASE  = 114

	SPICE_MSG_INPUTS_INIT          = 101
	SPICE_MSG_INPUTS_KEY_MODIFIERS = 102

	SPICE_MSG_INPUTS_MOUSE_MOTION_ACK = 111

	// Keyboard led bits
	SPICE_SCROLL_LOCK_MODIFIER = 1
	SPICE_NUM_LOCK_MODIFIER    = 2
	SPICE_CAPS_LOCK_MODIFIER   = 4

	SPICE_INPUTS_CAP_KEY_SCANCODE = 0
)

type ChInputs struct {
	cl   *Client
	conn *SpiceConn

	// btn state
	btn uint16
}

func (cl *Client) setupInputs(id uint8) (*ChInputs, error) {
	conn, err := cl.conn(ChannelInputs, id, nil) //caps(SPICE_INPUTS_CAP_KEY_SCANCODE))
	if err != nil {
		return nil, err
	}

	input := &ChInputs{cl: cl, conn: conn}
	conn.hndlr = input.handle
	go conn.ReadLoop()

	// reset keyboard leds
	input.conn.WriteMessage(SPICE_MSGC_INPUTS_KEY_MODIFIERS, uint16(0))

	cl.driver.SetEventsTarget(input)

	return input, nil
}

func (input *ChInputs) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_MSG_INPUTS_INIT:
		// Note: spice documentation is wrong, this is 16bits and not 32bits
		keyMod := binary.LittleEndian.Uint16(data)
		log.Printf("spice/inputs: got key modifier status from server, value = %d (initial)", keyMod)
	case SPICE_MSG_INPUTS_KEY_MODIFIERS:
		keyMod := binary.LittleEndian.Uint16(data)
		log.Printf("spice/inputs: got key modifier status from server, value = %d", keyMod)
	case SPICE_MSG_INPUTS_MOUSE_MOTION_ACK:
		// do nothing
	default:
		log.Printf("spice/inputs: got message type=%d", typ)
	}
}

func (input *ChInputs) OnKeyDown(k []byte) {
	scancode := make([]byte, 4)
	copy(scancode, k)

	input.conn.WriteMessage(SPICE_MSGC_INPUTS_KEY_DOWN, scancode)
}

func (input *ChInputs) OnKeyUp(k []byte) {
	scancode := make([]byte, 4)
	copy(scancode, k)

	// AT scancode: insert 0xF0 before last byte
	// XT scancode: set top bit of last part
	ln := len(k)
	scancode[ln-1] |= 0x80

	//log.Printf("spice: sending key up %s as %+v", ev.Name, scancode)

	input.conn.WriteMessage(SPICE_MSGC_INPUTS_KEY_UP, scancode)
}

func (input *ChInputs) MousePosition(x, y uint32) {
	var displayID uint8

	err := input.conn.WriteMessage(SPICE_MSGC_INPUTS_MOUSE_POSITION, x, y, input.btn, displayID)
	if err != nil {
		log.Printf("Failed to send mouse position: %s", err)
	}
}

func (input *ChInputs) MouseDown(btn uint8, x, y uint32) {
	state := uint16(1) << btn

	if input.btn&state == state {
		log.Printf("ignoring btn down %d", btn)
		return // already pressed
	}

	input.btn |= state

	input.conn.WriteMessage(SPICE_MSGC_INPUTS_MOUSE_PRESS, btn, input.btn)
}

func (input *ChInputs) MouseUp(btn uint8, x, y uint32) {
	state := uint16(1) << btn

	if input.btn&state == 0 {
		log.Printf("ignoring btn up %d", btn)
		return // already released
	}

	input.btn &= ^state

	input.conn.WriteMessage(SPICE_MSGC_INPUTS_MOUSE_RELEASE, btn, input.btn)
}
