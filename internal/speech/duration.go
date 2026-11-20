package speech

import (
	"encoding/binary"
	"errors"
	"math"
)

var ErrAudio = errors.New("Send a PCM WAV recording of at most 120 seconds.")

// ErrNoSpeech is a well-formed recording with nothing in it to hear: no
// samples (the learner stopped at once), too short to hold a word, or
// digital silence. It is not a fault; the page says "Didn't catch that".
var ErrNoSpeech = errors.New("nothing was heard in that recording")

// ErrTooLong is a well-formed recording longer than Scry will send on.
var ErrTooLong = errors.New("recording longer than 120 seconds")

// MinSpeechSeconds is the shortest recording that can hold a spoken word.
const MinSpeechSeconds = 0.25

// silencePeak is the loudest 16-bit sample a recording may have and still
// count as digital silence (about -66 dBFS); real microphones are louder.
const silencePeak = 16

// Inspect validates a recording like Duration and also says whether it
// can hold speech at all, so Scry never pays a provider to hear nothing.
func Inspect(b []byte) (float64, error) {
	seconds, start, size, err := parse(b)
	if err != nil {
		return 0, err
	}
	if size == 0 || seconds < MinSpeechSeconds {
		return seconds, ErrNoSpeech
	}
	peak := 0
	for i := start; i+1 < start+int(size); i += 2 {
		v := int(int16(binary.LittleEndian.Uint16(b[i:])))
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
			if peak > silencePeak {
				return seconds, nil
			}
		}
	}
	return seconds, ErrNoSpeech
}

// Duration validates PCM framing and counts audio samples on the server.
// Metadata/client duration is never trusted. Unsupported encodings fail closed.
func Duration(b []byte) (float64, error) {
	seconds, _, size, err := parse(b)
	if err == nil && size == 0 {
		return 0, ErrAudio
	}
	return seconds, err
}

func parse(b []byte) (float64, int, uint64, error) {
	fail := func() (float64, int, uint64, error) { return 0, 0, 0, ErrAudio }
	if len(b) < 44 || len(b) > 6<<20 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
		return fail()
	}
	var rate, align uint32
	var size uint64
	dataStart := 0
	format, data := false, false
	for pos := 12; pos+8 <= len(b); {
		n := uint64(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		start := pos + 8
		if n > uint64(len(b)-start) {
			return fail()
		}
		switch string(b[pos : pos+4]) {
		case "fmt ":
			if format || n < 16 {
				return fail()
			}
			format = true
			f := b[start : start+int(n)]
			channels := uint32(binary.LittleEndian.Uint16(f[2:4]))
			rate = binary.LittleEndian.Uint32(f[4:8])
			align = channels * 2
			if binary.LittleEndian.Uint16(f[:2]) != 1 || channels < 1 || channels > 2 || rate < 8000 || rate > 96000 || binary.LittleEndian.Uint16(f[14:16]) != 16 || uint32(binary.LittleEndian.Uint16(f[12:14])) != align || binary.LittleEndian.Uint32(f[8:12]) != rate*align {
				return fail()
			}
		case "data":
			if data {
				return fail()
			}
			data = true
			size, dataStart = n, start
		}
		pos = start + int(n) + int(n%2)
		if pos > len(b) {
			return fail()
		}
		if pos == len(b) {
			break
		}
		if len(b)-pos < 8 {
			return fail()
		}
	}
	if !format || !data || size%uint64(align) != 0 {
		return fail()
	}
	seconds := float64(size) / float64(rate*align)
	if math.IsNaN(seconds) {
		return fail()
	}
	if seconds > 120 {
		return 0, 0, 0, ErrTooLong
	}
	return seconds, dataStart, size, nil
}
