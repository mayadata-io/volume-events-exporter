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

package server

import "net/http"

// ServerInterface holds methods which are required to operate server
// ex: gRPC, REST
type ServerInterface interface {
	// Start will run the server in a GO routine and return
	// an error if occurs any
	Start() error

	// Stop will shutdown running server and return an error
	// if occurs any
	Stop() error

	// GetToken will return an valid token which is used to
	// interact with server
	GetToken() string

	// GetEventsReceiverEndpoint will return endpoint where
	// server is serving the request
	GetEventsReceiverEndpoint() string
}

// EventReceiver holds a method which will be triggered upon
// receiving a request from the client(over endpoints)
type EventsReceiver interface {
	ProcessData(req *http.Request) error
}
