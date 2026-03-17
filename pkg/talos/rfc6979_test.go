// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"testing"
)

// TestRFC6979SignTestVectors validates the RFC 6979 implementation against
// the official test vectors from RFC 6979 Appendix A.2.5 (ECDSA P-256 / SHA-256).
func TestRFC6979SignTestVectors(t *testing.T) {
	t.Parallel()

	// Private key from RFC 6979 A.2.5.
	xHex := "C9AFA9D845BA75166B5C215767B1D6934E50C3DB36E89B127B8A622B120F6721"

	xBytes, err := hex.DecodeString(xHex)
	if err != nil {
		t.Fatalf("failed to decode test private key hex: %v", err)
	}

	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), xBytes)
	if err != nil {
		t.Fatalf("failed to parse test private key: %v", err)
	}

	tests := []struct {
		name      string
		message   string
		expectedR string
		expectedS string
	}{
		{
			name:      "sample",
			message:   "sample",
			expectedR: "EFD48B2AACB6A8FD1140DD9CD45E81D69D2C877B56AAF991C34D0EA84EAF3716",
			expectedS: "F7CB1C942D657C41D436C7A1B6E29F65F3E900DBB9AFF4064DC4AB2F843ACDA8",
		},
		{
			name:      "test",
			message:   "test",
			expectedR: "F1ABB023518351CD71D881567B1EA663ED3EFCF6C5132B354F28D3B0B7D38367",
			expectedS: "019F4113742A2B14BD25926B49C649155F267E60D3814B4C0CC84250E46F0083",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hash := sha256.Sum256([]byte(tc.message))
			r, s := rfc6979Sign(key, hash[:])

			expectedR, _ := new(big.Int).SetString(tc.expectedR, 16)
			expectedS, _ := new(big.Int).SetString(tc.expectedS, 16)

			if r.Cmp(expectedR) != 0 {
				t.Errorf("r mismatch for %q:\n  got:  %064X\n  want: %064X", tc.message, r, expectedR)
			}

			if s.Cmp(expectedS) != 0 {
				t.Errorf("s mismatch for %q:\n  got:  %064X\n  want: %064X", tc.message, s, expectedS)
			}
		})
	}
}
