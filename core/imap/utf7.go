package imap

import (
	"errors"
	"strings"
	"unicode/utf16"
)

// DecodeUTF7 decodes an IMAP modified UTF-7 string to UTF-8.
func DecodeUTF7(s string) (string, error) {
	if s == "" {
		return "", nil
	}

	var result strings.Builder
	i := 0

	for i < len(s) {
		if s[i] == '&' && i+1 < len(s) {
			if s[i+1] == '-' {
				result.WriteRune('&')
				i += 2
				continue
			}

			end := i + 1
			for end < len(s) && s[end] != '-' {
				end++
			}
			if end >= len(s) {
				return "", errors.New("unterminated UTF-7 sequence")
			}

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

// EncodeUTF7 encodes a UTF-8 string to IMAP modified UTF-7.
func EncodeUTF7(s string) string {
	if s == "" {
		return ""
	}

	var result strings.Builder
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		start := i
		for i < len(runes) && runes[i] > 0x7F {
			i++
		}
		if i > start {
			result.WriteString(encodeUTF7Sequence(runes[start:i]))
		}

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

func encodeUTF7Sequence(runes []rune) string {
	var utf16Data []uint16
	for _, r := range runes {
		if r <= 0xFFFF {
			utf16Data = append(utf16Data, uint16(r))
		} else {
			r -= 0x10000
			utf16Data = append(utf16Data, uint16((r>>10)+0xD800), uint16((r&0x3FF)+0xDC00))
		}
	}

	var b []byte
	for _, u := range utf16Data {
		b = append(b, byte(u>>8), byte(u))
	}
	return "&" + encodeModifiedBase64(b) + "-"
}

func decodeUTF7Sequence(s string) (string, error) {
	b, err := decodeModifiedBase64(s)
	if err != nil {
		return "", err
	}
	if len(b)%2 != 0 {
		return "", errors.New("invalid UTF-7 sequence: odd number of bytes")
	}

	utf16Data := make([]uint16, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		utf16Data[i/2] = uint16(b[i])<<8 | uint16(b[i+1])
	}
	return string(utf16.Decode(utf16Data)), nil
}

const modifiedBase64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+,"

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

func decodeModifiedBase64(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, nil
	}

	var decodeTable [256]byte
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
