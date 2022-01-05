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
	"github.com/pkg/errors"
)

// Signer will create a signature that can be
// used to verify against public key
type Signer interface {
	Sign(obj interface{}) ([]byte, error)
}

// Unsigner will verify signature for given data using public key
type Unsigner interface {
	Unsign(data, signature []byte) error
}

// SignError holds the reason for signing/unsigning errors
// currently, it supports only empty path error
type SignError struct {
	reason errorReason
}

// SignStatus is used to decode the error to verify
// reason of error
type SignStatus interface {
	Reason() errorReason
}

// errorReason defines the cause of error
type errorReason string

const (
	emptyPathError errorReason = "EmptyPath"

	unknownError errorReason = "Unknown"
)

// Error implements error interface
func (s *SignError) Error() string {
	return string(s.reason)
}

// Reason will return cause of error by accessing s.reason
func (s *SignError) Reason() errorReason {
	return s.reason
}

func reasonForError(err error) errorReason {
	if status := SignStatus(nil); errors.As(err, &status) {
		return status.Reason()
	}
	return unknownError
}

func IsEmptyPathError(err error) bool {
	return reasonForError(err) == emptyPathError
}
