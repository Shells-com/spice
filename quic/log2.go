package quic

func ceil_log2(val uint32) uint32 {
	if val == 1 {
		return 0
	}

	result := uint32(0)
	val -= 1
	for ; val > 0; val = val >> 1 {
		result++
	}

	return result
}
