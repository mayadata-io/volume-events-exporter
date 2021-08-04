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

package tokenauth

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/mayadata-io/volume-events-exporter/pkg/collectorinterface"
	"github.com/mayadata-io/volume-events-exporter/pkg/env"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

const (
	// postMethod is used to send http POST request
	postMethod = "POST"
)

type TokenClient struct {
	// server url holds the URL to communicate with server
	serverURL string

	// serverAuthToken holds the token of the server
	serverAuthToken string

	// Client to interact with server
	client *http.Client
	// VolumeCollector implements methods required for event collector
	collectorinterface.VolumeEventCollector
}

func NewTokenClient(collectorInterface collectorinterface.VolumeEventCollector) collectorinterface.EventsSender {
	return &TokenClient{
		serverURL:            env.GetCallBackServerURL(),
		serverAuthToken:      env.GetCallBackServerAuthToken(),
		client:               &http.Client{},
		VolumeEventCollector: collectorInterface,
	}
}

func (d *TokenClient) Send(data string) error {
	var payload []byte
	var contentType string

	dataType := d.GetDataType()
	switch dataType {
	case collectorinterface.JSONDataType:
		payload = []byte(data)
		contentType = "application/json"
	case collectorinterface.YAMLDataType:
		// TODO: Convert string form of JSON into YAML
		payload = []byte(data)
		contentType = "text/yaml"
	default:
		return errors.Errorf("unsupported data type %s", dataType)
	}

	req, err := http.NewRequest(postMethod, d.serverURL, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Token", d.serverAuthToken)
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			klog.Errorf("failed to decode body error: %v", err)
		}
		return errors.Errorf("failed to post data to server status code: %d status: %s error: %v", resp.StatusCode, resp.Status, string(data))
	}
	return nil
}
