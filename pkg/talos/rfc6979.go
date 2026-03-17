// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

// RFC 6979 deterministic ECDSA nonce generation for P-256.
// This produces signatures that are fully deterministic (no randomness),
// which is required because Go 1.26+ ignores custom io.Reader in crypto/ecdsa.

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/asn1"
	"io"
	"math/big"
)

// deterministicECDSASigner wraps an ECDSA private key and produces RFC 6979
// deterministic signatures instead of using random nonces.
type deterministicECDSASigner struct {
	key *ecdsa.PrivateKey
}

func (s *deterministicECDSASigner) Public() crypto.PublicKey {
	return &s.key.PublicKey
}

func (s *deterministicECDSASigner) Sign(_ io.Reader, digest []byte, _ crypto.SignerOpts) ([]byte, error) {
	r, ss := rfc6979Sign(s.key, digest)

	return asn1.Marshal(struct {
		R, S *big.Int
	}{r, ss})
}

// rfc6979Sign computes an ECDSA signature using the deterministic nonce
// generation algorithm from RFC 6979, Section 3.2.
func rfc6979Sign(key *ecdsa.PrivateKey, hash []byte) (*big.Int, *big.Int) {
	curve := key.Curve
	n := curve.Params().N
	qLen := (n.BitLen() + 7) / 8 // byte length of the curve order

	// Use Bytes() instead of the deprecated D field.
	// Bytes never returns an error for valid keys.
	keyBytes, _ := key.Bytes() //nolint:errcheck
	d := new(big.Int).SetBytes(keyBytes)

	// int2octets: private key as a fixed-length big-endian integer.
	x := keyBytes

	// bits2octets: reduce hash modulo n, then encode as fixed-length.
	h1 := bits2octets(hash, n, qLen)

	// Section 3.2 steps a-f: initialize HMAC_DRBG.
	hLen := sha256.Size // 32 for SHA-256
	v := make([]byte, hLen)
	k := make([]byte, hLen)

	for i := range v {
		v[i] = 0x01
	}

	// Step d: K = HMAC_K(V || 0x00 || int2octets(x) || bits2octets(h1))
	k = hmacSHA256(k, v, []byte{0x00}, x, h1)
	// Step e: V = HMAC_K(V)
	v = hmacSHA256(k, v)
	// Step f: K = HMAC_K(V || 0x01 || int2octets(x) || bits2octets(h1))
	k = hmacSHA256(k, v, []byte{0x01}, x, h1)
	// Step g: V = HMAC_K(V)
	v = hmacSHA256(k, v)

	// Step h: generate k candidates until valid.
	for {
		// For P-256 with SHA-256, one HMAC output (32 bytes) = qLen.
		v = hmacSHA256(k, v)
		kCandidate := bits2int(v, n)

		if kCandidate.Sign() > 0 && kCandidate.Cmp(n) < 0 {
			r, ss := rawECDSASign(curve, n, d, kCandidate, hash)
			if r.Sign() > 0 && ss.Sign() > 0 {
				return r, ss
			}
		}

		// Not valid, update K and V per RFC 6979 step h.3.
		k = hmacSHA256(k, v, []byte{0x00})
		v = hmacSHA256(k, v)
	}
}

// rawECDSASign computes (r, s) given a nonce k, following the standard ECDSA
// signature algorithm.
func rawECDSASign(curve elliptic.Curve, n, d, k *big.Int, hash []byte) (*big.Int, *big.Int) {
	// R = k * G
	// No non-deprecated API exposes the x-coordinate of a scalar multiplication.
	rx, _ := curve.ScalarBaseMult(k.Bytes()) //nolint:staticcheck
	r := new(big.Int).Mod(rx, n)

	if r.Sign() == 0 {
		return r, new(big.Int)
	}

	// s = k^-1 * (hash + r * d) mod n
	e := bits2int(hash, n)
	s := new(big.Int).Mul(r, d)
	s.Add(s, e)

	kInv := new(big.Int).ModInverse(k, n)
	s.Mul(s, kInv)
	s.Mod(s, n)

	return r, s
}

// hmacSHA256 computes HMAC-SHA256(key, data...).
func hmacSHA256(key []byte, data ...[]byte) []byte {
	h := hmac.New(sha256.New, key)
	for _, d := range data {
		h.Write(d) //nolint:errcheck
	}

	return h.Sum(nil)
}

// int2octets converts a big.Int to a fixed-length (qLen) big-endian byte slice.
func int2octets(v *big.Int, qLen int) []byte {
	out := v.Bytes()
	if len(out) >= qLen {
		return out[:qLen]
	}

	padded := make([]byte, qLen)
	copy(padded[qLen-len(out):], out)

	return padded
}

// bits2int interprets a byte slice as a big-endian integer and reduces to
// the bit length of n.
func bits2int(b []byte, n *big.Int) *big.Int {
	v := new(big.Int).SetBytes(b)
	excess := len(b)*8 - n.BitLen()

	if excess > 0 {
		v.Rsh(v, uint(excess))
	}

	return v
}

// bits2octets converts a hash to a fixed-length byte slice reduced modulo n.
func bits2octets(hash []byte, n *big.Int, qLen int) []byte {
	z := bits2int(hash, n)
	if z.Cmp(n) >= 0 {
		z.Sub(z, n)
	}

	return int2octets(z, qLen)
}
