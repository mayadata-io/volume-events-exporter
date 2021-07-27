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

package nfspv

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mayadata-io/volume-events-exporter/pkg/collectorinterface"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

const (
	OpenEBSNFSLabelKey = "nfs.openebs.io/volume"
)

// nfsVolume will implement necessary methods
// required to satisfy collector interface
type nfsVolume struct {
	clientset kubernetes.Interface

	pvcLister corev1listers.PersistentVolumeClaimLister

	pvLister           corev1listers.PersistentVolumeLister
	pvObj              *corev1.PersistentVolume
	nfsServerNamespace string
	annotationPrefix   string
}

func NewNFSVolume(
	clientset kubernetes.Interface,
	pvcLister corev1listers.PersistentVolumeClaimLister,
	pvLister corev1listers.PersistentVolumeLister,
	pvObj *corev1.PersistentVolume,
	nfsServerNamespace string) collectorinterface.VolumeEventCollector {
	return &nfsVolume{
		clientset:          clientset,
		pvcLister:          pvcLister,
		pvLister:           pvLister,
		pvObj:              pvObj,
		nfsServerNamespace: nfsServerNamespace,
		annotationPrefix:   "nfs.",
	}
}

func (n *nfsVolume) CollectCreateEventData() (string, error) {

	volumeData, err := n.getVolumeData()
	if err != nil {
		return "", err
	}

	if volumeData.NFSPVC != nil {
		volumeData.NFSPVC.DeletionTimestamp = nil
		volumeData.NFSPVC.DeletionGracePeriodSeconds = nil
	}

	volumeData.NFSPV.DeletionTimestamp = nil
	volumeData.NFSPV.DeletionGracePeriodSeconds = nil

	volumeData.BackingPVC.DeletionTimestamp = nil
	volumeData.BackingPVC.DeletionGracePeriodSeconds = nil

	volumeData.BackingPV.DeletionTimestamp = nil
	volumeData.BackingPV.DeletionGracePeriodSeconds = nil

	createData := &NFSCreateVolumeData{
		VolumeProvisioned: volumeData,
	}
	rawData, err := json.Marshal(createData)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal create volume data")
	}
	return string(rawData), nil
}

func (n *nfsVolume) CollectDeleteEventData() (string, error) {
	// TODO: Is below check required?
	if n.pvObj.DeletionTimestamp == nil {
		return "", errors.Errorf("volume is not yet marked for deletion")
	}

	volumeData, err := n.getVolumeData()
	if err != nil {
		return "", err
	}

	createData := &NFSDeleteVolumeData{
		VolumeDeleted: volumeData,
	}
	rawData, err := json.Marshal(createData)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal create volume data")
	}
	return string(rawData), nil
}

func (n *nfsVolume) AnnotateCreateEvent(pvObj *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	if pvObj.Annotations == nil {
		pvObj.Annotations = make(map[string]string)
	}
	annoKey := n.annotationPrefix + collectorinterface.OpenebsCreateAnnotationSuffix
	pvObj.Annotations[annoKey] = collectorinterface.OpenebsSentAnnotationValue
	return n.clientset.CoreV1().PersistentVolumes().Update(context.TODO(), pvObj, metav1.UpdateOptions{})
}

func (n *nfsVolume) AnnotateDeleteEvent(pvObj *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	pvCopy := pvObj.DeepCopy()
	if pvCopy.Annotations == nil {
		pvCopy.Annotations = make(map[string]string)
	}
	annoKey := n.annotationPrefix + collectorinterface.OpenebsDeleteAnnotationSuffix
	pvCopy.Annotations[annoKey] = collectorinterface.OpenebsSentAnnotationValue

	patchBytes, _, err := getPatchData(pvObj, pvCopy)
	if err != nil {
		return nil, err
	}
	newPVObj, err := n.clientset.CoreV1().
		PersistentVolumes().
		Patch(context.TODO(), pvCopy.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, err
	}
	// Update inmemory reference
	n.pvObj = newPVObj
	return newPVObj, nil
}

func (n *nfsVolume) RemoveEventFinalizer() error {
	openebsEventFinalizer := n.annotationPrefix + collectorinterface.OpenebsEventFinalizerSuffix

	backendPVC, err := n.getPVCCopy(n.nfsServerNamespace, "nfs-"+n.pvObj.Name)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	// Get the backend PVC and follow the below steps to make
	// finalizer removal more consistent across restart of process
	// Step1: Get backend PV and remove finalizer PV(NOTE: PV will never get deleted till removal of finalizer from PVC)
	// Step2: Remove finalizer on backend PVC
	// Step3: Remove finalizer on NFS PVC(at end)
	if backendPVC != nil {
		backendPV, err := n.getPVCopy(backendPVC.Spec.VolumeName)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}

		if backendPV != nil {
			err = n.removeFinalizerOnPV(backendPV, openebsEventFinalizer)
			if err != nil {
				return err
			}
		}
		err = n.removeFinalizerOnPVC(backendPVC, openebsEventFinalizer)
		if err != nil {
			return err
		}
	}
	err = n.removeFinalizerOnPV(n.pvObj, openebsEventFinalizer)
	return err
}

