package parser

import "mime"

// wordDecoder is reused across calls (stateless and safe for concurrent use).
var wordDecoder = new(mime.WordDecoder)

// DecodeMIMEHeader decodes an RFC 2047 encoded header value.
// Returns the original string unchanged if decoding fails.
func DecodeMIMEHeader(s string) string {
	decoded, err := wordDecoder.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}
