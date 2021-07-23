package quic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModels(t *testing.T) {
	m8 := newModel(8)
	m5 := newModel(5)

	// some checks
	assert.EqualValues(t, 256, m8.levels)
	assert.EqualValues(t, 8, m8.nBuckets)
	assert.EqualValues(t, 256, m8.nBucketPtrs)

	assert.EqualValues(t, 32, m5.levels)
	assert.EqualValues(t, 5, m5.nBuckets)
	assert.EqualValues(t, 32, m5.nBucketPtrs)

	// test newFamilyStat
	st5 := newFamilyStat(m5)
	st8 := newFamilyStat(m8)

	assert.EqualValues(t, m8.nBuckets, len(st8.bucketBuf))
	assert.EqualValues(t, m8.nBucketPtrs, len(st8.bucketPtrs))
	assert.EqualValues(t, m5.nBuckets, len(st5.bucketBuf))
	assert.EqualValues(t, m5.nBucketPtrs, len(st5.bucketPtrs))
}
