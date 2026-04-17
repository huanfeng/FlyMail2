package types

import (
	"fmt"
	"net/mail"
	"strings"
)

// Address represents a parsed email address with display name.
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// String formats the address as "Name <email>" or just "email".
func (a Address) String() string {
	if a.Name != "" {
		return fmt.Sprintf("%s <%s>", a.Name, a.Email)
	}
	return a.Email
}

// FormatAddressList formats a slice of Address into a comma-separated string.
func FormatAddressList(addrs []Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, ", ")
}

// ParseAddress parses a single RFC 5322 address string like "Name <user@example.com>".
func ParseAddress(s string) (Address, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Address{}, fmt.Errorf("empty address")
	}

	addr, err := mail.ParseAddress(s)
	if err != nil {
		// Fallback: treat the whole string as a bare email
		if strings.Contains(s, "@") {
			return Address{Email: s}, nil
		}
		return Address{}, fmt.Errorf("invalid address %q: %w", s, err)
	}
	return Address{Name: addr.Name, Email: addr.Address}, nil
}

// ParseAddressList parses a comma-separated list of RFC 5322 addresses.
func ParseAddressList(s string) []Address {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	parsed, err := mail.ParseAddressList(s)
	if err != nil {
		// Fallback: split by comma and try individually
		parts := strings.Split(s, ",")
		var result []Address
		for _, p := range parts {
			if a, err := ParseAddress(p); err == nil {
				result = append(result, a)
			}
		}
		return result
	}

	result := make([]Address, 0, len(parsed))
	for _, a := range parsed {
		result = append(result, Address{Name: a.Name, Email: a.Address})
	}
	return result
}
