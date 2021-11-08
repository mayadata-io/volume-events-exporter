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

// LoadPrivateKeyFromPath will return appropriate signer from
// private key
func LoadPrivateKeyFromPath(path string) (Signer, error) {
	if path == "" {
		return &empty.PrivateKey{}, nil
	}
	privateKeyInBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read private key")
	}
	return parsePrivateKey(privateKeyInBytes)
}

func parsePrivateKey(pemBytes []byte) (Signer, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("ssh: no key found")
	}

	var rawkey interface{}
	switch block.Type {
	case "RSA PRIVATE KEY":
		rsa, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rawkey = rsa
	default:
		return nil, errors.Errorf("ssh: unsupported key type %q", block.Type)
	}
	return newSignerFromKey(rawkey)
}

func newSignerFromKey(key interface{}) (Signer, error) {
	switch t := key.(type) {
	case *rsa.PrivateKey:
		// Validate the given key
		err := t.Validate()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to validate private key")
		}
		return &rsawrapper.PrivateKey{
			PrivateKey: t,
		}, nil
	default:
		return nil, errors.Errorf("Unsupported key type %T", key)
	}
}
