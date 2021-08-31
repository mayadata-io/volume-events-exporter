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
	"context"
	"strconv"
	"time"

	"net/http"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/mayadata-io/volume-events-exporter/tests/server"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// ServerConfig will instantiate the new rest server
type ServerConfig struct {

	// IPAddress holds address of the REST server
	IPAddress string

	// Port holds the port number on which server needs to run
	Port int

	// SecreteKey defines the secret key to communicate with the server
	SecretKey string

	// TLSTimeout defines the timeout expiry timeout of generated token
	TLSTimeout time.Duration

	// Clientset is used to interact with Kube-APIServer
	Clientset kubernetes.Interface

	// EventsReceiver to process events
	EventsReceiver server.EventsReceiver
}

type restServer struct {
	service *service
	// token will be generated and stored in-memory and it can be used
	// if integration test case required to interact with server
	token string
}

func NewRestServer(config ServerConfig) (server.ServerInterface, error) {
	token, err := getToken(config.SecretKey, config.TLSTimeout)
	if err != nil {
		return nil, err
	}
	r := &restServer{
		service: &service{
			clientset: config.Clientset,
			httpServer: &http.Server{
				Addr: config.IPAddress + ":" + strconv.Itoa(config.Port),
			},
			secretKey:     config.SecretKey,
			dataProcessor: config.EventsReceiver,
		},
		token: token,
	}

	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/event-server", r.service.isAuthorized(r.service.eventsHandler)).Methods("POST")
	r.service.httpServer.Handler = router

	return r, nil
}

// Start will start the REST Service
// as various endpoints to acknowledge data
// NOTE: Stop must be called after shuting down the node
func (r *restServer) Start() error {
	go func() {
		err := r.service.httpServer.ListenAndServe()
		if err != nil {
			klog.Warningf("error: %s", err.Error())
			return
		}
	}()
	return nil
}

// Stop will stop the running service by calling
// shutdown. If there are no active listeners
// call will be returned immediately else it
// will take 1minute to return
func (r *restServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.TODO(), 1*time.Minute)
	defer cancel()
	return r.service.httpServer.Shutdown(ctx)
}

// GetToken will return the token required that is required
// to interact with the server
func (r *restServer) GetToken() string {
	return r.token
}

func (r *restServer) GetEventsReceiverEndpoint() string {
	return "http://" + r.service.httpServer.Addr + "/event-server"
}

func getToken(secretKey string, tlsTimeout time.Duration) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)

	claims["authorized"] = true
	claims["user"] = "rest service"
	claims["exp"] = time.Now().Add(tlsTimeout).Unix()
	tokenString, err := token.SignedString([]byte(secretKey))
	if err != nil {
		return "", errors.Wrapf(err, "something went wrong")
	}
	return tokenString, nil
}
