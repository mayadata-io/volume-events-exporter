package rsa

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// GenerateKeyPair will generate new key RSA key-pair(private key and public key)
func GenerateKeyPair(bits int) ([]byte, []byte, error) {
	// Key-pair generated using below function will be used to sign the data
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, errors.Errorf("failed to generate RSA key-pair")
	}
	// 	publicKey, err := generatePublicKey(&privateKey.PublicKey)
	// 	if err != nil {
	// 		return nil, nil, errors.Errorf("failed to generate public key")
	// 	}
	return encodePrivateKeyToPEM(privateKey), encodePublicKeyToPEM(&privateKey.PublicKey), nil
}

// encodePrivateKeyToPEM encodes Private Key from RSA to PEM format
func encodePrivateKeyToPEM(privateKey *rsa.PrivateKey) []byte {
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// pem.Block
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	// Private key in PEM format
	privatePEM := pem.EncodeToMemory(&privBlock)

	return privatePEM
}

func encodePublicKeyToPEM(publicKey *rsa.PublicKey) []byte {
	publicDER := x509.MarshalPKCS1PublicKey(publicKey)
	pubBlock := pem.Block{
		Type:    "PUBLIC KEY",
		Headers: nil,
		Bytes:   publicDER,
	}
	return pem.EncodeToMemory(&pubBlock)
}

func generatePublicKey(publicKey *rsa.PublicKey) ([]byte, error) {
	publicRsaKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	return ssh.MarshalAuthorizedKey(publicRsaKey), nil
}
