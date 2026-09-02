package chain

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Addresses.
//
// An address is the one input a person types, and a typo sends money to
// nobody forever. EIP-55 mixes the case of the hex digits as a checksum; an
// all-lowercase address carries no checksum and is accepted as written, a
// mixed-case one is refused unless its case is exactly right.

// ValidAddress reports whether s is a 20-byte hex address, and if its letters
// are mixed case, whether that case is a correct EIP-55 checksum.
func ValidAddress(s string) error {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return fmt.Errorf("an address starts with 0x")
	}
	body := s[2:]
	if len(body) != 40 {
		return fmt.Errorf("an address is 40 hex characters after 0x, this one is %d", len(body))
	}
	if _, err := hex.DecodeString(body); err != nil {
		return fmt.Errorf("an address is hex; %q is not", body)
	}
	lower, upper := strings.ToLower(body), strings.ToUpper(body)
	if body == lower || body == upper {
		return nil
	}
	if Checksum(s) != "0x"+body {
		return fmt.Errorf("the address fails its checksum; one character is wrong")
	}
	return nil
}

// Checksum returns the EIP-55 mixed-case form of an address.
func Checksum(addr string) string {
	body := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(addr, "0x"), "0X"))
	h := Keccak256([]byte(body))
	hh := hex.EncodeToString(h[:])
	out := make([]byte, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c >= 'a' && c <= 'f' && hh[i] >= '8' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return "0x" + string(out)
}

// sameAddress compares two addresses regardless of case.
func sameAddress(a, b string) bool {
	return strings.EqualFold(strings.TrimPrefix(a, "0x"), strings.TrimPrefix(b, "0x"))
}

// topicFor pads an address to the 32-byte form it takes in an event topic.
func topicFor(addr string) string {
	body := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(addr, "0x"), "0X"))
	return "0x" + strings.Repeat("0", 64-len(body)) + body
}

// addressFromTopic reads the address out of a 32-byte topic.
func addressFromTopic(topic string) string {
	t := strings.TrimPrefix(topic, "0x")
	if len(t) < 40 {
		return ""
	}
	return Checksum("0x" + t[len(t)-40:])
}
