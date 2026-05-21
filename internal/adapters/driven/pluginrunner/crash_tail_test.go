package pluginrunner

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTailBuffer_KeepsLastBytes(t *testing.T) {
	tb := newTailBuffer(8)
	tb.Write([]byte("hello"))
	assert.Equal(t, "hello", tb.String(), "under-cap content kept verbatim")

	tb.Write([]byte("world!!")) // "helloworld!!" (12) -> keep last 8
	got := tb.String()
	assert.Len(t, got, 8)
	assert.Equal(t, "oworld!!", got)
	assert.True(t, strings.HasSuffix("helloworld!!", got))
}

func TestTailBuffer_SingleWriteLargerThanCap(t *testing.T) {
	tb := newTailBuffer(4)
	tb.Write([]byte("123456789"))
	assert.Equal(t, "6789", tb.String(), "a single oversized write is trimmed to the last cap bytes")
}

func TestTailBuffer_Empty(t *testing.T) {
	assert.Equal(t, "", newTailBuffer(16).String())
}
