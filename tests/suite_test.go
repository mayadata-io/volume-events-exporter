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
	"time"

	"os"
	"testing"

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

	//KeyPVNFSServerType defines if the NFS PV should be launched
	// using kernel or ganesha
	KeyPVNFSServerType = "NFSServerType"

	//KeyPVBackendStorageClass defines default provisioner to be used
	// to create the data(export) directory for NFS server
	KeyPVBackendStorageClass = "BackendStorageClass"
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
	flag.Parse()

	if err := initK8sClient(kubeConfigPath); err != nil {
		panic(fmt.Sprintf("failed to initialize k8s client err=%s", err))
	}
	if ipAddress == "" {
		ipAddress, err = externalIP()
		if err != nil {
			panic(fmt.Sprintf("failed to get externalIP address, err: %s", err))
		}
	}

	serverIface, err = newServer(ipAddress, port, serverType)
	Expect(err).To(BeNil(), "while instantiating the new server")
	err = serverIface.Start()
	Expect(err).To(BeNil(), "while starting the server")

	By("waiting for openebs-nfs-provisioner pod to come into running state")
	err = Client.waitForPods(OpenEBSNamespace, nfsProvisionerLabelSelector, corev1.PodRunning, 1)
	Expect(err).To(BeNil(), "while waiting for nfs deployment to be ready")

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
			},
		})
	}
	return nil, errors.Errorf("Unsupported server type %s", serverType)
}

// addOrUpdateEventControllerSidecar will add volume-event-controller side car only
// if container doesn't exist else updates the CALLBACK_URL and CALLBACK_TOKEN
func addEventControllerSideCar(deploymentNamespace, deploymentName string) error {
	deployObj, err := Client.getDeployment(deploymentNamespace, deploymentName)
	if err != nil {
		return err
	}
	var isVolumeEventsCollectorExist bool
	volumeEventsCollector := corev1.Container{
		Name:  "volume-events-collector",
		Image: "mayadataio/volume-events-exporter:ci",
		Args: []string{
			"--leader-election=false",
			"--generate-k8s-events=true",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "OPENEBS_IO_NFS_SERVER_NS",
				Value: OpenEBSNamespace,
			},
			{
				Name:  "CALLBACK_URL",
				Value: serverIface.GetEventsReceiverEndpoint(),
			},
			{
				Name:  "CALLBACK_TOKEN",
				Value: serverIface.GetToken(),
			},
		},
	}

	for idx, container := range deployObj.Spec.Template.Spec.Containers {
		if container.Name == volumeEventsCollector.Name {
			deployObj.Spec.Template.Spec.Containers[idx] = volumeEventsCollector
			isVolumeEventsCollectorExist = true
			break
		}
	}
	if !isVolumeEventsCollectorExist {
		deployObj.Spec.Template.Spec.Containers = append(deployObj.Spec.Template.Spec.Containers, volumeEventsCollector)
	}
	updatedDeployObj, err := Client.updateDeployment(deployObj)
	if err != nil {
		return err
	}
	return Client.waitForDeploymentRollout(updatedDeployObj.Namespace, updatedDeployObj.Name)
}

func removeEventsCollectorSidecar(deploymentNamespace, deploymentName string) error {
	var isVolumeEventsCollectorExist bool
	var index int

	deployObj, err := Client.getDeployment(deploymentNamespace, deploymentName)
	if err != nil {
		return err
	}

	for idx, container := range deployObj.Spec.Template.Spec.Containers {
		if container.Name == "volume-events-collector" {
			index = idx
			isVolumeEventsCollectorExist = true
		}
	}
	// Remove volume events collector sidecar
	if !isVolumeEventsCollectorExist {
		return nil
	}

	deployObj.Spec.Template.Spec.Containers = append(deployObj.Spec.Template.Spec.Containers[:index], deployObj.Spec.Template.Spec.Containers[index+1:]...)
	updatedDeployObj, err := Client.updateDeployment(deployObj)
	if err != nil {
		return err
	}
	return Client.waitForDeploymentRollout(updatedDeployObj.Namespace, updatedDeployObj.Name)
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
