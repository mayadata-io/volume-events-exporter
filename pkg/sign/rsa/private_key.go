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
