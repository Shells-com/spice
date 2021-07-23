package spice

import (
	"log"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

type timeBufferFragment struct {
	time uint32
	buf  []int16
}

type timeBuffer struct {
	cl   *Client
	play *ChPlayback
	lk   sync.Mutex
	frag []timeBufferFragment
	ping chan struct{}
	pos  int // position in output buffer
}

func NewTimeBuffer(cl *Client, d *ChPlayback) *timeBuffer {
	b := &timeBuffer{
		cl:   cl,
		play: d,
		ping: make(chan struct{}),
	}
	go b.runner()
	return b
}

func (b *timeBuffer) Append(mmtime uint32, buf []int16) error {
	b.lk.Lock()
	defer b.lk.Unlock()

	b.frag = append(b.frag, timeBufferFragment{time: mmtime, buf: buf})

	select {
	case b.ping <- struct{}{}:
	default:
	}
	return nil
}

func (b *timeBuffer) consumeFrag(frag timeBufferFragment) {
	// unlock during write so we can receive more samples
	b.lk.Unlock()
	defer b.lk.Lock()

	rbuf := frag.buf

	for {
		n := copy(b.play.buf[b.pos:], rbuf)
		if b.pos+n == len(b.play.buf) {
			// buffer was filled!
			for {
				err := b.play.stream.Write()
				if err != nil {
					if err == portaudio.OutputUnderflowed {
						log.Printf("PA: Output underflowed, retrying")
						time.Sleep(2 * time.Millisecond)
						continue
					}
					log.Printf("PA: failed to write: %s", err)
				}
				break
			}
			b.pos = 0
		} else {
			b.pos += n
		}

		if n == len(rbuf) {
			// all written!
			return
		}

		// resume
		rbuf = rbuf[n:]
	}
}

func (b *timeBuffer) release(t *time.Timer) {
	b.lk.Lock()
	defer b.lk.Unlock()

	mmt := b.cl.MediaTime()

	for {
		if len(b.frag) == 0 {
			return
		}

		if b.frag[0].time <= mmt {
			// need to run NOW
			//log.Printf("playing sound at offset %d", int64(b.frag[0].time)-int64(mmt))
			frag := b.frag[0]
			b.frag = b.frag[1:]

			b.consumeFrag(frag)
			continue
		}

		// next will be >= 1
		next := b.cl.MediaTill(b.frag[0].time)
		t.Reset(next)
		return
	}
}

func (b *timeBuffer) runner() {
	t := time.NewTimer(5 * time.Second)

	for {
		b.release(t)

		if t == nil {
			select {
			case <-b.ping:
				// cause refresh
			}
		} else {
			select {
			case <-b.ping:
			// cause refresh
			case <-t.C:
				// the time has come
			}
		}
	}
}
