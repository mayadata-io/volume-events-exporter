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

package controller

import (
	"testing"

	"github.com/mayadata-io/volume-events-exporter/pkg/nfspv"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldSendEvent(t *testing.T) {
	tests := map[string]struct {
		pvObj                   *corev1.PersistentVolume
		expectedShouldSendEvent bool
	}{
		"When volume doesn't required to send event information": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by": "openebs.io/nfsrwx",
					},
					CreationTimestamp: metav1.Now(),
				},
			},
			expectedShouldSendEvent: false,
		},
		"When volume requires to send event information": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-2",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by": "openebs.io/nfsrwx",
						"events.openebs.io/required":      "true",
					},
					CreationTimestamp: metav1.Now(),
				},
			},
			expectedShouldSendEvent: true,
		},
		"When create event already sent and not yet eligible to send delete event": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-3",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"events.openebs.io/required":         "true",
						"nfs.event.openebs.io/volume-create": "sent",
					},
					CreationTimestamp: metav1.Now(),
				},
			},
			expectedShouldSendEvent: false,
		},
		"When create information is required to send": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-4",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"events.openebs.io/required":         "true",
						"nfs.event.openebs.io/volume-create": "pending",
					},
					CreationTimestamp: metav1.Now(),
				},
			},
			expectedShouldSendEvent: true,
		},
		"When create event already sent and volume is eligible for sending delete event": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-5",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"events.openebs.io/required":         "true",
						"nfs.event.openebs.io/volume-create": "sent",
					},
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			expectedShouldSendEvent: true,
		},
		"When create and delete event both are sent": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-5",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"events.openebs.io/required":         "true",
						"nfs.event.openebs.io/volume-create": "sent",
						"nfs.event.openebs.io/volume-delete": "sent",
					},
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			// Finalizer removal needs to be done
			expectedShouldSendEvent: true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			gotOutput := shouldSendEvent(test.pvObj)
			if gotOutput != test.expectedShouldSendEvent {
				t.Errorf("%q test failed expected %t but got %t", name, test.expectedShouldSendEvent, gotOutput)
			}
		})
	}
}

func TestIsCreateVolumeEventSent(t *testing.T) {
	tests := map[string]struct {
		pvObj          *corev1.PersistentVolume
		expectedOutput bool
	}{
		"When create event is not yet sent": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by": "openebs.io/nfsrwx",
					},
				},
			},
			expectedOutput: false,
		},
		"When create event is already sent": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"nfs.event.openebs.io/volume-create": "sent",
					},
				},
			},
			expectedOutput: true,
		},
		"When create event is not sent but marked for deleted": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by": "openebs.io/nfsrwx",
					},
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			expectedOutput: false,
		},
		"When create event is already sent for localpv": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":      "openebs.io/nfsrwx",
						"local.event.openebs.io/volume-create": "sent",
					},
				},
			},
			expectedOutput: true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			gotOutput := isCreateVolumeEventSent(test.pvObj)
			if gotOutput != test.expectedOutput {
				t.Errorf("%q test failed expected %t but got %t", name, test.expectedOutput, gotOutput)
			}
		})
	}
}

func TestIsDeleteVolumeEventSent(t *testing.T) {
	tests := map[string]struct {
		pvObj          *corev1.PersistentVolume
		expectedOutput bool
	}{
		"When delete event is not yet sent": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"nfs.event.openebs.io/volume-create": "sent",
					},
				},
			},
			expectedOutput: false,
		},
		"When delete event is already sent": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"nfs.event.openebs.io/volume-create": "sent",
						"nfs.event.openebs.io/volume-delete": "sent",
					},
				},
			},
			expectedOutput: true,
		},
		"When delete event is not sent but marked for deleted": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by": "openebs.io/nfsrwx",
					},
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			expectedOutput: false,
		},
		"When delete event is already sent for localpv": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":      "openebs.io/nfsrwx",
						"local.event.openebs.io/volume-delete": "sent",
					},
				},
			},
			expectedOutput: true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			gotOutput := isDeleteVolumeEventSent(test.pvObj)
			if gotOutput != test.expectedOutput {
				t.Errorf("%q test failed expected %t but got %t", name, test.expectedOutput, gotOutput)
			}
		})
	}
}

func TestGetEventSender(t *testing.T) {
	tests := map[string]struct {
		pvObj         *corev1.PersistentVolume
		controller    *PVEventController
		isErrExpected bool
	}{
		"When nfspv requesting for sending volume information": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nfs-pv-1",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by": "openebs.io/nfsrwx",
					},
					Finalizers: []string{
						"kuberenetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
					Labels: map[string]string{
						nfspv.OpenEBSCASLabelKey: nfspv.OpenEBSNFSCASLabelValue,
					},
					CreationTimestamp: metav1.Now(),
				},
			},
			controller:    &PVEventController{},
			isErrExpected: false,
		},
		"When nfspv exist reconciles after removing finalizer": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nfs-pv-2",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by": "openebs.io/nfsrwx",
					},
					Finalizers: []string{
						"custom.io/volume-protection",
					},
					Labels: map[string]string{
						nfspv.OpenEBSCASLabelKey: nfspv.OpenEBSNFSCASLabelValue,
					},
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			controller:    &PVEventController{},
			isErrExpected: false,
		},
		"When nfspv has already sent information but finalizer exist": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nfs-pv-3",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"nfs.event.openebs.io/volume-create": "sent",
						"nfs.event.openebs.io/volume-delete": "sent",
					},
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
					Labels: map[string]string{
						nfspv.OpenEBSCASLabelKey: nfspv.OpenEBSNFSCASLabelValue,
					},
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			controller:    &PVEventController{},
			isErrExpected: false,
		},
		"When localpv is requesting for event information": {
			pvObj: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nfs-pv-4",
					Annotations: map[string]string{
						"pv.kubernetes.io/provisioned-by":    "openebs.io/nfsrwx",
						"nfs.event.openebs.io/volume-create": "sent",
						"nfs.event.openebs.io/volume-delete": "sent",
					},
					Finalizers: []string{
						"local.events.openebs.io/finalizer",
					},
					Labels: map[string]string{
						"openebs.io/cas-type": "local",
					},
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			controller:    &PVEventController{},
			isErrExpected: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			_, err := test.controller.getEventSender(test.pvObj)
			if test.isErrExpected && err == nil {
				t.Errorf("%q test failed expected error to occur but got nil", name)
			}
			if !test.isErrExpected && err != nil {
				t.Errorf("%q test failed expected error not to occur but got %v", name, err)
			}
		})
	}
}
