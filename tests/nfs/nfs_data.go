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

package nfs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mayadata-io/volume-events-exporter/pkg/nfspv"
	"github.com/mayadata-io/volume-events-exporter/pkg/sign"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	VolumeCreateNFSPVCKey       = "it.nfs.openebs.io/vc-nfspvc"
	VolumeCreateNFSPVKey        = "it.nfs.openebs.io/vc-nfspv"
	VolumeCreateBackendPVCKey   = "it.nfs.openebs.io/vc-backend-pvc"
	VolumeCreateBackendPVKey    = "it.nfs.openebs.io/vc-backend-pv"
	VolumeCreateNFSSignatureKey = "it.nfs.openebs.io/vc-signature"

	VolumeDeleteNFSPVKey        = "it.nfs.openebs.io/vd-nfspv"
	VolumeDeleteBackendPVCKey   = "it.nfs.openebs.io/vd-backend-pvc"
	VolumeDeleteBackendPVKey    = "it.nfs.openebs.io/vd-backend-pv"
	VolumeDeleteNFSSignatureKey = "it.nfs.openebs.io/vd-signature"
)

type NFSCreateDeleteVolumeData struct {
	VolumeProvisioned *nfspv.NFSVolumeData `json:"volume_provisioned"`
	VolumeDeleted     *nfspv.NFSVolumeData `json:"volume_deleted"`
	Signature         string               `json:"signature"`
}

type NFS struct {
	Clientset kubernetes.Interface
	Unsigner  sign.Unsigner
}

func (n *NFS) ProcessData(req *http.Request) error {
	nfsData := &NFSCreateDeleteVolumeData{}
	err := decodeBody(req, nfsData)
	if err != nil {
		return errors.Wrapf(err, "decode of data failed")
	}

	if nfsData.VolumeProvisioned != nil {
		err = n.processProvisionData(nfsData.VolumeProvisioned, nfsData.Signature)
		if err != nil {
			return err
		}
	}
	if nfsData.VolumeDeleted != nil {
		err = n.processDeProvisionData(nfsData.VolumeDeleted, nfsData.Signature)
		if err != nil {
			return err
		}
	}

	return nil
}

