package spice

import (
	"encoding/binary"
	"log"
	"sync/atomic"

	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
)

type ChRecord struct {
	cl   *Client
	conn *SpiceConn

	mode     uint16 // 1=raw 3=opus
	channels uint32
	format   uint16
	freq     uint32
	stream   *portaudio.Stream
	pcm      []int16 // default to 16bits sound
	enc      *opus.Encoder
	run      uint32
}

const (
	SPICE_MSG_RECORD_START  = 101
	SPICE_MSG_RECORD_STOP   = 102
	SPICE_MSG_RECORD_VOLUME = 103
	SPICE_MSG_RECORD_MUTE   = 104

	SPICE_MSGC_RECORD_DATA       = 101
	SPICE_MSGC_RECORD_MODE       = 102
	SPICE_MSGC_RECORD_START_MARK = 103

	SPICE_RECORD_CAP_CELT_0_5_1 = 0
	SPICE_RECORD_CAP_VOLUME     = 1
	SPICE_RECORD_CAP_OPUS       = 2
)

func (cl *Client) setupRecord(id uint8) (*ChRecord, error) {
	conn, err := cl.conn(ChannelRecord, id, caps(SPICE_RECORD_CAP_VOLUME, SPICE_RECORD_CAP_OPUS))
	if err != nil {
		return nil, err
	}
	m := &ChRecord{cl: cl, conn: conn}
	conn.hndlr = m.handle

	switch {
	case testCap(conn.validCaps[0], SPICE_RECORD_CAP_OPUS):
		m.mode = SPICE_AUDIO_DATA_MODE_OPUS
	default:
		m.mode = SPICE_AUDIO_DATA_MODE_RAW
	}

	go m.conn.ReadLoop()

	return m, nil
}

func (d *ChRecord) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_MSG_RECORD_START:
		// uint32 channels, uint16 fmt=S16 uinf32 freq
		if len(data) < 10 {
			log.Printf("record start: truncated info, giving up")
			return
		}
		channels := binary.LittleEndian.Uint32(data[:4])
		format := binary.LittleEndian.Uint16(data[4:6])
		freq := binary.LittleEndian.Uint32(data[6:10])

		log.Printf("spice/record: audio start channels=%d format=%d freq=%d", channels, format, freq)

		// send mediatime
		mmtime := d.cl.MediaTime()
		d.conn.WriteMessage(SPICE_MSGC_RECORD_MODE, mmtime, d.mode)
		log.Printf("spice/record: sent mode packet, mmtime=%d mode=%d", mmtime, d.mode)

		if channels == d.channels && format == d.format && freq == d.freq {
			// no change
			return
		}

		if format != 1 {
			log.Printf("spice/record: unsupported audio format %d, only supported is 1=S16", format)
			return
		}

		if d.stream != nil {
			d.stream.Abort()
			d.stream.Close()
			d.stream = nil
		}

		d.pcm = make([]int16, 10*channels*freq/1000) // 48000kHz 2channels = 10*2*48000/1000 = 480
		stream, err := portaudio.OpenDefaultStream(int(channels), 0, float64(freq), len(d.pcm)/int(channels), &d.pcm)
		if err != nil {
			log.Printf("spice/record: failed to initialize output: %s", err)
			return
		}

		d.stream = stream

		// store info
		d.channels = channels
		d.format = format
		d.freq = freq

		d.stream.Start()
		atomic.StoreUint32(&d.run, 1)

		switch d.mode {
		case SPICE_AUDIO_DATA_MODE_OPUS:
			// initialize decoder
			d.enc, err = opus.NewEncoder(int(d.freq), int(d.channels), opus.AppVoIP) // use voice optimized encoding since microphone is most likely voice
			if err != nil {
				log.Printf("spice/record: failed to initializa opus encoder: %s", err)
				return
			}
		}
		// start read loop from stream
		go d.startRecord()
	case SPICE_MSG_RECORD_STOP:
		// TODO stop
		log.Printf("spice/record: TODO stop recording")
		atomic.StoreUint32(&d.run, 0)
	case SPICE_MSG_RECORD_VOLUME:
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
		log.Printf("spice/record: volume information: %v", vol)
	case SPICE_MSG_RECORD_MUTE:
		// uint8 mute
		if len(data) < 1 {
			return
		}
		log.Printf("spice/record: mute information: %d", data[0])
	default:
		log.Printf("spice/record: got message type=%d", typ)
	}
}

func (d *ChRecord) startRecord() {
	defer d.stream.Stop()
	buf := make([]byte, 512) // buffer for encoded data

	d.conn.WriteMessage(SPICE_MSGC_RECORD_START_MARK, d.cl.MediaTime())

	for {
		err := d.stream.Read()
		if err != nil {
			log.Printf("failed to read: %s", err)
			return
		}
		// encode d.pcm
		n, err := d.enc.Encode(d.pcm, buf)
		if err != nil {
			log.Printf("spice/record: failed PCM Opus encoding: %s", err)
			return
		}
		// send buf[:n]
		d.conn.WriteMessage(SPICE_MSGC_RECORD_DATA, d.cl.MediaTime(), buf[:n])

		if atomic.LoadUint32(&d.run) != 1 {
			return
		}
	}
}
