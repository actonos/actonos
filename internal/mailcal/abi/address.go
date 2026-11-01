package abi

import (
	"errors"
	"net/mail"
	"strings"

	"golang.org/x/net/idna"
)

// ErrBadAddress is returned for input that is not a single plain mailbox.
var ErrBadAddress = errors.New("invalid email address")

// NormalizeEmail returns the canonical form used everywhere addresses are compared:
// trimmed, lower-case local part and domain, domain converted to ASCII (punycode).
// Display names, comments, quoted local parts and anything that is not exactly one
// mailbox are rejected, so "Boss <a@b.com>, evil@x.com" can never slip through as one.
func NormalizeEmail(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsAny(s, "<>,;\"() \t\r\n") {
		return "", ErrBadAddress
	}
	parsed, err := mail.ParseAddress(s)
	if err != nil || parsed.Address != s {
		return "", ErrBadAddress
	}
	at := strings.LastIndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return "", ErrBadAddress
	}
	local, domain := strings.ToLower(s[:at]), s[at+1:]
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil || ascii == "" {
		return "", ErrBadAddress
	}
	return local + "@" + strings.ToLower(ascii), nil
}

// DomainOf returns the (already normalized) domain of a normalized email.
func DomainOf(email string) string {
	if at := strings.LastIndexByte(email, '@'); at >= 0 {
		return email[at+1:]
	}
	return ""
}

// NormalizeAddress normalizes the email part and keeps the display name.
func NormalizeAddress(a Address) (Address, error) {
	email, err := NormalizeEmail(a.Email)
	if err != nil {
		return Address{}, err
	}
	return Address{Name: strings.TrimSpace(a.Name), Email: email}, nil
}

// NormalizeAddresses normalizes a list and removes duplicates, keeping first occurrence order.
func NormalizeAddresses(in []Address) ([]Address, error) {
	seen := make(map[string]bool, len(in))
	out := make([]Address, 0, len(in))
	for _, a := range in {
		n, err := NormalizeAddress(a)
		if err != nil {
			return nil, err
		}
		if seen[n.Email] {
			continue
		}
		seen[n.Email] = true
		out = append(out, n)
	}
	return out, nil
}
