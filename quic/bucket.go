package quic

type quicBucket struct {
	bestcode uint32 // best code so far
	counters []uint32
}

func newBucket(bpp uint32) *quicBucket {
	return &quicBucket{
		bestcode: bpp,               // 7 or 4 etc
		counters: make([]uint32, 8), // 8?
	}
}

func (b *quicBucket) updateModel(f *quicFamily, s *quicState, curval, bpc uint32) {
	bpp := bpc
	pcounters := b.counters

	bestcode := bpp - 1
	pcounters[bestcode] += f.golombCodeLen(curval, bestcode)
	bestcodeLen := pcounters[bestcode]

	for i := bpp - 2; i < bpp; i -= 1 { // NOTE: expression i<bpp for signed int i would be: i>=0
		pcounters[i] += f.golombCodeLen(curval, i)
		ithcodeLen := pcounters[i]

		if ithcodeLen < bestcodeLen {
			bestcode = i
			bestcodeLen = ithcodeLen
		}
	}

	b.bestcode = bestcode

	if bestcodeLen > s.wm_trigger { // halving counters?
		for i := uint32(0); i < bpp; i += 1 {
			pcounters[i] >>= 1
		}
	}
}
