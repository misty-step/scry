package speech

import (
	"encoding/binary"
	"errors"
	"math"
)

var ErrAudio = errors.New("Send a PCM WAV recording of at most 120 seconds.")

// Duration validates PCM framing and counts audio samples on the server.
// Metadata/client duration is never trusted. Unsupported encodings fail closed.
func Duration(b []byte) (float64, error) {
	if len(b) < 44 || len(b) > 6<<20 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
		return 0, ErrAudio
	}
	var rate, align uint32
	var size uint64
	format, data := false, false
	for pos := 12; pos+8 <= len(b); {
		n := uint64(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		start := pos + 8
		if n > uint64(len(b)-start) {
			return 0, ErrAudio
		}
		switch string(b[pos : pos+4]) {
		case "fmt ":
			if format || n < 16 {
				return 0, ErrAudio
			}
			format = true
			f := b[start : start+int(n)]
			channels := uint32(binary.LittleEndian.Uint16(f[2:4]))
			rate = binary.LittleEndian.Uint32(f[4:8])
			align = channels * 2
			if binary.LittleEndian.Uint16(f[:2]) != 1 || channels < 1 || channels > 2 || rate < 8000 || rate > 96000 || binary.LittleEndian.Uint16(f[14:16]) != 16 || uint32(binary.LittleEndian.Uint16(f[12:14])) != align || binary.LittleEndian.Uint32(f[8:12]) != rate*align {
				return 0, ErrAudio
			}
		case "data":
			if data {
				return 0, ErrAudio
			}
			data = true
			size = n
		}
		pos = start + int(n) + int(n%2)
		if pos > len(b) {
			return 0, ErrAudio
		}
		if pos == len(b) {
			break
		}
		if len(b)-pos < 8 {
			return 0, ErrAudio
		}
	}
	if !format || !data || size == 0 || size%uint64(align) != 0 {
		return 0, ErrAudio
	}
	seconds := float64(size) / float64(rate*align)
	if math.IsNaN(seconds) || seconds > 120 {
		return 0, ErrAudio
	}
	return seconds, nil
}
