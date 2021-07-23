package quic

type quicFamilyStat struct {
	bucketPtrs []*quicBucket
	bucketBuf  []*quicBucket
}

func newFamilyStat(m *quicModel) *quicFamilyStat {
	st := &quicFamilyStat{}

	var bstart, bend uint32

	repcntr := m.repfirst + 1
	bsize := m.firstsize

	for bend < m.levels-1 {
		repcntr -= 1
		if repcntr == 0 {
			repcntr = m.repnext
			bsize *= m.mulsize
		}

		bend = bstart + bsize - 1
		if bend+bsize >= m.levels {
			bend = m.levels - 1
		}

		bucket := newBucket(m.bpc - 1) // bpc8→7 bpc5→4
		st.bucketBuf = append(st.bucketBuf, bucket)

		for i := bstart; i <= bend; i++ {
			st.bucketPtrs = append(st.bucketPtrs, bucket)
		}

		// for next iteration
		bstart = bend + 1
	}

	return st
}
