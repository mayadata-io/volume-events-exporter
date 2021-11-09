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

	"github.com/mayadata-io/volume-events-exporter/tests/nfs"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/*
 * This test verifies the behavior of volume-event-controller when
 * mismatch SIGNING_KEY is provided. This behavior will be verified
 * with following steps:
 * 1. Update the nfs-provisioner deployment with mismatched SIGNING_KEY
 * 2. Create a PVC and it should create NFS resources
 * 3. Verify event on NFS PV informing about SIGNING mismatch
 * 4. Update the nfs-provisioner deployment with correct SIGNING_KEY
 * 5. Verify whether events exported to REST server
 * 6. Delete PVC and verify events
 */

var _ = Describe("TEST NFS PVC CREATE & DELETE EVENTS WITH MISMATCH SIGNING KEY AND UPDATING LATER WITH VALID KEY", func() {
	var (
		// PVC configuration
		accessModes    = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		capacity       = "1Gi"
		pvcName        = "mismatch-signing-event-nfs-pvc"
		scName         = "openebs-rwx"
		nfsPVName      string
		backendPVCName string
		backendPVName  string

		maxRetryCount = 15
	)

	When("mismatched signing key information is added on deployment", func() {
		It("should come to running state", func() {
			err := addSigningKeyPath(OpenEBSNamespace, nfsProvisionerName, corev1.VolumeMount{Name: "signature", MountPath: "/secret/rsa"}, "/secret/rsa/id_rsa_fake_private")
			Expect(err).To(BeNil(), "while updating deployment with mismatched signatures")
		})
	})

	When("pvc with storageclass openebs-rwx is created", func() {
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
			Expect(err).To(BeNil(), "while creating pvc {%s} in namespace {%s}", pvcName, applicationNamespace)

			pvcPhase, err := Client.waitForPVCBound(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while waiting for pvc %s/%s bound phase", applicationNamespace, pvcName)
			Expect(pvcPhase).To(Equal(corev1.ClaimBound), "pvc %s/%s should be in bound phase", applicationNamespace, pvcName)
		})
	})

	When("pvc gets into bounded state", func() {
		It("should have NFS server in running state", func() {
			pvcObj, err := Client.getPVC(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while fetching pvc %s/%s", applicationNamespace, pvcName)
			nfsPVName = pvcObj.Spec.VolumeName
			// NOTE: Backend PVC name will be nfs-<nfs pv name>
			backendPVCName = "nfs-" + nfsPVName

			nfsServerLabelSelector := "openebs.io/nfs-server=nfs-" + pvcObj.Spec.VolumeName
			err = Client.waitForPods(OpenEBSNamespace, nfsServerLabelSelector, corev1.PodRunning, 1)
			Expect(err).To(BeNil(), "while verifying NFS Server running status")
		})

		It("should get validation error from REST server while exporting event details", func() {
			errMessage := "crypto/rsa: verification error"
			var isVerificationErrorExist bool

			Expect(nfsPVName).ShouldNot(BeEmpty(), "NFS PV name should not be empty")
			pvObj, err := Client.getPV(nfsPVName)
			Expect(err).To(BeNil(), "while fetching NFS PV %s", nfsPVName)

			for retries := 0; retries < maxRetryCount; retries++ {
				// Verify for verification error event on NFS PV
				eventList, err := Client.getEvents(pvObj)
				Expect(err).To(BeNil(), "while fetching PV %s", pvObj.Name)
				for _, event := range eventList.Items {
					if event.Reason == "EventInformation" &&
						strings.Contains(event.Message,
							fmt.Sprintf("failed to send create event data of volume %s to server", pvObj.Name)) &&
						strings.Contains(event.Message, errMessage) {
						isVerificationErrorExist = true
						break
					}
				}
			}
			Expect(isVerificationErrorExist).To(BeTrue(), "Exporting volume create event should fail due to signing key mismatch")
		})
	})

	When("nfs-provisioner deployment is updated with correct signing key", func() {
		It("should get updated accordingly", func() {
			err := addSigningKeyPath(OpenEBSNamespace, nfsProvisionerName, corev1.VolumeMount{Name: "signature", MountPath: "/secret/rsa"}, "/secret/rsa/id_rsa_private")
			Expect(err).To(BeNil(), "while updating deployment with correct signatures")
		})

		It("should have sent details to server... verify signature annotation of backing PVC", func() {
			var backingPVCObj *corev1.PersistentVolumeClaim
			var err error
			var isEventReceived bool

			for retries := 0; retries < maxRetryCount; retries++ {
				backingPVCObj, err = Client.getPVC(OpenEBSNamespace, backendPVCName)
				Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

				_, isEventReceived = backingPVCObj.Annotations[nfs.VolumeCreateNFSPVCKey]
				if isEventReceived {
					break
				}
				// Reconciliation will happen at every 60 seconds but if any error occurs it will
				// get reconcile easily
				time.Sleep(time.Second * 10)
			}
			Expect(isEventReceived).To(BeTrue(), "NFS pvc %s/%s details are not exported to server", applicationNamespace, pvcName)
			backendPVC, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)
			backendPVName = backendPVC.Spec.VolumeName

			Expect(backingPVCObj.Annotations[nfs.VolumeCreateNFSPVCKey]).To(Equal(applicationNamespace+"-"+pvcName), "while verifying nfs pvc create event data")
			Expect(backingPVCObj.Annotations[nfs.VolumeCreateNFSPVKey]).To(Equal(nfsPVName), "while verifying nfs pv create event data")
			Expect(backingPVCObj.Annotations[nfs.VolumeCreateBackendPVCKey]).To(Equal(OpenEBSNamespace+"-"+backendPVCName), "while verifying backend pvc create event data")
			Expect(backingPVCObj.Annotations[nfs.VolumeCreateBackendPVKey]).To(Equal(backendPVName), "while verifying backend pv create event data")
			Expect(backingPVCObj.Annotations[nfs.VolumeCreateNFSSignatureKey]).To(Equal("verified"), "while verifying signature of volume_provisioned signed data")
		})
	})

	When("pvc is deleted", func() {
		It("should delete pvc", func() {
			err := Client.deletePVC(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while deleting pvc %s/%s", applicationNamespace, pvcName)
		})

		It("should send events to server", func() {
			var backingPVCObj *corev1.PersistentVolumeClaim
			var err error
			var isEventReceived bool

			for retry := 0; retry < maxRetryCount; retry++ {
				backingPVCObj, err = Client.getPVC(OpenEBSNamespace, backendPVCName)
				Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

				_, isEventReceived = backingPVCObj.Annotations[nfs.VolumeDeleteNFSPVKey]
				if isEventReceived {
					break
				}
				// Reconciliation will happen at every 60 seconds but if any error occurs it will
				// get reconcile easily
				time.Sleep(time.Second * 10)
			}
			Expect(isEventReceived).To(BeTrue(), "NFS pv %s details are not exported to server for delete event", nfsPVName)
			Expect(backingPVCObj.Annotations[nfs.VolumeDeleteNFSPVKey]).To(Equal(nfsPVName), "while verifying NFS pv delete event data")
			Expect(backingPVCObj.Annotations[nfs.VolumeDeleteBackendPVCKey]).To(Equal(OpenEBSNamespace+"-"+backendPVCName), "while verifying backend pvc delete event data")
			Expect(backingPVCObj.Annotations[nfs.VolumeDeleteBackendPVKey]).To(Equal(backendPVName), "while verifying backend pv delete event data")
			Expect(backingPVCObj.Annotations[nfs.VolumeDeleteNFSSignatureKey]).To(Equal("verified"), "while verifying signature of volume_deleted signed data")
		})
	})

	When("test event finalizers are removed on resource", func() {
		It("should get removed", func() {
			// Remove test finalizer on Backend PVC
			backendPVC, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
			Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)
			removeFinalizer(&backendPVC.ObjectMeta, integrationTestFinalizer)
			_, err = Client.updatePVC(backendPVC)
			Expect(err).To(BeNil(), "while removing test protection finalizer on backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

		})

		It("should get deleted from cluster", func() {

			// Check NFS PV existence
			var isNFSPVExist bool = true
			for retry := 0; retry < maxRetryCount; retry++ {
				_, err := Client.getPV(nfsPVName)
				if err != nil && k8serrors.IsNotFound(err) {
					isNFSPVExist = false
					break
				}
				Expect(err).To(BeNil(), "while checking for existence of NFS pv %s", nfsPVName)
				time.Sleep(time.Second * 10)
			}
			Expect(isNFSPVExist).To(BeFalse(), "NFS pv %s shouldn't exist in cluster", nfsPVName)

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

			// Check backend PV existence
			var isBackendPVExist bool = true
			for retry := 0; retry < maxRetryCount; retry++ {
				_, err := Client.getPV(backendPVName)
				if err != nil && k8serrors.IsNotFound(err) {
					isBackendPVExist = false
					break
				}
				Expect(err).To(BeNil(), "while checking for existence of backend pv %s", backendPVName)
				time.Sleep(time.Second * 10)
			}
			Expect(isBackendPVExist).To(BeFalse(), "backend pv %s shouldn't exist in cluster", OpenEBSNamespace, backendPVName)
		})
	})

	When("signing key path information is removed from deployment", func() {
		It("should get removed from configuration", func() {
			err := removeSigningKeyPath(OpenEBSNamespace, nfsProvisionerName)
			Expect(err).To(BeNil(), "while removing the signatures of deployment")
		})
	})
})