// processProvisionData will process the data received over network and
// add following annotation to backend pvc which is later used to verify in Integration test
// - it.nfs.openebs.io/vc-nfspvc: <nfspvc-ns>-<nfspvc-name>
// - it.nfs.openebs.io/vc-nfspv: <nfs-pv-name>
// - it.nfs.openebs.io/vc-backend-pvc: <backend-pvc>-<backend-pvc-name>
// - it.nfs.openebs.io/vc-backend-pv: <backend-pv-name>
// - it.nfs.openebs.io/vc-signature: "verified" (this annotation will be set only when RSA signature is successfully verified)
// NOTE:
//	- Kubernetes doesn't allow to add new finalizers when object is marked for deletion
//	  so adding all the details on backend pvc annotation.
//	- Backend PVC will also add integration test finalizer {it.openebs.io/test-verification}
//	  during provisioning time which will get removed after after verifying from Integration test
func (n *NFS) processProvisionData(nfsVolumeData *nfspv.NFSVolumeData, signature string) error {
	isNFSPVExist := nfsVolumeData.NFSPV != nil
	isBackendPVCExist := nfsVolumeData.BackingPVC != nil
	isBackendPVExist := nfsVolumeData.BackingPV != nil
	// All(nfspv, backend pvc, backend pv) data should exist
	if !isNFSPVExist || !isBackendPVCExist || !isBackendPVExist {
		return errors.Errorf("expected to have NFS PV(%t), Backend PVC(%t) and Backend PV(%t) to exist", isNFSPVExist, isBackendPVCExist, isBackendPVExist)
	}
	testAnnotations := map[string]string{}

	if nfsVolumeData.NFSPVC != nil {
		testAnnotations[VolumeCreateNFSPVCKey] = nfsVolumeData.NFSPVC.Namespace + "-" + nfsVolumeData.NFSPVC.Name
	}

	if nfsVolumeData.NFSPV.CreationTimestamp.IsZero() {
		return errors.Errorf("expected to have creation timestamp on NFS PV %s", nfsVolumeData.NFSPV.Name)
	}
	if !nfsVolumeData.NFSPV.DeletionTimestamp.IsZero() {
		return errors.Errorf("expected no to have deletion timestamp on NFS PV %s", nfsVolumeData.NFSPV.Name)
	}
	if !nfsVolumeData.BackingPVC.DeletionTimestamp.IsZero() {
		return errors.Errorf("expected no to have deletion timestamp on backing PVC %s/%s", nfsVolumeData.BackingPVC.Namespace, nfsVolumeData.BackingPVC.Name)
	}
	if !nfsVolumeData.BackingPV.DeletionTimestamp.IsZero() {
		return errors.Errorf("expected no to have deletion timestamp on backing PV %s", nfsVolumeData.BackingPV.Name)
	}
	isSignatureVerified, err := n.verifySignature(nfsVolumeData, signature)
	if err != nil {
		return errors.Wrapf(err, "failed to verify signature")
	}

	testAnnotations[VolumeCreateNFSPVKey] = nfsVolumeData.NFSPV.Name
	testAnnotations[VolumeCreateBackendPVCKey] = nfsVolumeData.BackingPVC.Namespace + "-" + nfsVolumeData.BackingPVC.Name
	testAnnotations[VolumeCreateBackendPVKey] = nfsVolumeData.BackingPV.Name
	if isSignatureVerified {
		testAnnotations[VolumeCreateNFSSignatureKey] = "verified"
	}

	if nfsVolumeData.BackingPVC.Annotations == nil {
		nfsVolumeData.BackingPVC.Annotations = map[string]string{}
	}
	for key, value := range testAnnotations {
		nfsVolumeData.BackingPVC.Annotations[key] = value
	}

	_, err = n.Clientset.CoreV1().
		PersistentVolumeClaims(nfsVolumeData.BackingPVC.Namespace).
		Update(context.TODO(), nfsVolumeData.BackingPVC, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	klog.Infof("Addedd annotations %v on backend pvc %s/%s for create event", testAnnotations, nfsVolumeData.BackingPVC.Namespace, nfsVolumeData.BackingPVC.Name)
	return nil
}

// processDeProvisionData will process the data received over network and
// add following annotation to backend pvc which is later used to verify in Integration test
// - it.nfs.openebs.io/vd-nfspvc: <nfspvc-ns>-<nfspvc-name>
// - it.nfs.openebs.io/vd-nfspv: <nfs-pv-name>
// - it.nfs.openebs.io/vd-backend-pvc: <backend-pvc>-<backend-pvc-name>
// - it.nfs.openebs.io/vd-backend-pv: <backend-pv-name>
// - it.nfs.openebs.io/vd-signature: "true" (this annotation will be set only when RSA signature is successfully verified)
// NOTE:
//	- Kubernetes doesn't allow to add new finalizers when object is marked for deletion
//	  so adding all the details on backend pvc annotation.
//	- Backend PVC will also add integration test finalizer {it.openebs.io/test-verification}
//	  during provisioning time which will get removed after after verifying from Integration test
func (n *NFS) processDeProvisionData(nfsVolumeData *nfspv.NFSVolumeData, signature string) error {
	isNFSPVExist := nfsVolumeData.NFSPV != nil
	isBackendPVCExist := nfsVolumeData.BackingPVC != nil
	isBackendPVExist := nfsVolumeData.BackingPV != nil
	// All(nfspv, backend pvc, backend pv) data should exist
	if !isNFSPVExist || !isBackendPVCExist || !isBackendPVExist {
		return errors.Errorf("expected to have NFS PV(%t), Backend PVC(%t) and Backend PV(%t) to exist", isNFSPVExist, isBackendPVCExist, isBackendPVExist)
	}

	if nfsVolumeData.NFSPV.CreationTimestamp.IsZero() {
		return errors.Errorf("expected to have creation timestamp on NFS PV %s", nfsVolumeData.NFSPV.Name)
	}

	if nfsVolumeData.NFSPV.DeletionTimestamp.IsZero() {
		return errors.Errorf("expected to have deletion timestamp on NFS PV %s", nfsVolumeData.NFSPV.Name)
	}
	isSignatureVerified, err := n.verifySignature(nfsVolumeData, signature)
	if err != nil {
		return errors.Wrapf(err, "failed to verify signature")
	}

	testAnnotations := map[string]string{
		VolumeDeleteNFSPVKey:      nfsVolumeData.NFSPV.Name,
		VolumeDeleteBackendPVCKey: nfsVolumeData.BackingPVC.Namespace + "-" + nfsVolumeData.BackingPVC.Name,
		VolumeDeleteBackendPVKey:  nfsVolumeData.BackingPV.Name,
	}
	if isSignatureVerified {
		testAnnotations[VolumeDeleteNFSSignatureKey] = "verified"
	}

	if nfsVolumeData.BackingPVC.Annotations == nil {
		nfsVolumeData.BackingPVC.Annotations = map[string]string{}
	}
	for key, value := range testAnnotations {
		nfsVolumeData.BackingPVC.Annotations[key] = value
	}

	_, err = n.Clientset.CoreV1().
		PersistentVolumeClaims(nfsVolumeData.BackingPVC.Namespace).
		Update(context.TODO(), nfsVolumeData.BackingPVC, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	klog.Infof("Addedd annotations %v on backend pvc %s/%s for delete event", testAnnotations, nfsVolumeData.BackingPVC.Namespace, nfsVolumeData.BackingPVC.Name)

	return nil
}

func (n *NFS) verifySignature(obj interface{}, signature string) (bool, error) {
	// If signature is not provided then no need to verify
	if signature == "" {
		return false, nil
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return false, err
	}
	sigInBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false, err
	}
	klog.Infof("[Debug] going to verify signature")
	err = n.Unsigner.Unsign(data, sigInBytes)
	if err != nil {
		return false, err
	}
	klog.Infof("[Debug] Verified the signature")
	return true, nil
}

// TODO: Move below function to some common package

// getContentType will return data type by looking at Header
func getContentType(req *http.Request) (string, error) {

	if req.Header == nil {
		return "", fmt.Errorf("Request does not have any header")
	}

	return req.Header.Get("Content-Type"), nil
}

// // decodeNFSData will decode the body(JSON) obtained over network
// // - First it will try to unmarshal data using create volume and if volume_provisioned tag
// //	 doesn't exist then it will go to next step
// // - It will try to unmarshal data using delete volume and if volume_deleted tag exist
// //   then it will return data
// func decodeNFSData(req *http.Request, nfsCreateDeleteData *NFSCreateDeleteVolumeData) error {
// 	var nfsCreateData nfspv.NFSCreateVolumeData
// 	// nfsCreateData will be populated
// 	err := decodeBody(req, &nfsCreateData)
// 	if err != nil && err != io.EOF {
// 		return err
// 	}
// 	if nfsCreateData.VolumeProvisioned != nil {
// 		nfsCreateDeleteData.createVolume = &nfsCreateData
// 		return nil
// 	}
//
// 	var nfsDeleteData nfspv.NFSDeleteVolumeData
// 	err = decodeBody(req, &nfsDeleteData)
// 	if err != nil {
// 		return err
// 	}
// 	if nfsDeleteData.VolumeDeleted != nil {
// 		nfsCreateDeleteData.deleteVolume = &nfsDeleteData
// 		return nil
// 	}
// 	return errors.Errorf("either volume_provisioned or volume_deleted tag should exist to process the data")
// }

// Decode the request body to appropriate structure based on content type
func decodeBody(req *http.Request, out interface{}) error {

	cType, err := getContentType(req)
	if err != nil {
		return err
	}

	if strings.Contains(cType, "yaml") {
		return errors.Errorf("expecting JSON based content type")
	}

	// default is assumed to be json content
	return decodeJsonBody(req, out)
}

// decodeJsonBody is used to decode a JSON request body
func decodeJsonBody(req *http.Request, out interface{}) error {
	dec := json.NewDecoder(req.Body)
	return dec.Decode(&out)
}
