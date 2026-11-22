package app

import (
	"fmt"
	"strings"
)

// Bech32 (BIP-173) encoding for Cosmos-style addresses. It is written here, not imported, so
// address validation has no third-party dependency. The checksum is verified on every decode.

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

var bech32Generator = [5]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}

func bech32Polymod(values []byte) uint32 {
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := 0; i < 5; i++ {
			if (top>>uint(i))&1 == 1 {
				chk ^= bech32Generator[i]
			}
		}
	}
	return chk
}

func bech32HRPExpand(hrp string) []byte {
	out := make([]byte, 0, len(hrp)*2+1)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]>>5)
	}
	out = append(out, 0)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]&31)
	}
	return out
}

// convertBits regroups bits between 8-bit bytes and 5-bit groups.
func convertBits(data []byte, from, to uint, pad bool) ([]byte, error) {
	acc, bits := uint32(0), uint(0)
	maxV := uint32(1)<<to - 1
	var out []byte
	for _, v := range data {
		if uint32(v)>>from != 0 {
			return nil, fmt.Errorf("invalid data range")
		}
		acc = acc<<from | uint32(v)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte((acc>>bits)&maxV))
		}
	}
	if pad {
		if bits > 0 {
			out = append(out, byte((acc<<(to-bits))&maxV))
		}
	} else if bits >= from || (acc<<(to-bits))&maxV != 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	return out, nil
}

// Bech32Encode encodes data (raw bytes) under the human-readable part hrp.
func Bech32Encode(hrp string, data []byte) (string, error) {
	five, err := convertBits(data, 8, 5, true)
	if err != nil {
		return "", err
	}
	values := append(bech32HRPExpand(hrp), five...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	mod := bech32Polymod(values) ^ 1
	var b strings.Builder
	b.WriteString(hrp)
	b.WriteByte('1')
	for _, v := range five {
		b.WriteByte(bech32Charset[v])
	}
	for i := 0; i < 6; i++ {
		b.WriteByte(bech32Charset[(mod>>uint(5*(5-i)))&31])
	}
	return b.String(), nil
}

// Bech32Decode verifies the checksum and returns the HRP and the decoded bytes.
func Bech32Decode(s string) (string, []byte, error) {
	if strings.ToLower(s) != s && strings.ToUpper(s) != s {
		return "", nil, fmt.Errorf("mixed case")
	}
	s = strings.ToLower(s)
	pos := strings.LastIndexByte(s, '1')
	if pos < 1 || pos+7 > len(s) || len(s) > 90 {
		return "", nil, fmt.Errorf("malformed bech32 string")
	}
	hrp := s[:pos]
	var data []byte
	for i := pos + 1; i < len(s); i++ {
		idx := strings.IndexByte(bech32Charset, s[i])
		if idx < 0 {
			return "", nil, fmt.Errorf("invalid character %q", s[i])
		}
		data = append(data, byte(idx))
	}
	if bech32Polymod(append(bech32HRPExpand(hrp), data...)) != 1 {
		return "", nil, fmt.Errorf("checksum mismatch")
	}
	raw, err := convertBits(data[:len(data)-6], 5, 8, false)
	if err != nil {
		return "", nil, err
	}
	return hrp, raw, nil
}

// ValidAddress reports whether s is a well-formed bech32 address with the given prefix and a
// 20-byte payload, as Cosmos account addresses are.
func ValidAddress(s, hrp string) error {
	gotHRP, payload, err := Bech32Decode(s)
	if err != nil {
		return err
	}
	if gotHRP != hrp {
		return fmt.Errorf("prefix %q, want %q", gotHRP, hrp)
	}
	if len(payload) != 20 {
		return fmt.Errorf("payload is %d bytes, want 20", len(payload))
	}
	return nil
}
