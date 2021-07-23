package quic

type quicModel struct {
	bpc         uint32
	ncounters   uint32
	levels      uint32
	nBucketPtrs uint32
	repfirst    uint32
	firstsize   uint32
	repnext     uint32
	mulsize     uint32
	nBuckets    uint32
}

// model evolution, warning: only 1,3 and 5 allowed
const quicEvol = 3

func newModel(bpc uint32) *quicModel {
	m := quicModel{
		bpc:       bpc,
		levels:    1 << bpc,
		ncounters: 8,
	}

	// find_model_params

	switch quicEvol {
	case 1: // buckets contain following numbers of contexts: 1 1 1 2 2 4 4 8 8 ...
		m.repfirst = 3
		m.firstsize = 1
		m.repnext = 2
		m.mulsize = 2
	case 3: // 1 2 4 8 16 32 64 ...
		m.repfirst = 1
		m.firstsize = 1
		m.repnext = 1
		m.mulsize = 2
	case 5: // 1 4 16 64 256 1024 4096 16384 65536
		m.repfirst = 1
		m.firstsize = 1
		m.repnext = 1
		m.mulsize = 4
	case 0, 2, 4:
		panic("quic: findmodelparams(): evol value obsolete!!!")
	default:
		panic("quic: findmodelparams(): evol out of range!!!")
	}

	m.nBuckets = 0
	repcntr := m.repfirst + 1 // first bucket
	bsize := m.firstsize

	var bstart, bend uint32

	for {
		if m.nBuckets > 0 {
			bstart = bend + 1
		} else {
			bstart = 0
		}

		// bucket size
		repcntr -= 1
		if repcntr == 0 {
			repcntr = m.repnext
			bsize *= m.mulsize
		}

		bend = bstart + bsize - 1 // bucket end

		if m.nBucketPtrs == 0 {
			m.nBucketPtrs = m.levels
		}

		if bend >= m.levels-1 {
			break
		}
		m.nBuckets += 1
	}

	return &m
}
