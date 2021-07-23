package spice

import "fmt"

type Ropd uint16

const (
	SpiceRopdInversSrc Ropd = 1 << iota
	SpiceRopdInversBrush
	SpiceRopdInversDest
	SpiceRopdOpPut
	SpiceRopdOpOr
	SpiceRopdOpAnd
	SpiceRopdOpXor
	SpiceRopdOpBlackness
	SpiceRopdOpWhiteness
	SpiceRopdOpInvers
	SpiceRopdInversRes
)

type Channel uint8

const (
	ChannelMain Channel = iota + 1
	ChannelDisplay
	ChannelInputs
	ChannelCursor
	ChannelPlayback
	ChannelRecord
	ChannelTunnel // obsolete
	ChannelSmartcard
	ChannelUsbRedir
	ChannelPort
	ChannelWebdav
)

func (c Channel) String() string {
	switch c {
	case ChannelMain:
		return "Main"
	case ChannelDisplay:
		return "Display"
	case ChannelInputs:
		return "Inputs"
	case ChannelCursor:
		return "Cursor"
	case ChannelPlayback:
		return "Playback"
	case ChannelRecord:
		return "Record"
	case ChannelTunnel:
		return "Tunnel"
	case ChannelSmartcard:
		return "Smartcard"
	case ChannelUsbRedir:
		return "UsbRedir"
	case ChannelPort:
		return "Port"
	case ChannelWebdav:
		return "Webdav"
	default:
		return fmt.Sprintf("Channel#%d", c)
	}
}

type ImageScaleMode uint8

const (
	ImageScaleModeInterpolate ImageScaleMode = iota
	ImageScaleModeNearest
)

type spicePacket struct {
	typ  uint16
	data []interface{}
}
