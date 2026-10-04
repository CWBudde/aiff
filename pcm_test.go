package aiff

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestSignedPCM(t *testing.T) {
	for _, depth := range []int{8, 16, 24, 32} {
		for _, channels := range []int{1, 2, 6} {
			low := int32(-(int64(1) << uint(depth-1)))
			high := int32((int64(1) << uint(depth-1)) - 1)
			samples := make([]int32, 17*channels)
			for i := range samples {
				samples[i] = []int32{low, high, -1, 0, 1}[i%5]
			}
			var out bytes.Buffer
			f := PCMFormat{44100, channels, depth, 17}
			w, err := NewPCMWriter(&out, f)
			if err != nil {
				t.Fatal(err)
			}
			if err = w.WritePCM(samples[:channels*7]); err != nil {
				t.Fatal(err)
			}
			if err = w.WritePCM(samples[channels*7:]); err != nil {
				t.Fatal(err)
			}
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			d, err := NewPCMReader(bytes.NewReader(out.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if d.Format != f {
				t.Fatalf("format: %+v", d.Format)
			}
			got := make([]int32, len(samples)+channels)
			n, err := d.ReadPCM(got)
			if err != nil || n != len(samples) {
				t.Fatalf("read: %d %v", n, err)
			}
			for i, v := range samples {
				if got[i] != v {
					t.Fatalf("depth %d channel %d sample %d: %d != %d", depth, channels, i, got[i], v)
				}
			}
			if _, err = d.ReadPCM(got); err != io.EOF {
				t.Fatalf("EOF: %v", err)
			}
		}
	}
}

// Fixture bytes are constructed without the encoder, including signed 8-bit,
// little-endian AIFC, SSND offset/blocksize, odd chunks and reversed ordering.
func TestIndependentPCMContainers(t *testing.T) {
	for _, codec := range []string{"AIFF", "NONE", "twos", "sowt"} {
		comm := []byte{0, 1, 0, 0, 0, 3, 0, 8, 0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}
		form := "AIFF"
		if codec != "AIFF" {
			form = "AIFC"
			comm = append(comm, []byte(codec)...)
			comm = append(comm, 0, 0)
		}
		chunk := func(id string, p []byte) []byte {
			h := make([]byte, 8)
			copy(h, id)
			binary.BigEndian.PutUint32(h[4:], uint32(len(p)))
			h = append(h, p...)
			if len(p)&1 != 0 {
				h = append(h, 0)
			}
			return h
		}
		body := append([]byte(form), chunk("JUNK", []byte{42})...)
		body = append(body, chunk("SSND", []byte{0, 0, 0, 2, 0, 0, 0, 99, 9, 9, 128, 255, 127})...)
		body = append(body, chunk("COMM", comm)...)
		h := make([]byte, 8)
		copy(h, "FORM")
		binary.BigEndian.PutUint32(h[4:], uint32(len(body)))
		raw := append(h, body...)
		d, err := NewPCMReader(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(codec, err)
		}
		out := make([]int32, 3)
		if n, err := d.ReadPCM(out); n != 3 || err != nil {
			t.Fatal(n, err)
		}
		for i, v := range []int32{-128, -1, 127} {
			if out[i] != v {
				t.Fatal(codec, out)
			}
		}
	}
}
func TestMalformedPCM(t *testing.T) {
	var b bytes.Buffer
	w, _ := NewPCMWriter(&b, PCMFormat{48000, 1, 16, 1})
	_ = w.WritePCM([]int32{2})
	_ = w.Close()
	valid := b.Bytes()
	for i := 0; i < len(valid); i++ {
		if _, err := NewPCMReader(bytes.NewReader(valid[:i])); err == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}
	for _, offset := range []int{4, 16, 20, 26, 28, 42, 46} {
		p := append([]byte(nil), valid...)
		p[offset] = 255
		if _, err := NewPCMReader(bytes.NewReader(p)); err == nil {
			t.Fatalf("accepted corrupt offset %d", offset)
		}
	}
}
func TestWriterValidation(t *testing.T) {
	var b bytes.Buffer
	w, err := NewPCMWriter(&b, PCMFormat{48000, 2, 8, 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, samples := range [][]int32{{1}, {1, 128}, {1, 2, 3, 4}} {
		if err = w.WritePCM(samples); err == nil {
			t.Fatal("accepted", samples)
		}
	}
	if err = w.Close(); err == nil {
		t.Fatal("incomplete")
	}
	if err = w.WritePCM([]int32{-128, 127}); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = w.WritePCM(nil); err == nil {
		t.Fatal("closed write")
	}
}
func FuzzPCMReader(f *testing.F) {
	f.Add([]byte("FORM"))
	var b bytes.Buffer
	w, _ := NewPCMWriter(&b, PCMFormat{48000, 1, 8, 1})
	_ = w.WritePCM([]int32{-1})
	_ = w.Close()
	f.Add(b.Bytes())
	f.Fuzz(func(t *testing.T, p []byte) {
		d, err := NewPCMReader(bytes.NewReader(p))
		if err != nil {
			return
		}
		_, _ = d.ReadPCM(make([]int32, d.Format.Channels*8))
	})
}
