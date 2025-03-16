// Package spice implements a client for the SPICE remote desktop protocol
// This file implements the record channel for audio recording functionality
package spice

import (
	"encoding/binary"
	"log"
	"sync/atomic"

	"github.com/gordonklaus/portaudio"
	"github.com/hraban/opus"
)

// ChRecord handles the audio recording channel for capturing audio from the client
// and sending it to the SPICE server
type ChRecord struct {
	cl      *Client           // Reference to the parent client
	conn    *SpiceConn        // Connection to record channel

	mode     uint16           // Audio encoding mode (1=raw, 3=opus)
	channels uint32           // Number of audio channels
	format   uint16           // Audio format (1=16-bit signed PCM)
	freq     uint32           // Sample rate in Hz
	stream   *portaudio.Stream // Audio input stream
	pcm      []int16          // PCM buffer for audio data (16-bit signed)
	enc      *opus.Encoder    // Opus encoder for audio compression
	run      uint32           // Atomic flag to control recording state
}

const (
	// Messages from server to client
	SPICE_MSG_RECORD_START  = 101 // Start audio recording
	SPICE_MSG_RECORD_STOP   = 102 // Stop audio recording
	SPICE_MSG_RECORD_VOLUME = 103 // Set recording volume
	SPICE_MSG_RECORD_MUTE   = 104 // Mute/unmute recording

	// Messages from client to server
	SPICE_MSGC_RECORD_DATA       = 101 // Send recorded audio data
	SPICE_MSGC_RECORD_MODE       = 102 // Set audio recording mode
	SPICE_MSGC_RECORD_START_MARK = 103 // Mark recording start time

	// Capability flags for record channel
	SPICE_RECORD_CAP_CELT_0_5_1 = 0 // Support for CELT 0.5.1 codec (deprecated)
	SPICE_RECORD_CAP_VOLUME     = 1 // Support for volume control
	SPICE_RECORD_CAP_OPUS       = 2 // Support for Opus audio codec
)

// setupRecord establishes a connection to the record channel and initializes it
// It negotiates audio encoding capabilities with the server
func (cl *Client) setupRecord(id uint8) (*ChRecord, error) {
	// Connect to record channel with volume control and Opus codec capabilities
	conn, err := cl.conn(ChannelRecord, id, caps(SPICE_RECORD_CAP_VOLUME, SPICE_RECORD_CAP_OPUS))
	if err != nil {
		return nil, err
	}
	
	// Create record handler and set message callback
	m := &ChRecord{cl: cl, conn: conn}
	conn.hndlr = m.handle

	// Select audio encoding mode based on negotiated capabilities
	switch {
	case testCap(conn.validCaps[0], SPICE_RECORD_CAP_OPUS):
		m.mode = SPICE_AUDIO_DATA_MODE_OPUS
	default:
		m.mode = SPICE_AUDIO_DATA_MODE_RAW
	}

	// Start message processing loop in background
	go m.conn.ReadLoop()

	return m, nil
}

// handle processes incoming messages from the SPICE server for the record channel
func (d *ChRecord) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_MSG_RECORD_START:
		// Parse audio format parameters: channels, format, and frequency
		if len(data) < 10 {
			log.Printf("record start: truncated info, giving up")
			return
		}
		channels := binary.LittleEndian.Uint32(data[:4])
		format := binary.LittleEndian.Uint16(data[4:6])
		freq := binary.LittleEndian.Uint32(data[6:10])

		log.Printf("spice/record: audio start channels=%d format=%d freq=%d", channels, format, freq)

		// Send current media time and our preferred encoding mode to the server
		mmtime := d.cl.MediaTime()
		d.conn.WriteMessage(SPICE_MSGC_RECORD_MODE, mmtime, d.mode)
		log.Printf("spice/record: sent mode packet, mmtime=%d mode=%d", mmtime, d.mode)

		// If audio parameters haven't changed, no need to reconfigure
		if channels == d.channels && format == d.format && freq == d.freq {
			return
		}

		// Currently only support 16-bit signed PCM
		if format != 1 {
			log.Printf("spice/record: unsupported audio format %d, only supported is 1=S16", format)
			return
		}

		// Clean up existing audio stream if any
		if d.stream != nil {
			d.stream.Abort()
			d.stream.Close()
			d.stream = nil
		}

		// Create PCM buffer (10ms of audio data)
		d.pcm = make([]int16, 10*channels*freq/1000) // e.g., 48000Hz, 2channels = 10*2*48000/1000 = 960 samples
		
		// Open audio input stream (channels input, 0 output)
		stream, err := portaudio.OpenDefaultStream(int(channels), 0, float64(freq), len(d.pcm)/int(channels), &d.pcm)
		if err != nil {
			log.Printf("spice/record: failed to initialize input: %s", err)
			return
		}

		d.stream = stream

		// Store audio configuration
		d.channels = channels
		d.format = format
		d.freq = freq

		// Start audio capture
		d.stream.Start()
		atomic.StoreUint32(&d.run, 1)

		// Initialize audio encoder based on mode
		switch d.mode {
		case SPICE_AUDIO_DATA_MODE_OPUS:
			// Initialize Opus encoder with voice optimization for microphone input
			d.enc, err = opus.NewEncoder(int(d.freq), int(d.channels), opus.AppVoIP)
			if err != nil {
				log.Printf("spice/record: failed to initialize opus encoder: %s", err)
				return
			}
		}
		
		// Start background goroutine for capturing and sending audio data
		go d.startRecord()
		
	case SPICE_MSG_RECORD_STOP:
		// Signal recording to stop by clearing the run flag
		log.Printf("spice/record: stopping recording")
		atomic.StoreUint32(&d.run, 0)
		
	case SPICE_MSG_RECORD_VOLUME:
		// Parse volume settings for each channel
		if len(data) < 1 {
			return
		}
		cnt := int(data[0])    // Number of channels
		data = data[1:]
		if len(data) != cnt*2 {
			return // Invalid data length
		}
		
		// Extract volume level for each channel
		vol := make([]uint16, cnt)
		for n := 0; n < cnt; n++ {
			vol[n] = binary.LittleEndian.Uint16(data[:2])
			data = data[2:]
		}
		log.Printf("spice/record: volume information: %v", vol)
		
	case SPICE_MSG_RECORD_MUTE:
		// Parse mute status
		if len(data) < 1 {
			return
		}
		log.Printf("spice/record: mute information: %d", data[0])
		
	default:
		log.Printf("spice/record: got message type=%d", typ)
	}
}

// startRecord continually captures audio data from the microphone,
// encodes it, and sends it to the SPICE server
func (d *ChRecord) startRecord() {
	defer d.stream.Stop()
	
	// Allocate buffer for encoded audio data
	buf := make([]byte, 512)

	// Send recording start marker with current media time
	d.conn.WriteMessage(SPICE_MSGC_RECORD_START_MARK, d.cl.MediaTime())

	// Main recording loop
	for {
		// Read audio data from microphone into PCM buffer
		err := d.stream.Read()
		if err != nil {
			log.Printf("spice/record: failed to read audio: %s", err)
			return
		}
		
		// Encode PCM data using Opus encoder
		n, err := d.enc.Encode(d.pcm, buf)
		if err != nil {
			log.Printf("spice/record: failed PCM Opus encoding: %s", err)
			return
		}
		
		// Send encoded audio data to server with current media time
		d.conn.WriteMessage(SPICE_MSGC_RECORD_DATA, d.cl.MediaTime(), buf[:n])

		// Check if recording should stop
		if atomic.LoadUint32(&d.run) != 1 {
			return
		}
	}
}
