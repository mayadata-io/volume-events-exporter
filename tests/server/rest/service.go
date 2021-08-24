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

package rest

import (
	"fmt"
	"net/http"

	"github.com/dgrijalva/jwt-go"
	"github.com/mayadata-io/volume-events-exporter/tests/server"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

// Service implements the endpoints that are
// required to full fill the integration test needs.
//
// Endpoints:-
// event-server(eventsHandler): Endpoint which will receives the volume create and
//				 delete events. When events are received service will
//				 update the respective CR's with finalizer{it.openebs.io/volume-create-protection,
//				 it.openebs.io/volume-destroy-protection}. This finalizer on
//				 CR's ensure that REST service received the request.
//				 TODO: Do we need to received data in-memory?

type service struct {
	clientset     kubernetes.Interface
	httpServer    *http.Server
	secretKey     string
	dataProcessor server.EventsReceiver
}

// isAuthorized is a token based authentication check that client should pass the token in request Header
func (s *service) isAuthorized(endpointHandler func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header["Token"] != nil {
			token, err := jwt.Parse(r.Header["Token"][0], func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, errors.Errorf("Unable to authorize")
				}
				return []byte(s.secretKey), nil
			})
			if err != nil {
				fmt.Fprintf(w, "Failed to parse token error: %s", err.Error())
			}

			if token.Valid {
				endpointHandler(w, r)
			}
		} else {
			fmt.Fprintf(w, "UnAuthorized!! Access Denied")
		}
	})
}

func (s *service) eventsHandler(resp http.ResponseWriter, req *http.Request) {
	var httpCode int
	var message string

	klog.Infof("Received event handler event to process data")
	err := s.dataProcessor.ProcessData(req)
	if err != nil {
		httpCode = 500
		message = errors.Wrapf(err, "failed to process data").Error()
		resp.WriteHeader(httpCode)
		_, err = resp.Write([]byte(message))
		if err != nil {
			klog.Errorf("Failed to send error response: %s error: %v", message, err)
		}
		return
	}
	httpCode = 200
	message = "Ok"
	resp.WriteHeader(httpCode)
	_, err = resp.Write([]byte(message))
	if err != nil {
		klog.Errorf("Failed to send response: %s error: %v", message, err)
	}
}
