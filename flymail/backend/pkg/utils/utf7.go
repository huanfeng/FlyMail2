package utils

import (
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// EncodeUTF7 encodes a string to modified UTF-7 for IMAP
func EncodeUTF7(s string) string {
	if s == "" {
		return ""
	}

	var result strings.Builder
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		start := i
		// Find sequence of non-ASCII characters
		for i < len(runes) && runes[i] > 0x7F {
			i++
		}

		if i > start {
			// Encode non-ASCII sequence
			result.WriteString(encodeUTF7Sequence(runes[start:i]))
		}

		// Find sequence of ASCII characters
		start = i
		for i < len(runes) && runes[i] <= 0x7F {
			if runes[i] == '&' {
				result.WriteString("&-")
			} else {
				result.WriteRune(runes[i])
			}
			i++
		}
	}

	return result.String()
}

// DecodeUTF7 decodes a modified UTF-7 string from IMAP
func DecodeUTF7(s string) (string, error) {
	if s == "" {
		return "", nil
	}

	var result strings.Builder
	i := 0

	for i < len(s) {
		if s[i] == '&' && i+1 < len(s) {
			if s[i+1] == '-' {
				// Escaped ampersand
				result.WriteRune('&')
				i += 2
				continue
			}

			// Find the end of the encoded sequence
			end := i + 1
			for end < len(s) && s[end] != '-' {
				end++
			}

			if end >= len(s) {
				return "", errors.New("unterminated UTF-7 sequence")
			}

			// Decode the sequence
			decoded, err := decodeUTF7Sequence(s[i+1 : end])
			if err != nil {
				return "", err
			}
			result.WriteString(decoded)
			i = end + 1
		} else {
			result.WriteByte(s[i])
			i++
		}
	}

	return result.String(), nil
}

// encodeUTF7Sequence encodes a sequence of runes to modified base64
func encodeUTF7Sequence(runes []rune) string {
	// Convert runes to UTF-16
	var utf16Data []uint16
	for _, r := range runes {
		if r <= 0xFFFF {
			utf16Data = append(utf16Data, uint16(r))
		} else {
			// Handle surrogate pairs
			r -= 0x10000
			high := uint16((r >> 10) + 0xD800)
			low := uint16((r & 0x3FF) + 0xDC00)
			utf16Data = append(utf16Data, high, low)
		}
	}

	// Convert UTF-16 to bytes
	var bytes []byte
	for _, u := range utf16Data {
		bytes = append(bytes, byte(u>>8), byte(u))
	}

	// Encode to modified base64
	encoded := encodeModifiedBase64(bytes)
	return "&" + encoded + "-"
}

// decodeUTF7Sequence decodes a modified base64 sequence to string
func decodeUTF7Sequence(s string) (string, error) {
	// Decode from modified base64
	bytes, err := decodeModifiedBase64(s)
	if err != nil {
		return "", err
	}

	// Convert bytes to UTF-16
	if len(bytes)%2 != 0 {
		return "", errors.New("invalid UTF-7 sequence: odd number of bytes")
	}

	var utf16Data []uint16
	for i := 0; i < len(bytes); i += 2 {
		utf16Data = append(utf16Data, uint16(bytes[i])<<8|uint16(bytes[i+1]))
	}

	// Convert UTF-16 to runes
	runes := utf16.Decode(utf16Data)
	return string(runes), nil
}

// Modified base64 encoding for IMAP UTF-7
const modifiedBase64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+,"

// encodeModifiedBase64 encodes bytes to modified base64
func encodeModifiedBase64(src []byte) string {
	if len(src) == 0 {
		return ""
	}

	var result strings.Builder

	for i := 0; i < len(src); i += 3 {
		var b1, b2, b3 byte
		var pad int

		b1 = src[i]
		if i+1 < len(src) {
			b2 = src[i+1]
		} else {
			pad = 1
		}
		if i+2 < len(src) {
			b3 = src[i+2]
		} else if pad == 0 {
			pad = 2
		}

		// Encode
		val := uint32(b1)<<16 | uint32(b2)<<8 | uint32(b3)
		result.WriteByte(modifiedBase64[(val>>18)&0x3F])
		result.WriteByte(modifiedBase64[(val>>12)&0x3F])
		if pad < 2 {
			result.WriteByte(modifiedBase64[(val>>6)&0x3F])
		}
		if pad < 1 {
			result.WriteByte(modifiedBase64[val&0x3F])
		}
	}

	return result.String()
}

// decodeModifiedBase64 decodes modified base64 to bytes
func decodeModifiedBase64(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, nil
	}

	// Build decode table
	decodeTable := make([]byte, 256)
	for i := range decodeTable {
		decodeTable[i] = 0xFF
	}
	for i, c := range modifiedBase64 {
		decodeTable[c] = byte(i)
	}

	var result []byte
	var val uint32
	var bits int

	for i := 0; i < len(s); i++ {
		b := decodeTable[s[i]]
		if b == 0xFF {
			return nil, errors.New("invalid character in modified base64")
		}

		val = (val << 6) | uint32(b)
		bits += 6

		if bits >= 8 {
			bits -= 8
			result = append(result, byte(val>>bits))
			val &= (1 << bits) - 1
		}
	}

	return result, nil
}

// IsValidUTF8 checks if a string is valid UTF-8
func IsValidUTF8(s string) bool {
	return utf8.ValidString(s)
}
