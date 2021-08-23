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
	"time"

	"github.com/mayadata-io/volume-events-exporter/tests/nfs"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TEST NFS PVC CREATE & DELTE EVENTS", func() {
	var (
		// PVC configuration
		accessModes    = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		capacity       = "1Gi"
		pvcName        = "sanity-event-nfs-pvc"
		scName         = "openebs-rwx"
		nfsPVName      string
		backendPVCName string
		backendPVName  string

		// backend pvc configuration
		integrationTestFinalizer = "it.nfs.openebs.io/test-protection"

		maxRetryCount = 15
	)

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

			err = markNFSResources(applicationNamespace, pvcName)
			Expect(err).To(BeNil(), "while makrking for events")
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

		It("should have sent details to server... verify annotation of backing PVC", func() {
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
})

// This function can be removed once configmap support is merged for
// configuring metadata on NFS provisioner owned resources
func markNFSResources(nfsPVCNamespace, nfsPVCName string) error {
	nfsEventFinalizer := "nfs.events.openebs.io/finalizer"
	integrationTestFinalizer := "it.nfs.openebs.io/test-protection"
	nfsPVC, err := Client.getPVC(nfsPVCNamespace, nfsPVCName)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch PVC %s/%s", nfsPVCNamespace, nfsPVCName)
	}

	nfsPV, err := Client.getPV(nfsPVC.Spec.VolumeName)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch NFS pv %s", nfsPVC.Spec.VolumeName)
	}

	backendPVCName := "nfs-" + nfsPVC.Spec.VolumeName
	backendPVC, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch backend pvc %s/%s", OpenEBSNamespace, backendPVCName)
	}
	backendPVC.Finalizers = append(backendPVC.Finalizers, nfsEventFinalizer, integrationTestFinalizer)
	_, err = Client.updatePVC(backendPVC)
	if err != nil {
		return errors.Wrapf(err, "while adding event finalzers on NFS pv %s", backendPVCName)
	}

	backendPV, err := Client.getPV(backendPVC.Spec.VolumeName)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch backend pv %s", backendPVC.Spec.VolumeName)
	}
	backendPV.Finalizers = append(backendPV.Finalizers, nfsEventFinalizer)
	_, err = Client.updatePV(backendPV)
	if err != nil {
		return errors.Wrapf(err, "while adding event finalizers on backend pv %s", backendPV.Name)
	}

	nfsPV.Finalizers = append(nfsPV.Finalizers, nfsEventFinalizer)
	if nfsPV.Annotations == nil {
		nfsPV.Annotations = map[string]string{}
	}
	nfsPV.Annotations["events.openebs.io/required"] = "true"
	_, err = Client.updatePV(nfsPV)
	if err != nil {
		return errors.Wrapf(err, "while adding event finalizers on NFS pvc %s/%s", nfsPV.Namespace, nfsPV.Name)
	}
	return nil
}

// nfsPVObj, err := Client.getPV(nfsPVCObj.Spec.VolumeName)
// Expect(err).To(BeNil(), "while fetching NFS pv %s", nfsPVCObj.Spec.VolumeName)
// Expect(isFinalizerExist(&nfsPVObj.ObjectMeta, nfs.ProvisionedFinalizerProtection)).To(BeTrue(), "NFS pv %s details are not exported to server for create event")

// nfsPVName = nfsPVObj.Name
// backendPVCName = "nfs-" + nfsPVName
// backendPVC, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
// Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)
// Expect(isFinalizerExist(&backendPVC.ObjectMeta, nfs.ProvisionedFinalizerProtection)).To(BeTrue(), "backend pvc %s/%s details are not exported to server for create event", OpenEBSNamespace, backendPVCName)

// backendPVName = backendPVC.Spec.VolumeName
// backendPVObj, err := Client.getPV(backendPVC.Spec.VolumeName)
// Expect(err).To(BeNil(), "while fetching backend pv %s", backendPVC.Spec.VolumeName)
// Expect(isFinalizerExist(&backendPVObj.ObjectMeta, nfs.ProvisionedFinalizerProtection)).To(BeTrue(), "backend pv %s details are not exported to server for create event", backendPVObj.Name)

// // Remove event finalizers on backend PVC
// backendPVC, err := Client.getPVC(OpenEBSNamespace, backendPVCName)
// Expect(err).To(BeNil(), "while fetching backend pvc %s/%s", OpenEBSNamespace, backendPVCName)
// removeFinalizer(&backendPVC.ObjectMeta, nfs.ProvisionedFinalizerProtection)
// removeFinalizer(&backendPVC.ObjectMeta, nfs.DeProvisionedFinalizerProtection)
// _, err = Client.updatePVC(backendPVC)
// Expect(err).To(BeNil(), "while removing event finalizers on backend pvc %s/%s", OpenEBSNamespace, backendPVCName)

// // Remove event finalizer on backend PV
// backendPV, err := Client.getPV(backendPVName)
// Expect(err).To(BeNil(), "while fetching NFS pv %s", backendPVName)
// removeFinalizer(&backendPV.ObjectMeta, nfs.ProvisionedFinalizerProtection)
// removeFinalizer(&backendPV.ObjectMeta, nfs.DeProvisionedFinalizerProtection)
// _, err = Client.updatePV(backendPV)
// Expect(err).To(BeNil(), "while removing event finalzers on backend pv %s", backendPVName)
