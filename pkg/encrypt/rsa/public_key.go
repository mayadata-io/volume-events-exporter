package rsa

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
)

// PublicKey is wrapper over rsa public key
// used to verify given signature
type PublicKey struct {
	*rsa.PublicKey
}

// Unsign will verify the signatures
func (p *PublicKey) Unsign(data, sig []byte) error {
	hash := sha256.New()
	hash.Write(data)
	return rsa.VerifyPSS(p.PublicKey, crypto.SHA256, hash.Sum(nil), sig, nil)
}
