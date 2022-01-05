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
	corev1 "k8s.io/api/core/v1"
)

// NFSCreateVolumeData holds create volume information to send to server
type NFSCreateVolumeData struct {
	VolumeProvisioned *NFSVolumeData `json:"volume_provisioned"`
	Signature         string         `json:"signature"`
}

// NFSDeleteVolumeData holds delete volume information to send to server
type NFSDeleteVolumeData struct {
	VolumeDeleted *NFSVolumeData `json:"volume_deleted"`
	Signature     string         `json:"signature"`
}

// NFSVolumeData holds the information about NFS & corresponding backend volumes
type NFSVolumeData struct {
	NFSPVC     *corev1.PersistentVolumeClaim `json:"nfs_pvc"`
	NFSPV      *corev1.PersistentVolume      `json:"nfs_pv"`
	BackingPVC *corev1.PersistentVolumeClaim `json:"backing_pvc"`
	BackingPV  *corev1.PersistentVolume      `json:"backing_pv"`
}
