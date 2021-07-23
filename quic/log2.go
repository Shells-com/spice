package quic

var tab64 = [...]uint64{
	63, 0, 58, 1, 59, 47, 53, 2,
	60, 39, 48, 27, 54, 33, 42, 3,
	61, 51, 37, 40, 49, 18, 28, 20,
	55, 30, 34, 11, 43, 14, 22, 4,
	62, 57, 46, 52, 38, 26, 32, 41,
	50, 36, 17, 19, 29, 10, 13, 21,
	56, 45, 25, 31, 35, 16, 9, 12,
	44, 24, 15, 8, 23, 7, 6, 5,
}

func log2_64(value uint64) uint64 {
	value |= value >> 1
	value |= value >> 2
	value |= value >> 4
	value |= value >> 8
	value |= value >> 16
	value |= value >> 32
	return tab64[((value-(value>>1))*0x07EDD5E59A4E28C2)>>58]
}

var tab32 = [...]uint32{
	0, 9, 1, 10, 13, 21, 2, 29,
	11, 14, 16, 18, 22, 25, 3, 30,
	8, 12, 20, 28, 15, 17, 24, 7,
	19, 27, 23, 6, 26, 5, 4, 31,
}

func log2_32(value uint32) uint32 {
	value |= value >> 1
	value |= value >> 2
	value |= value >> 4
	value |= value >> 8
	value |= value >> 16
	return tab32[(value*0x07C4ACDD)>>27]
}

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
