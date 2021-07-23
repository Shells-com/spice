package quic

type quicChannel struct {
	state               *quicState
	familyStat8bpc      *quicFamilyStat
	familyStat5bpc      *quicFamilyStat
	correlate_row_width uint32
	correlate_row_zero  byte // TODO do we really need this?
	correlate_row       []byte
	model               *quicModel
	buckets_ptrs        []*quicBucket
}

func newChannel(width, bpc uint32) *quicChannel {
	res := &quicChannel{}
	res.state = newState()
	res.familyStat8bpc = newFamilyStat(newModel(8))
	res.familyStat5bpc = newFamilyStat(newModel(5))
	res.model = newModel(bpc)
	res.correlate_row = make([]byte, width)
	res.correlate_row_width = width

	switch bpc {
	case 8:
		res.buckets_ptrs = res.familyStat8bpc.bucketPtrs
	case 5:
		res.buckets_ptrs = res.familyStat5bpc.bucketPtrs
	default:
		panic("quic: bad bpc in newChannel()")
	}

	return res
}
