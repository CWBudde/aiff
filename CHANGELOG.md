# Changelog

## [v0.1.0] - 2026-10-04

- Continue the archived go-audio/aiff project under github.com/cwbudde/aiff, retaining its Apache-2.0 license, history and legacy API.
- Add bounded PCMReader/PCMWriter APIs for signed 8/16/24/32-bit AIFF and uncompressed AIFC (NONE, twos, sowt), whole-frame buffers, SSND offsets, chunk alignment and strict truncation/size validation. Standard AIFF output supports non-seekable writers, exact frame counts and odd-byte padding.
- Add independent container fixtures, malformed-input tests, fuzzing and native/386/WebAssembly CI.
