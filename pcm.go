package aiff

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/bits"
)

// PCMFormat describes signed integer AIFF/AIFC PCM. Frames counts per-channel samples.
type PCMFormat struct {
	SampleRate, Channels, BitDepth int
	Frames                         int64
}

// PCMReader validates the entire FORM container before exposing signed PCM.
// Unknown chunks are skipped without allocation. ReadPCM borrows no caller storage.
type PCMReader struct {
	Format    PCMFormat
	r         io.ReadSeeker
	remaining int64
	little    bool
	scratch   []byte
}

// NewPCMReader accepts AIFF and uncompressed AIFC (NONE, twos, sowt).
// The caller owns r. It must contain one complete seekable file.
func NewPCMReader(r io.ReadSeeker) (*PCMReader, error) {
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("aiff.open: size: %w", err)
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var h [12]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return nil, fmt.Errorf("aiff.open: header: %w", err)
	}
	form := string(h[8:])
	limit := int64(binary.BigEndian.Uint32(h[4:8])) + 8
	if string(h[:4]) != "FORM" || (form != "AIFF" && form != "AIFC") || limit < 12 || limit > end {
		return nil, fmt.Errorf("aiff.open: invalid FORM container")
	}
	d := &PCMReader{r: r}
	var comm, sound bool
	var start, available int64
	for pos := int64(12); pos < limit; {
		if limit-pos < 8 {
			return nil, fmt.Errorf("aiff.open: truncated chunk header")
		}
		if _, err = r.Seek(pos, io.SeekStart); err != nil {
			return nil, err
		}
		var ch [8]byte
		if _, err = io.ReadFull(r, ch[:]); err != nil {
			return nil, err
		}
		n := int64(binary.BigEndian.Uint32(ch[4:]))
		body, next := pos+8, pos+8+n+(n&1)
		if next > limit {
			return nil, fmt.Errorf("aiff.open: truncated %.4s chunk", ch[:4])
		}
		switch string(ch[:4]) {
		case "COMM":
			if comm || n < 18 {
				return nil, fmt.Errorf("aiff.open: invalid or duplicate COMM")
			}
			comm = true
			var c [22]byte
			count := 18
			if form == "AIFC" {
				count = 22
			}
			if n < int64(count) {
				return nil, fmt.Errorf("aiff.open: short AIFC COMM")
			}
			if _, err = io.ReadFull(r, c[:count]); err != nil {
				return nil, err
			}
			d.Format.Channels = int(binary.BigEndian.Uint16(c[:2]))
			d.Format.Frames = int64(binary.BigEndian.Uint32(c[2:6]))
			d.Format.BitDepth = int(binary.BigEndian.Uint16(c[6:8]))
			exponent := binary.BigEndian.Uint16(c[8:10])
			mantissa := binary.BigEndian.Uint64(c[10:18])
			rate := math.Ldexp(float64(mantissa), int(exponent)-16383-63)
			if exponent&0x8000 != 0 || exponent == 0x7fff || rate < 1 || rate > math.MaxInt32 || math.Trunc(rate) != rate {
				return nil, fmt.Errorf("aiff.open: invalid integral sample rate")
			}
			d.Format.SampleRate = int(rate)
			if form == "AIFC" {
				switch string(c[18:22]) {
				case "NONE", "twos":
				case "sowt":
					d.little = true
				default:
					return nil, fmt.Errorf("aiff.open: unsupported compression %.4s", c[18:22])
				}
			}
		case "SSND":
			if sound || n < 8 {
				return nil, fmt.Errorf("aiff.open: invalid or duplicate SSND")
			}
			sound = true
			var s [8]byte
			if _, err = io.ReadFull(r, s[:]); err != nil {
				return nil, err
			}
			offset := int64(binary.BigEndian.Uint32(s[:4]))
			if offset > n-8 {
				return nil, fmt.Errorf("aiff.open: invalid SSND offset")
			}
			start = body + 8 + offset
			available = n - 8 - offset
		}
		pos = next
	}
	f := d.Format
	if !comm || !sound || f.Channels < 1 || (f.BitDepth != 8 && f.BitDepth != 16 && f.BitDepth != 24 && f.BitDepth != 32) {
		return nil, fmt.Errorf("aiff.open: missing or unsupported PCM format")
	}
	d.remaining = f.Frames * int64(f.Channels) * int64(f.BitDepth/8)
	if d.remaining > available {
		return nil, fmt.Errorf("aiff.open: truncated PCM frames")
	}
	if _, err = r.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	return d, nil
}

