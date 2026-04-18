package utils

import (
	"unicode/utf8"

	corevimap "flymail-core/imap"
)

// EncodeUTF7 encodes a string to modified UTF-7 for IMAP.
// Delegates to core/imap.EncodeUTF7.
func EncodeUTF7(s string) string {
	return corevimap.EncodeUTF7(s)
}

// DecodeUTF7 decodes a modified UTF-7 string from IMAP.
// Delegates to core/imap.DecodeUTF7.
func DecodeUTF7(s string) (string, error) {
	return corevimap.DecodeUTF7(s)
}

// IsValidUTF8 checks if a string is valid UTF-8.
func IsValidUTF8(s string) bool {
	return utf8.ValidString(s)
}
