/*
Copyright © 2021 The MayaData Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sign

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func fakeCreateRSAPublicKey(path string) (*rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	if path != "" {
		publicDer := x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)
		pubBlock := pem.Block{
			Type:    "PUBLIC KEY",
			Headers: nil,
			Bytes:   publicDer,
		}
		publicKeyBytes := pem.EncodeToMemory(&pubBlock)
		err = ioutil.WriteFile(path, publicKeyBytes, 0600)
		if err != nil {
			return nil, err
		}
	}
	return &privateKey.PublicKey, nil
}

func fakeCreateSSHPublicKey(path string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}
	pubKeyBytes := ssh.MarshalAuthorizedKey(publicRsaKey)
	err = ioutil.WriteFile(path, pubKeyBytes, 0600)
	if err != nil {
		return err
	}
	return nil
}

func TestLoadPublicKeyFromPath(t *testing.T) {
	testDir, err := os.MkdirTemp(os.TempDir(), "rsa-keys")
	if err != nil {
		t.Fatalf("failed to create temporary directory, error: %v", err)
	}
	tests := map[string]struct {
		path          string
		isErrExpected bool
	}{
		"When public key exist in RSA PEM format": {
			path: func(path string) string {
				_, err = fakeCreateRSAPublicKey(path)
				if err != nil {
					t.Fatalf("failed to create RSA public key")
				}
				return path
			}(filepath.Join(testDir, "valid_public_rsa")),
		},
		"When invalid path is given to load public key": {
			path:          filepath.Join(testDir, "invalid_public_key"),
			isErrExpected: true,
		},
		"When path is not provided": {
			path: "",
		},
		"When public key exist in SSH format": {
			path: func(path string) string {
				err = fakeCreateSSHPublicKey(path)
				if err != nil {
					t.Fatalf("failed to create SSH public key")
				}
				return path
			}(filepath.Join(testDir, "valid_public_ssh_rsa")),
			isErrExpected: true,
		},
	}
	for name, test := range tests {
		_, err := LoadPublicKeyFromPath(test.path)
		if err != nil && !test.isErrExpected {
			t.Fatalf("%s test failed expected error not to occur but got error %v", name, err)
		}
		if err == nil && test.isErrExpected {
			t.Fatalf("%s test failed expected error to occur", name)
		}
	}
	os.RemoveAll(testDir)
}