// ReadPCM fills dst with interleaved signed integers. dst must hold whole frames.
// It returns io.EOF only when no samples remain; its scratch grows to at most dst's size.
func (d *PCMReader) ReadPCM(dst []int32) (int, error) {
	if len(dst)%d.Format.Channels != 0 {
		return 0, fmt.Errorf("aiff.read: buffer must contain whole frames")
	}
	if len(dst) == 0 {
		return 0, nil
	}
	if d.remaining == 0 {
		return 0, io.EOF
	}
	width := d.Format.BitDepth / 8
	count := int(min(int64(len(dst)), d.remaining/int64(width)))
	size := count * width
	if cap(d.scratch) < size {
		d.scratch = make([]byte, size)
	}
	raw := d.scratch[:size]
	if _, err := io.ReadFull(d.r, raw); err != nil {
		return 0, fmt.Errorf("aiff.read: PCM: %w", err)
	}
	for i := range count {
		var v uint32
		for j := range width {
			index := i*width + j
			if d.little {
				v |= uint32(raw[index]) << uint(j*8)
			} else {
				v = v<<8 | uint32(raw[index])
			}
		}
		shift := uint(32 - d.Format.BitDepth)
		dst[i] = int32(v<<shift) >> shift
	}
	d.remaining -= int64(size)
	return count, nil
}

// PCMWriter writes a known number of signed integer PCM frames as AIFF.
// The caller owns w. Close checks the frame count and writes odd-byte padding.
type PCMWriter struct {
	Format  PCMFormat
	w       io.Writer
	samples int64
	closed  bool
	scratch []byte
}

// NewPCMWriter emits a standard AIFF header without needing a seekable writer.
func NewPCMWriter(w io.Writer, f PCMFormat) (*PCMWriter, error) {
	if f.SampleRate < 1 || f.SampleRate > math.MaxInt32 || f.Channels < 1 || f.Channels > math.MaxUint16 || f.Frames < 0 || f.Frames > math.MaxUint32 || (f.BitDepth != 8 && f.BitDepth != 16 && f.BitDepth != 24 && f.BitDepth != 32) {
		return nil, fmt.Errorf("aiff.write: unsupported PCM format")
	}
	size := f.Frames * int64(f.Channels) * int64(f.BitDepth/8)
	if size+(size&1)+46 > math.MaxUint32 {
		return nil, fmt.Errorf("aiff.write: FORM size limit exceeded")
	}
	var h [54]byte
	copy(h[:], "FORM")
	binary.BigEndian.PutUint32(h[4:], uint32(size+(size&1)+46))
	copy(h[8:], "AIFFCOMM")
	binary.BigEndian.PutUint32(h[16:], 18)
	binary.BigEndian.PutUint16(h[20:], uint16(f.Channels))
	binary.BigEndian.PutUint32(h[22:], uint32(f.Frames))
	binary.BigEndian.PutUint16(h[26:], uint16(f.BitDepth))
	exponent := bits.Len32(uint32(f.SampleRate)) - 1
	binary.BigEndian.PutUint16(h[28:], uint16(16383+exponent))
	binary.BigEndian.PutUint64(h[30:], uint64(f.SampleRate)<<uint(63-exponent))
	copy(h[38:], "SSND")
	binary.BigEndian.PutUint32(h[42:], uint32(size+8))
	if err := writePCMBytes(w, h[:]); err != nil {
		return nil, fmt.Errorf("aiff.write: header: %w", err)
	}
	return &PCMWriter{Format: f, w: w}, nil
}

// WritePCM writes interleaved signed samples without rescaling or rounding.
func (e *PCMWriter) WritePCM(src []int32) error {
	if e.closed {
		return fmt.Errorf("aiff.write: writer is closed")
	}
	if len(src)%e.Format.Channels != 0 || int64(len(src)) > e.Format.Frames*int64(e.Format.Channels)-e.samples {
		return fmt.Errorf("aiff.write: invalid frame count")
	}
	low, high := -(int64(1) << uint(e.Format.BitDepth-1)), (int64(1)<<uint(e.Format.BitDepth-1))-1
	for _, v := range src {
		if int64(v) < low || int64(v) > high {
			return fmt.Errorf("aiff.write: sample exceeds bit depth")
		}
	}
	width := e.Format.BitDepth / 8
	size := len(src) * width
	if cap(e.scratch) < size {
		e.scratch = make([]byte, size)
	}
	raw := e.scratch[:size]
	for i, v := range src {
		for j := range width {
			raw[i*width+j] = byte(uint32(v) >> uint((width-j-1)*8))
		}
	}
	if err := writePCMBytes(e.w, raw); err != nil {
		return fmt.Errorf("aiff.write: PCM: %w", err)
	}
	e.samples += int64(len(src))
	return nil
}

// Close finalizes a complete stream. Repeated successful closes are harmless.
func (e *PCMWriter) Close() error {
	if e.closed {
		return nil
	}
	if e.samples != e.Format.Frames*int64(e.Format.Channels) {
		return fmt.Errorf("aiff.close: incomplete PCM")
	}
	if e.samples*int64(e.Format.BitDepth/8)&1 != 0 {
		if err := writePCMBytes(e.w, []byte{0}); err != nil {
			return err
		}
	}
	e.closed = true
	return nil
}
func writePCMBytes(w io.Writer, p []byte) error {
	n, err := w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return err
}
