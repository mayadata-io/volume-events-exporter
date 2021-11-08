package sign

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"

	"github.com/mayadata-io/volume-events-exporter/pkg/encrypt/empty"
	rsawrapper "github.com/mayadata-io/volume-events-exporter/pkg/encrypt/rsa"
	"github.com/pkg/errors"
)

// LoadPublicKeyFromPath will load and parse PEM public key from given path
func LoadPublicKeyFromPath(path string) (Unsigner, error) {
	if path == "" {
		return &empty.PublicKey{}, nil
	}
	publicKeyInBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read public key")
	}
	return parsePublicKey(publicKeyInBytes)
}

func parsePublicKey(pemBytes []byte) (Unsigner, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.Errorf("No public key found")
	}

	var rawkey interface{}
	switch block.Type {
	case "PUBLIC KEY":
		rsa, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rawkey = rsa
	default:
		return nil, errors.Errorf("ssh: unsupported key type %q", block.Type)
	}
	return newUnsignerFromKey(rawkey)
}

func newUnsignerFromKey(key interface{}) (Unsigner, error) {
	switch t := key.(type) {
	case *rsa.PublicKey:
		return &rsawrapper.PublicKey{
			PublicKey: t,
		}, nil
	default:
		return nil, errors.Errorf("Unsupported public key type %T", key)
	}
}
