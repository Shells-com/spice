package quic

import (
	"math/bits"
)

type quicFamily struct {
	bpc             uint32
	limit           uint32
	nGRcodewords    [8]uint32
	notGRcwlen      [8]uint32
	notGRprefixmask [8]uint32
	notGRsuffixlen  [8]uint32
	xlatU2L         [256]uint32
	xlatL2U         [256]uint32
}

var (
	family_5bpc = initFamily(5, 26)
	family_8bpc = initFamily(8, 26)
)

func initFamily(bpc, limit uint32) *quicFamily {
	family := &quicFamily{
		bpc:   bpc,
		limit: limit,
	}

	for l := uint32(0); l < bpc; l++ {
		altprefixlen := limit - bpc
		if altprefixlen > bppmask[bpc-l] {
			altprefixlen = bppmask[bpc-l]
		}
		altcodewords := bppmask[bpc] + 1 - (altprefixlen << l)
		family.nGRcodewords[l] = altprefixlen << l
		family.notGRcwlen[l] = altprefixlen + ceil_log2(altcodewords)
		family.notGRprefixmask[l] = bppmask[32-altprefixlen]
		family.notGRsuffixlen[l] = ceil_log2(altcodewords)
	}

	pixelbitmask := bppmask[bpc]
	pixelbitmaskshr := pixelbitmask >> 1

	for s := uint32(0); s <= pixelbitmask; s += 1 {
		if s <= pixelbitmaskshr {
			family.xlatU2L[s] = s << 1
		} else {
			family.xlatU2L[s] = ((pixelbitmask - s) << 1) + 1
		}
	}

	/* corelate_init */
	for s := uint32(0); s <= pixelbitmask; s++ {
		if (s & 0x01) == 0x01 {
			family.xlatL2U[s] = pixelbitmask - (s >> 1)
		} else {
			family.xlatL2U[s] = (s >> 1)
		}
	}

	return family
}

func (f *quicFamily) golombCodeLen(n, l uint32) uint32 {
	if n < f.nGRcodewords[l] {
		return (n >> l) + 1 + l
	} else {
		return f.notGRcwlen[l]
	}
}

func (f *quicFamily) golombDecoding(l, nbits uint32) (rc byte, cwlen int) {
	// we know l < 8, so result will always be 8 bits
	if nbits > f.notGRprefixmask[l] {
		zeroPrefix := uint32(bits.LeadingZeros32(nbits))
		cwlen = int(zeroPrefix + 1 + l)
		rc = byte((zeroPrefix << l) | (nbits>>(32-cwlen))&bppmask[l])
		return rc, cwlen
	} else {
		cwlen = int(f.notGRcwlen[l])
		rc = byte(f.nGRcodewords[l] + ((nbits >> (32 - cwlen)) & bppmask[f.notGRsuffixlen[l]]))
		return rc, cwlen
	}
}
