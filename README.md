# aiff

Pure Go AIFF/AIFC reader and AIFF writer, continued from the archived [go-audio/aiff](https://github.com/go-audio/aiff) project. Original history and Apache-2.0 licensing are preserved. The legacy API remains available; new consumers should use the validated streaming PCM API.

```go
decoder, err := aiff.NewPCMReader(reader) // io.ReadSeeker, caller owned
// Check err, then inspect decoder.Format before allocating buffers.
samples := make([]int32, 4096*decoder.Format.Channels)
n, err := decoder.ReadPCM(samples) // interleaved signed integers, no rescaling
```

`NewPCMWriter(writer, PCMFormat{SampleRate: 48000, Channels: 2, BitDepth: 24, Frames: frames})` writes standard AIFF to any io.Writer. Use WritePCM and Close; Close validates the declared frame count and writes odd-byte padding. The caller owns the underlying stream.

Supported input: signed 8/16/24/32-bit integer AIFF and AIFC NONE/twos/sowt. Floating-point and compressed AIFC codecs are rejected. Integral sample rates are required. Unknown metadata chunks are skipped without allocation; PCMFormat does not expose metadata. Readers validate FORM/chunk boundaries, COMM/SSND uniqueness, sample counts and SSND offsets. Applications must enforce their own decoded memory limits. Scratch grows only to the caller's PCM buffer size.

The original Decoder/Encoder API retains upstream behavior and is provided for compatibility. Its permissive parsing is unsuitable for untrusted input.

Run `just ci`, `just fuzz`, and `just test-wasm`. Releases go through `just tag-release VERSION`; consume official tags, never local replacements or pseudo-versions.
