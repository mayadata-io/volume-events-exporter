package rsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
)

// PrivateKey is a wrapper over privatekey of RSA
// which  will implement methods to sign the data
type PrivateKey struct {
	*rsa.PrivateKey
}

// Sign will sign the given data
func (r *PrivateKey) Sign(obj interface{}) ([]byte, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, err = hash.Write(data)
	if err != nil {
		return nil, err
	}
	return rsa.SignPSS(rand.Reader, r.PrivateKey, crypto.SHA256, hash.Sum(nil), nil)
}
