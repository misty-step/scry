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

func TestD2_InspectFindsRecordingsWithNothingToHear(t *testing.T) {
	mk := func(n int, tone bool) []byte {
		b := make([]byte, 44+n*2)
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
		binary.LittleEndian.PutUint32(b[40:], uint32(n*2))
		for i := 0; tone && i < n; i++ {
			binary.LittleEndian.PutUint16(b[44+2*i:], uint16(int16(3000*((i%20)-10)/10)))
		}
		return b
	}
	for name, c := range map[string]struct {
		b    []byte
		want error
	}{
		"no samples": {mk(0, false), ErrNoSpeech},
		"0.1 s":      {mk(1600, true), ErrNoSpeech},
		"silent":     {mk(16000, false), ErrNoSpeech},
		"audible":    {mk(16000, true), nil},
		"too long":   {mk(16000*121, true), ErrTooLong},
		"garbage":    {[]byte("RIFF nonsense"), ErrAudio},
	} {
		if _, err := Inspect(c.b); err != c.want {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}
