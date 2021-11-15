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
)

func fakeCreateRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	if path != "" {
		privDer := x509.MarshalPKCS1PrivateKey(privateKey)
		privBlock := pem.Block{
			Type:    "RSA PRIVATE KEY",
			Headers: nil,
			Bytes:   privDer,
		}
		privKeyBytes := pem.EncodeToMemory(&privBlock)
		err = ioutil.WriteFile(path, privKeyBytes, 0600)
		if err != nil {
			return nil, err
		}
	}
	return privateKey, nil
}

func TestLoadPrivateKeyFromPath(t *testing.T) {
	testDir, err := os.MkdirTemp(os.TempDir(), "rsa-keys")
	if err != nil {
		t.Fatalf("failed to create temporary directory, error: %v", err)
	}
	tests := map[string]struct {
		path          string
		isErrExpected bool
	}{
		"When private key exist in RSA PEM format": {
			path: func(path string) string {
				_, err = fakeCreateRSAPrivateKey(path)
				if err != nil {
					t.Fatalf("failed to create RSA public key")
				}
				return path
			}(filepath.Join(testDir, "valid_public_rsa")),
		},
		"When invalid path is given to load private key": {
			path:          filepath.Join(testDir, "invalid_public_key"),
			isErrExpected: true,
		},
		"When path is empty to load private key": {
			path: "",
		},
	}
	for name, test := range tests {
		_, err := LoadPrivateKeyFromPath(test.path)
		if err != nil && !test.isErrExpected {
			t.Fatalf("%s test failed expected error not to occur but got error %v", name, err)
		}
		if err == nil && test.isErrExpected {
			t.Fatalf("%s test failed expected error to occur", name)
		}
	}
	os.RemoveAll(testDir)
}
