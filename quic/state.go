package quic

const defWminext = 2048

type quicState struct {
	waitcnt      uint32
	tabrand_seed uint8
	wm_trigger   uint32
	wmidx        uint32
	wmileft      uint32

	melcstate uint32 // index to the state array

	// contents of the state array location indexed by melcstate: the
	// "expected" run length is 2^melclen, shorter runs are encoded by a 1
	// followed by the run length in binary representation, wit a fixed
	// length of melclen bits
	melclen uint32

	melcorder uint64 // 2^ melclen
}

func newState() *quicState {
	res := &quicState{
		tabrand_seed: 0xff,
		wmileft:      defWminext,
	}
	res.reset()

	return res
}

func (s *quicState) setWmTrigger() {
	wm := s.wmidx
	if wm > 10 {
		wm = 10
	}
	s.wm_trigger = besttrigtab[quicEvol/2][wm]
}

func (s *quicState) tabrand() uint32 {
	s.tabrand_seed += 1
	return tabrand_chaos[s.tabrand_seed]
}

func (s *quicState) reset() {
	s.waitcnt = 0
	s.tabrand_seed = 0xff
	s.wmidx = 0
	s.wmileft = defWminext

	s.setWmTrigger()

	s.melcstate = 0
	s.melclen = uint32(quicJ[0])
	s.melcorder = 1 << s.melclen
}
