package spice

import (
	"encoding/binary"
	"encoding/hex"
	"log"

	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
)

const (
	SPICE_MSG_PLAYBACK_DATA    = 101
	SPICE_MSG_PLAYBACK_MODE    = 102
	SPICE_MSG_PLAYBACK_START   = 103
	SPICE_MSG_PLAYBACK_STOP    = 104
	SPICE_MSG_PLAYBACK_VOLUME  = 105
	SPICE_MSG_PLAYBACK_MUTE    = 106
	SPICE_MSG_PLAYBACK_LATENCY = 107

	SPICE_AUDIO_DATA_MODE_RAW        = 1
	SPICE_AUDIO_DATA_MODE_CELT_0_5_1 = 2
	SPICE_AUDIO_DATA_MODE_OPUS       = 3

	// need to send capabilities
	SPICE_PLAYBACK_CAP_CELT_0_5_1 = 0
	SPICE_PLAYBACK_CAP_VOLUME     = 1
	SPICE_PLAYBACK_CAP_LATENCY    = 2
	SPICE_PLAYBACK_CAP_OPUS       = 3
)

type ChPlayback struct {
	cl   *Client
	conn *SpiceConn

	mode     uint16 // 1=raw 3=opus
	channels uint32
	format   uint16
	freq     uint32
	stream   *portaudio.Stream
	buf      []int16 // default to 16bits sound
	dec      *opus.Decoder
	w        *timeBuffer

	mute bool
}

func (cl *Client) setupPlayback(id uint8) (*ChPlayback, error) {
	conn, err := cl.conn(ChannelPlayback, id, caps(SPICE_PLAYBACK_CAP_VOLUME, SPICE_PLAYBACK_CAP_OPUS))
	if err != nil {
		return nil, err
	}
	m := &ChPlayback{cl: cl, conn: conn, mute: false}
	conn.hndlr = m.handle

	go m.conn.ReadLoop()

	return m, nil
}

func (d *ChPlayback) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_MSG_PLAYBACK_DATA:
		// uint32 time
		// uint8 data[] @as_ptr(data_size);
		if d.mute {
			return
		}
		if len(data) < 4 {
			return
		}
		if d.stream == nil {
			// audio output is not ready
			return
		}
		tim := binary.LittleEndian.Uint32(data[:4])
		data = data[4:]
		switch d.mode {
		case SPICE_AUDIO_DATA_MODE_RAW:
			buf := make([]int16, len(data)/2)
			for i := 0; i < len(buf); i++ {
				buf[i] = int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
			}
			d.w.Append(tim, buf)
		case SPICE_AUDIO_DATA_MODE_OPUS:
			// decode data
			// it looks like we are always getting 10ms audio data at a time, but I don't know if that's reliable
			frameSize := d.channels * 10 * d.freq / 1000
			pcm := make([]int16, int(frameSize))
			n, err := d.dec.Decode(data, pcm)
			if err != nil {
				log.Printf("spice/playback: failed to decode Opus data: %s", err)
				return
			}

			pcm = pcm[:n*int(d.channels)]
			// send
			d.w.Append(tim, pcm)
		}
	case SPICE_MSG_PLAYBACK_MODE:
		// initialize mode
		// 00000000  05 2b 30 82 03 00                                 |.+0...|
		if len(data) < 6 {
			log.Printf("spice/playback: SPICE_MSG_PLAYBACK_MODE data truncated")
			return
		}
		tim := binary.LittleEndian.Uint32(data[:4])
		d.mode = binary.LittleEndian.Uint16(data[4:6])
		log.Printf("spice/playback: time=%d mode=%d %s", tim, d.mode, hex.EncodeToString(data))
	case SPICE_MSG_PLAYBACK_START:
		// uint32 channels, uint16 fmt=S16 uinf32 freq, uint32 time
		if len(data) < 14 {
			log.Printf("playback start: truncated channel, giving up")
			return
		}
		channels := binary.LittleEndian.Uint32(data[:4])
		format := binary.LittleEndian.Uint16(data[4:6])
		freq := binary.LittleEndian.Uint32(data[6:10])
		tim := binary.LittleEndian.Uint32(data[10:14])

		log.Printf("spice/playback: audio start channels=%d format=%d freq=%d time=%d", channels, format, freq, tim)

		if channels == d.channels && format == d.format && freq == d.freq {
			// no change
			return
		}

		if format != 1 {
			log.Printf("spice/playback: unsupported audio format %d, only supported is 1=S16", format)
			return
		}

		if d.stream != nil {
			d.stream.Abort()
			d.stream.Close()
			d.stream = nil
		}

		d.buf = make([]int16, 10*channels*freq/1000) // 48000kHz 2channels = 10*2*48000/1000 = 480
		stream, err := portaudio.OpenDefaultStream(0, int(channels), float64(freq), len(d.buf)/int(channels), &d.buf)
		if err != nil {
			log.Printf("spice/playback: failed to initialize output: %s", err)
			return
		}

		d.stream = stream

		// store info
		d.channels = channels
		d.format = format
		d.freq = freq
		d.w = NewTimeBuffer(d.cl, d)

		d.stream.Start()

		switch d.mode {
		case SPICE_AUDIO_DATA_MODE_OPUS:
			// initialize decoder
			d.dec, err = opus.NewDecoder(int(d.freq), int(d.channels))
			if err != nil {
				log.Printf("spice/playback: failed to initializa opus decoder: %s", err)
			}
		}
	case SPICE_MSG_PLAYBACK_STOP:
		// don't care
	case SPICE_MSG_PLAYBACK_VOLUME:
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
		log.Printf("spice/playback: volume information: %v", vol)
	case SPICE_MSG_PLAYBACK_MUTE:
		// uint8 mute
		if len(data) < 1 {
			return
		}
		log.Printf("spice/playback: mute information: %d", data[0])
	default:
		log.Printf("spice/playback: got message type=%d", typ)
	}
}