func (n *nfsVolume) removeFinalizerOnPVC(pvcObj *corev1.PersistentVolumeClaim, finalizer string) error {
	isFinalizerRemoved := removeFinalizer(&pvcObj.ObjectMeta, finalizer)
	if !isFinalizerRemoved {
		// If finalizer is not deleted means finalizer doesn't exist so no need take action
		return nil
	}
	_, err := n.clientset.CoreV1().
		PersistentVolumeClaims(pvcObj.Namespace).
		Update(context.TODO(), pvcObj, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to delete %s finalizer on PVC %s/%s", finalizer, pvcObj.Namespace, pvcObj.Name)
	}
	return nil
}

func (n *nfsVolume) removeFinalizerOnPV(pvObj *corev1.PersistentVolume, finalizer string) error {
	isFinalizerRemoved := removeFinalizer(&pvObj.ObjectMeta, finalizer)
	if !isFinalizerRemoved {
		// If finalizer is not deleted means finalizer doesn't exist so no need take action
		return nil
	}
	_, err := n.clientset.CoreV1().
		PersistentVolumes().
		Update(context.TODO(), pvObj, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to delete %s finalizer on PV %s", finalizer, pvObj.Name)
	}
	return nil
}

func removeFinalizer(objectMeta *metav1.ObjectMeta, finalizer string) bool {
	originalFinalizers := objectMeta.GetFinalizers()

	var finalizerCopy []string
	var isFinalizerExist bool
	for _, curFinalizer := range originalFinalizers {
		if curFinalizer == finalizer {
			isFinalizerExist = true
			continue
		}
		finalizerCopy = append(finalizerCopy, curFinalizer)
	}
	objectMeta.Finalizers = finalizerCopy
	return isFinalizerExist
}

func (n *nfsVolume) getVolumeData() (*NFSVolumeData, error) {
	nfsPVC, err := n.getPVCCopy(n.pvObj.Spec.ClaimRef.Namespace, n.pvObj.Spec.ClaimRef.Name)
	if err != nil && !k8serrors.IsNotFound(err) {
		// NotFound is a case where controller is down meantime user deleted NFS PVC
		return nil, errors.Wrapf(err, "failed to get PVC %s/%s", n.pvObj.Spec.ClaimRef.Namespace, n.pvObj.Spec.ClaimRef.Name)
	}

	nfsPV, err := n.getPVCopy(n.pvObj.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get PV %s", n.pvObj.Name)
	}

	// NOTE: We are naming backend PVC with "nfs-"+nfs-pv name
	backendPVCName := "nfs-" + nfsPV.Name
	backendPVC, err := n.getPVCCopy(n.nfsServerNamespace, backendPVCName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get PVC {%s/%s}", n.nfsServerNamespace, backendPVCName)
	}

	backendPV, err := n.getPVCopy(backendPVC.Spec.VolumeName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get PV %s", backendPVC.Spec.VolumeName)
	}

	return &NFSVolumeData{
		NFSPVC:     nfsPVC,
		NFSPV:      nfsPV,
		BackingPVC: backendPVC,
		BackingPV:  backendPV,
	}, nil
}

func (n *nfsVolume) getPVCopy(pvName string) (*corev1.PersistentVolume, error) {
	if n.pvObj.Name == pvName {
		return n.pvObj.DeepCopy(), nil
	}

	pvObj, err := n.pvLister.Get(pvName)
	if err != nil {
		return nil, err
	}
	// Since we are fetching from cahce it is required
	// to make deepcopy so that callers can mutuate object
	return pvObj.DeepCopy(), nil
}

func (n *nfsVolume) getPVCCopy(namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	pvcObj, err := n.pvcLister.PersistentVolumeClaims(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	// Since we are fetching from cahce it is required
	// to make deepcopy so that callers can mutuate object
	return pvcObj.DeepCopy(), nil
}

func getPatchData(oldObj, newObj interface{}) ([]byte, []byte, error) {
	oldData, err := json.Marshal(oldObj)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal old object failed: %v", err)
	}
	newData, err := json.Marshal(newObj)
	if err != nil {
		return nil, nil, fmt.Errorf("mashal new object failed: %v", err)
	}
	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, oldObj)
	if err != nil {
		return nil, nil, fmt.Errorf("CreateTwoWayMergePatch failed: %v", err)
	}
	return patchBytes, oldData, nil
}
