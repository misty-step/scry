package speech

import (
	"encoding/binary"
	"testing"
)

func TestG12_ValidatedSampleDuration(t *testing.T) {
	b := make([]byte, 44+16000*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], 32000)
	if d, e := Duration(b); e != nil || d != 1 {
		t.Fatalf("duration=%v error=%v", d, e)
	}
	for _, at := range []int{0, 4, 8, 16, 20, 22, 24, 28, 32, 34, 40} {
		c := append([]byte(nil), b...)
		c[at] = 255
		if _, e := Duration(c); e == nil {
			t.Errorf("accepted corrupt header at %d", at)
		}
	}
	for _, n := range []int{0, 8, 43, 45, len(b) - 1} {
		if _, e := Duration(b[:n]); e == nil {
			t.Errorf("accepted truncation %d", n)
		}
	}
}
