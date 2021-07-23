package spice

func caps(c ...uint32) []uint32 {
	var v uint32

	for _, n := range c {
		v |= 1 << n
	}
	return []uint32{v}
}

func testCap(b, value uint32) bool {
	return (b & (1 << value)) != 0
}
