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
	"fmt"
	"time"

	"github.com/ghodss/yaml"
	"github.com/mayadata-io/volume-events-exporter/tests/nfs"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TEST DELETE EVENTS FOR RELEASED NFS PV WHILE EXPORTER IS DISABLED", func() {
	var (
		// PVC configuration
		applicationNamespace = "default"
		accessModes          = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		capacity             = "1Gi"
		pvcName              = "delete-event-released-nfs-pv"
		scName               = "delete-released-pv-sc"
		nfsPVName            string
		backendPVCName       string
		backendPVName        string

		scNfsServerType = "kernel"

		maxRetryCount = 15
	)

	When("Create storageclass with Reclaim policy set to Retain", func() {
		It("should create storageclass", func() {
			By("creating storageclass")
			scReclaimPolicy := corev1.PersistentVolumeReclaimRetain

			casObj := []Config{
				{
					Name:  KeyPVNFSServerType,
					Value: scNfsServerType,
				},
			}

			casObjStr, err := yaml.Marshal(casObj)
			Expect(err).To(BeNil(), "while marshaling cas object")

			err = Client.createStorageClass(&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: scName,
					Annotations: map[string]string{
						"openebs.io/cas-type":   "nfsrwx",
						"cas.openebs.io/config": string(casObjStr),
					},
				},
				Provisioner:   "openebs.io/nfsrwx",
				ReclaimPolicy: &scReclaimPolicy,
			})
			Expect(err).To(BeNil(), "while creating SC{%s}", scName)
		})
	})

	When("PVC with storageclass "+scName+"is created", func() {
		It("should create a pvc ", func() {
			By("creating above pvc")
			err := Client.createPVC(&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: applicationNamespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
					AccessModes:      accessModes,
					Resources: corev1.ResourceRequirements{
						Requests: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceStorage: resource.MustParse(capacity),
						},
					},
				},
			})
			Expect(err).To(BeNil(), "while creating pvc %s/%s", applicationNamespace, pvcName)

			pvcPhase, err := Client.waitForPVCBound(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while waiting for pvc %s/%s bound phase", applicationNamespace, pvcName)
			Expect(pvcPhase).To(Equal(corev1.ClaimBound), "pvc %s/%s should be in bound phase", applicationNamespace, pvcName)

			pvcObj, err := Client.getPVC(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while fetching pvc %s/%s", applicationNamespace, pvcName)
			nfsPVName = pvcObj.Spec.VolumeName
			backendPVCName = "nfs-" + nfsPVName

			nfsServerLabelSelector := "openebs.io/nfs-server=nfs-" + pvcObj.Spec.VolumeName
			err = Client.waitForPods(OpenEBSNamespace, nfsServerLabelSelector, corev1.PodRunning, 1)
			Expect(err).To(BeNil(), "while verifying NFS Server running status")

			backendPVCObj, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while fetching pvc %s/%s", OpenEBSNamespace, backendPVCName)
			backendPVName = backendPVCObj.Spec.VolumeName
		})
	})

	When("PVC is deleted", func() {
		It("should delete pvc", func() {
			Expect(backendPVCName).NotTo(BeEmpty(), "Backend PVC name should not be empty")

			err := Client.deletePVC(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while deleting pvc %s/%s", applicationNamespace, pvcName)

			isPvcDeleted := false
			for retry := 0; retry < maxRetryCount; retry++ {
				_, err = Client.getPVC(applicationNamespace, pvcName)
				if err != nil && k8serrors.IsNotFound(err) {
					isPvcDeleted = true
					break
				}

				time.Sleep(time.Second * 5)
			}
			Expect(isPvcDeleted).To(BeTrue(), "pvc should be deleted")

			var isDeleteEventReceived bool
			for retry := 5; retry >= 0; retry-- {
				backendPVCObj, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
				Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

				_, isDeleteEventReceived = backendPVCObj.Annotations[nfs.VolumeDeleteNFSPVKey]
				if isDeleteEventReceived {
					break
				}
				time.Sleep(time.Second * 5)
			}
			Expect(isDeleteEventReceived).To(BeFalse(), "When NFS pvc %s/%s with retain policy is deleted it should not send events", applicationNamespace, pvcName)
		})
	})

	When("Remove volume-events-collector sidecar", func() {
		It("should remove volume-events-collector sidecar", func() {
			err := removeEventsCollectorSidecar(OpenEBSNamespace, nfsProvisionerName)
			Expect(err).To(BeNil(), "while removing volume-event-exporter sidecar")

		})
	})

	When("Deleting NFS PV", func() {
		It("should delete nfs pv", func() {
			Expect(nfsPVName).NotTo(BeEmpty(), "NFS PV name should not be empty")

			err := Client.deletePV(nfsPVName)
			Expect(err).To(BeNil(), "while deleting NFS pv %s", nfsPVName)
		})
	})

	When("Add volume-events-collector sidecar", func() {
		It("should add volume-events-collector sidecar", func() {
			err := addEventControllerSideCar(OpenEBSNamespace, nfsProvisionerName)
			Expect(err).To(BeNil(), "while adding volume-event-exporter sidecar")
		})

		It("NFS PV should be removed from cluster", func() {
			Expect(nfsPVName).NotTo(BeEmpty(), "NFS PV name should not be empty")

			By("checking deletion of NFS PV")
			var isNFSPVDeleted bool
			for retry := 0; retry < maxRetryCount; retry++ {
				_, err := Client.getPV(nfsPVName)
				if err != nil && k8serrors.IsNotFound(err) {
					isNFSPVDeleted = true
					break
				}
				Expect(err).To(BeNil(), "while fetching NFS pv %s", nfsPVName)
				time.Sleep(time.Second * 5)
			}
			Expect(isNFSPVDeleted).To(BeTrue(), "NFS pv %s should be deleted", nfsPVName)
		})

		It("should send events to server", func() {
			Expect(backendPVCName).NotTo(BeEmpty(), "Backend PVC name should not be empty")
			Expect(backendPVName).NotTo(BeEmpty(), "Backend PV name should not be empty")
			Expect(nfsPVName).NotTo(BeEmpty(), "NFS PV name should not be empty")

			var backendPVCObj *corev1.PersistentVolumeClaim
			var err error
			var isEventReceived bool

			for retry := 0; retry < maxRetryCount; retry++ {
				backendPVCObj, err = Client.getPVC(OpenEBSNamespace, backendPVCName)
				Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

				fmt.Printf("pvc %s/%s\n", OpenEBSNamespace, backendPVCName)
				_, isEventReceived = backendPVCObj.Annotations[nfs.VolumeDeleteNFSPVKey]
				if isEventReceived {
					break
				}
				time.Sleep(time.Second * 15)
			}
			Expect(isEventReceived).To(BeTrue(), "NFS pv %s details are not exported to server for delete event", nfsPVName)
			Expect(backendPVCObj.Annotations[nfs.VolumeDeleteNFSPVKey]).To(Equal(nfsPVName), "while verifying NFS pv delete event data")
			Expect(backendPVCObj.Annotations[nfs.VolumeDeleteBackendPVCKey]).To(Equal(OpenEBSNamespace+"-"+backendPVCName), "while verifying backend pvc delete event data")
			Expect(backendPVCObj.Annotations[nfs.VolumeDeleteBackendPVKey]).To(Equal(backendPVName), "while verifying backend pv delete event data")
		})
	})

	When("Test event finalizers are removed from nfs resources", func() {
		It("should delete nfs resources", func() {
			Expect(backendPVCName).NotTo(BeEmpty(), "Backend PVC name should not be empty")

			// Remove test finalizer on Backend PVC
			backendPVC, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

			removeFinalizer(&backendPVC.ObjectMeta, integrationTestFinalizer)

			_, err = Client.updatePVC(backendPVC)
			Expect(err).To(BeNil(), "while removing test protection finalizer on backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

			err = Client.deleteDeployment(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while deleting deployment %s/%s", OpenEBSNamespace, backendPVCName)

			err = Client.deletePVC(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while deleting backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

			err = Client.deleteService(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while deleting service %s/%s", OpenEBSNamespace, backendPVCName)
		})

		It("should get deleted from cluster", func() {
			Expect(backendPVCName).NotTo(BeEmpty(), "Backend PVC name should not be empty")

			// Check backend PVC existence
			var isBackendPVCExist bool = true
			for retry := 0; retry < maxRetryCount; retry++ {
				_, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
				if err != nil && k8serrors.IsNotFound(err) {
					isBackendPVCExist = false
					break
				}
				Expect(err).To(BeNil(), "while checking for existence of backend pvc %s/%s", OpenEBSNamespace, backendPVCName)
				time.Sleep(time.Second * 10)
			}
			Expect(isBackendPVCExist).To(BeFalse(), "backend pvc %s/%s shouldn't exist in cluster", OpenEBSNamespace, backendPVCName)
		})
	})

	When("StorageClass is deleted", func() {
		It("should delete the SC", func() {
			By("deleting SC " + scName)
			err := Client.deleteStorageClass(scName)
			Expect(err).To(BeNil(), "while deleting sc %s", scName)
		})
	})
})
