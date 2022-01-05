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

package tests

import (
	"flag"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"os"
	"testing"

	"github.com/mayadata-io/volume-events-exporter/pkg/sign"
	"github.com/mayadata-io/volume-events-exporter/tests/nfs"
	"github.com/mayadata-io/volume-events-exporter/tests/server"
	"github.com/mayadata-io/volume-events-exporter/tests/server/rest"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	// auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var (
	// CLI options
	kubeConfigPath string
	ipAddress      string
	port           int
	serverType     string

	serverIface server.ServerInterface

	// Artifacts required configuration
	applicationNamespace        = "event-exporter-tests-ns"
	nfsProvisionerName          = "openebs-nfs-provisioner"
	nfsProvisionerLabelSelector = "openebs.io/component-name=openebs-nfs-provisioner"
	OpenEBSNamespace            = "openebs"
	nfsHookConfigName           = "hook-config"
	nfsHookConfigDataKey        = "hook-config"

	//KeyPVNFSServerType defines if the NFS PV should be launched
	// using kernel or ganesha
	KeyPVNFSServerType = "NFSServerType"

	//KeyPVBackendStorageClass defines default provisioner to be used
	// to create the data(export) directory for NFS server
	KeyPVBackendStorageClass = "BackendStorageClass"

	// integrationTestFinalizer will be configured only on backend PVC.
	// This finalizer is required for test to ensure whether volume events
	// (create/delete) are exported to server, once the server receives a volume
	// event will add received `metadata.name` as an annotation on backend PVC,
	// Since the finalizer exist test will be able to verify annotations
	// of occurred events and if everything is good, test will remove finalizer
	// manually
	integrationTestFinalizer = "it.nfs.openebs.io/test-protection"

	// signingKeyDir holds the path to directory which contains signingKeys
	signingKeyDir string

	// signingPrivateKeyPath holds path to file which contains private key
	signingPrivateKeyPath string

	// signingPublicKeyPath holds path to file which contains public key
	signingPublicKeyPath string

	// fakeSigningPrivateKeyPath holds path to file which contains valid private key
	fakeSigningPrivateKeyPath string
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test application deployment")
}

func init() {
	flag.StringVar(&kubeConfigPath, "kubeconfig", os.Getenv("KUBECONFIG"), "path to kubeconfig to invoke kubernetes API calls")
	flag.StringVar(&ipAddress, "address", "", "address on which server(event listener) will start. Defaults to machine IP Address")
	flag.IntVar(&port, "port", 9090, "port on which server will listen. Defaults to 9090")
	flag.StringVar(&serverType, "type", "rest", "type of the server to serve service. Supported only REST")
}

var _ = BeforeSuite(func() {
	var err error
	// flag.Parse()

	if err := initK8sClient(kubeConfigPath); err != nil {
		panic(fmt.Sprintf("failed to initialize k8s client err=%s", err))
	}
	if ipAddress == "" {
		ipAddress, err = externalIP()
		if err != nil {
			panic(fmt.Sprintf("failed to get externalIP address, err: %s", err))
		}
	}

	signingKeyDir, err = createSingingKeyDirectory()
	Expect(err).To(BeNil(), "while creating signing directory")

	err = generateRSAKeys()
	Expect(err).To(BeNil(), "while generating RSA key pairs")

	serverIface, err = newServer(ipAddress, port, serverType)
	Expect(err).To(BeNil(), "while instantiating the new server")
	err = serverIface.Start()
	Expect(err).To(BeNil(), "while starting the server")

	By("waiting for openebs-nfs-provisioner pod to come into running state")
	err = Client.waitForPods(OpenEBSNamespace, nfsProvisionerLabelSelector, corev1.PodRunning, 1)
	Expect(err).To(BeNil(), "while waiting for nfs deployment to be ready")

	err = updateNFSHookConfig(OpenEBSNamespace, nfsHookConfigName)
	Expect(err).To(BeNil(), "while updating nfs hook configuration as required per test")

	err = addEventControllerSideCar(OpenEBSNamespace, nfsProvisionerName)
	Expect(err).To(BeNil(), "while adding volume-event-exporter sidecar")

	By("building a namespace")
	err = Client.createNamespace(applicationNamespace)
	Expect(err).To(BeNil(), "while creating namespace {%s}", applicationNamespace)
})

var _ = AfterSuite(func() {
	if Client != nil {
		By("deleting namespace")
		err := Client.destroyNamespace(applicationNamespace)
		Expect(err).To(BeNil(), "while deleting namespace {%s}", applicationNamespace)
	}
	if serverIface != nil {
		err := serverIface.Stop()
		Expect(err).To(BeNil(), "while stopping the server")
	}

})

func newServer(address string, port int, serverType string) (server.ServerInterface, error) {
	unsigner, err := sign.LoadPublicKeyFromPath(signingPublicKeyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load public key")
	}
	switch serverType {
	case "rest":
		return rest.NewRestServer(rest.ServerConfig{
			IPAddress:  address,
			Port:       port,
			SecretKey:  "mayadata-io-secret",
			TLSTimeout: 2 * time.Hour,
			Clientset:  Client.Interface,
			EventsReceiver: &nfs.NFS{
				Clientset: Client.Interface,
				Unsigner:  unsigner,
			},
		})
	}
	return nil, errors.Errorf("Unsupported server type %s", serverType)
}

// externalIP will fetch the IP from ifconfig
func externalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("are you connected to the network?")
}

func createSingingKeyDirectory() (string, error) {
	if signingKeyDir != "" {
		return signingKeyDir, nil
	}
	dirPath, err := os.MkdirTemp(os.TempDir(), "rsa-keys")
	if err != nil {
		return "", err
	}
	signingKeyDir = dirPath
	signingPrivateKeyPath = filepath.Join(signingKeyDir, "id_rsa_private")
	signingPublicKeyPath = filepath.Join(signingKeyDir, "id_rsa.pub")
	fakeSigningPrivateKeyPath = filepath.Join(signingKeyDir, "id_rsa_fake_private")
	return signingKeyDir, nil
}
