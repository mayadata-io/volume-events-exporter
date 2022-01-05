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
package rsa

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fakeCreateRSAKeys(size int) (*PrivateKey, *PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, size)
	if err != nil {
		return nil, nil, err
	}
	return &PrivateKey{PrivateKey: privateKey}, &PublicKey{PublicKey: &privateKey.PublicKey}, nil
}

func TestSign(t *testing.T) {
	privateKey, publicKey, err := fakeCreateRSAKeys(2048)
	if err != nil {
		t.Fatalf("failed to get test1 RSA keys error: %v", err)
	}
	mismatchedPrivateKey, _, err := fakeCreateRSAKeys(4096)
	if err != nil {
		t.Fatalf("failed to get test2 RSA keys error: %v", err)
	}
	tests := map[string]struct {
		data                interface{}
		privateKey          *PrivateKey
		publicKey           *PublicKey
		isErrExpected       bool
		isUnsignErrExpected bool
	}{
		"When valid data is given to sign": {
			data: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "pv1"},
			},
			privateKey: privateKey,
			publicKey:  publicKey,
		},
		"When empty data is given to sign": {
			privateKey:    privateKey,
			publicKey:     publicKey,
			isErrExpected: false,
		},
		"When different public key is used to verify signing": {
			data: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "pv2"},
			},
			privateKey:          mismatchedPrivateKey,
			publicKey:           publicKey,
			isUnsignErrExpected: true,
		},
	}
	for name, test := range tests {
		signedBytes, err := test.privateKey.Sign(test.data)
		if err != nil && !test.isErrExpected {
			t.Fatalf("%q test failed expected error not to occur but got error %v", name, err)
		}
		if err == nil && test.isErrExpected {
			t.Fatalf("%q test failed expected error to occur", name)
		}
		if signedBytes != nil {
			dataInBytes, err := json.Marshal(test.data)
			if err != nil {
				t.Fatalf("%q test failed while marshaling the data", name)
			}
			unsignErr := test.publicKey.Unsign(dataInBytes, signedBytes)
			if unsignErr != nil && !test.isUnsignErrExpected {
				t.Fatalf("%q test failed expected error not to occur while unsigning error %v", name, unsignErr)
			}
			if unsignErr == nil && test.isUnsignErrExpected {
				t.Fatalf("%q test failed expected error to occur while unsigning", name)
			}
		}
	}
}
