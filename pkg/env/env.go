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

package env

import (
	"os"
	"strings"
)

var (
	// NFSServerNamespace defines the namespace of NFS Server resources
	NFSServerNamespace = "OPENEBS_IO_NFS_SERVER_NS"

	// OpenEBSNamespace defines the namespace where pod is running
	// This environment variable set via Kubernetes downward API
	OpenEBSNamespace = "OPENEBS_NAMESPACE"

	// ServerCallBackURL defines the server URL to send volume events
	ServerCallBackURL = "CALLBACK_URL"

	// ServerCallBackToken defines the server authentication token
	ServerCallBackAuthToken = "CALLBACK_TOKEN"
)

func GetNFSServerNamespace() string {
	nfsServerNamespace := os.Getenv(NFSServerNamespace)
	if nfsServerNamespace != "" {
		return nfsServerNamespace
	}
	return os.Getenv(OpenEBSNamespace)
}

func GetCallBackServerURL() string {
	return strings.TrimSpace(os.Getenv(ServerCallBackURL))
}

func GetCallBackServerAuthToken() string {
	return strings.TrimSpace(os.Getenv(ServerCallBackAuthToken))
}
