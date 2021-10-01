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
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/mayadata-io/volume-events-exporter/tests/nfs"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TEST NFS PVC WITH INVALID BACKEND STORAGECLASS", func() {
	var (
		// SC configuration
		backendSCName      = "invalid-backend-sc"
		scNfsServerType    = "kernel"
		nfsProvisionerName = "openebs.io/nfsrwx"

		// PVC configuration
		accessModes    = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		capacity       = "1Gi"
		pvcName        = "nfs-pvc-invalid-backend-sc"
		scName         = "openebs-rwx-invalid-backend-sc"
		backendPVCName string

		maxRetryCount = 15
	)

	When("StorageClass with backend sc "+backendSCName+"is created", func() {
		It("should create a StorageClass", func() {
			casObj := []Config{
				{
					Name:  KeyPVNFSServerType,
					Value: scNfsServerType,
				},
				{
					Name:  KeyPVBackendStorageClass,
					Value: backendSCName,
				},
			}
			casObjStr, err := yaml.Marshal(casObj)
			Expect(err).To(BeNil(), "while marshaling CAS object")

			err = Client.createStorageClass(&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: scName,
					Annotations: map[string]string{
						"openebs.io/cas-type":   "nfsrwx",
						"cas.openebs.io/config": string(casObjStr),
					},
				},
				Provisioner: nfsProvisionerName,
			})
			Expect(err).To(BeNil(), "while creating SC {%s}", scName)
		})
	})

	When(fmt.Sprintf("pvc with storageclass %s is created", scName), func() {
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
		})

		It("should have NFS pvc "+pvcName+"in pending state", func() {
			pvcObj, err := Client.getPVC(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while fetching pvc %s/%s", applicationNamespace, pvcName)
			Expect(pvcObj.Status.Phase).To(Equal(corev1.ClaimPending), "while verifying NFS PVC claim phase")

			var isExpectedEventExist bool
			maxRetryCount := 15
			backendPVCName = "nfs-pvc-" + string(pvcObj.UID)
			for retries := 0; retries < maxRetryCount; retries++ {
				// Verify for provision failure events on PVC
				eventList, err := Client.getEvents(pvcObj)
				Expect(err).To(BeNil(), "while fetching PVC %s/%s", pvcObj.Namespace, pvcObj.Name)

				for _, event := range eventList.Items {
					if event.Reason == "ProvisioningFailed" &&
						strings.Contains(event.Message,
							fmt.Sprintf("timed out waiting for PVC{%s/%s} to bound", OpenEBSNamespace, backendPVCName)) {
						isExpectedEventExist = true
						break
					}
				}
				if isExpectedEventExist {
					break
				}
				// event will be generated after 60 seconds
				time.Sleep(time.Second * 10)
			}
			Expect(isExpectedEventExist).To(BeTrue(), "ProvisioningFailed event should exist with PVC bound timed out")

			// Add integration test finalizer on backend PVC, so that object
			// will remains same even after deletion unless removal of finalizer
			// NOTE: Below snippet will be removed after merging
			backendPVCObj, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while fetching pvc %s/%s", OpenEBSNamespace, backendPVCName)
			backendPVCObj.Finalizers = append(backendPVCObj.Finalizers, integrationTestFinalizer)
			_, err = Client.updatePVC(backendPVCObj)
			Expect(err).To(BeNil(), "while adding %s finlaizer to pvc %s/%s", integrationTestFinalizer, OpenEBSNamespace, backendPVCName)
		})

		It("should not export any events to REST service", func() {
			var backendPVCObj *corev1.PersistentVolumeClaim
			var err error
			var isEventReceived bool

			Expect(backendPVCName).NotTo(BeEmpty(), "backend pvc name shouldn't be empty")
			for retries := 0; retries < maxRetryCount; retries++ {
				backendPVCObj, err = Client.getPVC(OpenEBSNamespace, backendPVCName)
				Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

				_, isEventReceived = backendPVCObj.Annotations[nfs.VolumeCreateNFSPVCKey]
				if isEventReceived {
					break
				}
				// Reconciliation will happen at every 60 seconds but if any error occurs it will
				// get reconcile easily
				time.Sleep(time.Second * 10)
			}
			Expect(isEventReceived).To(BeFalse(), "REST service shouldn't receive any events")
			_, isNFSPVCAnnoExist := backendPVCObj.Annotations[nfs.VolumeCreateNFSPVCKey]
			_, isNFSPVAnnoExist := backendPVCObj.Annotations[nfs.VolumeCreateNFSPVKey]
			_, isBackendPVCExist := backendPVCObj.Annotations[nfs.VolumeCreateBackendPVCKey]
			_, isBackendPVExist := backendPVCObj.Annotations[nfs.VolumeCreateBackendPVKey]
			Expect(isNFSPVCAnnoExist).To(BeFalse(), "REST service shouldn't receive any create event but has NFS pvc name")
			Expect(isNFSPVAnnoExist).To(BeFalse(), "REST service shouldn't receive any create event but has NFS pv name")
			Expect(isBackendPVCExist).To(BeFalse(), "REST service shouldn't receive any create event but has backend pvc name")
			Expect(isBackendPVExist).To(BeFalse(), "REST service shouldn't receive any create event but has backend pv name")
		})
	})

	When("pvc "+pvcName+"is deleted", func() {
		It("should get delete", func() {
			err := Client.deletePVC(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while deleting pvc %s/%s", applicationNamespace, pvcName)
		})
		It("should get delete and shouldn't send any events to REST service", func() {
			var backendPVCObj *corev1.PersistentVolumeClaim
			var err error
			var isDeleteEventReceived bool

			Expect(backendPVCName).NotTo(BeEmpty(), "backend pvc name shouldn't be empty")
			for retries := 0; retries < maxRetryCount; retries++ {
				backendPVCObj, err = Client.getPVC(OpenEBSNamespace, backendPVCName)
				Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

				_, isDeleteEventReceived = backendPVCObj.Annotations[nfs.VolumeCreateNFSPVCKey]
				if isDeleteEventReceived {
					break
				}
				// Reconciliation will happen at every 60 seconds but if any error occurs it will
				// get reconcile easily
				time.Sleep(time.Second * 10)
			}
			Expect(isDeleteEventReceived).To(BeFalse(), "REST service shouldn't receive any events")
			_, isNFSPVAnnoExist := backendPVCObj.Annotations[nfs.VolumeDeleteNFSPVKey]
			_, isBackendPVCExist := backendPVCObj.Annotations[nfs.VolumeDeleteBackendPVCKey]
			_, isBackendPVExist := backendPVCObj.Annotations[nfs.VolumeDeleteBackendPVKey]
			Expect(isNFSPVAnnoExist).To(BeFalse(), "REST service shouldn't receive any delete event but has NFS pv name")
			Expect(isBackendPVCExist).To(BeFalse(), "REST service shouldn't receive any delete event but has backend pvc name")
			Expect(isBackendPVExist).To(BeFalse(), "REST service shouldn't receive any delete event but has backend pv name")
		})
	})

	When(integrationTestFinalizer+" finalizer is removed from backend pvc "+backendPVCName, func() {
		It("should delete backend pvc", func() {
			Expect(backendPVCName).NotTo(BeEmpty(), "backend pvc name shouldn't be empty")

			backendPVCObj, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

			var finalizers []string
			for index := range backendPVCObj.Finalizers {
				if backendPVCObj.Finalizers[index] != integrationTestFinalizer {
					finalizers = append(finalizers, backendPVCObj.Finalizers[index])
				}
			}
			backendPVCObj.Finalizers = finalizers

			_, err = Client.updatePVC(backendPVCObj)
			Expect(err).To(BeNil(), "while removing %s finlaizer to pvc %s/%s", integrationTestFinalizer, OpenEBSNamespace, backendPVCName)
		})
	})

	When("StorageClass "+scName+" is deleted", func() {
		It("should delete the SC", func() {
			By("deleting SC " + scName)
			err := Client.deleteStorageClass(scName)
			Expect(err).To(BeNil(), "while deleting sc %s", scName)
		})
	})
})
