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
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mayadata-io/volume-events-exporter/pkg/collectorinterface"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	corev1informer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type fixture struct {
	clientset   kubernetes.Interface
	pvcInformer corev1informer.PersistentVolumeClaimInformer
	pvInformer  corev1informer.PersistentVolumeInformer
}

func newFixture() *fixture {
	kubeClient := fake.NewSimpleClientset()
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, 5)
	return &fixture{
		clientset:   kubeClient,
		pvcInformer: kubeInformerFactory.Core().V1().PersistentVolumeClaims(),
		pvInformer:  kubeInformerFactory.Core().V1().PersistentVolumes(),
	}
}

func (f *fixture) createFakePVC(pvcObj *corev1.PersistentVolumeClaim) error {
	// Create fake PVC
	_, err := f.clientset.CoreV1().PersistentVolumeClaims(pvcObj.Namespace).Create(context.TODO(), pvcObj, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	// Add fake PVC into cache
	err = f.pvcInformer.Informer().GetIndexer().Add(pvcObj)
	return err
}

func (f *fixture) createFakePV(pvObj *corev1.PersistentVolume) error {
	// Create fake PV
	_, err := f.clientset.CoreV1().PersistentVolumes().Create(context.TODO(), pvObj, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	// Add fake PV into cache
	err = f.pvInformer.Informer().GetIndexer().Add(pvObj)
	return err
}

func (f *fixture) preCreateResources(
	nfsPVC, backendPVC *corev1.PersistentVolumeClaim,
	nfsPV, backendPV *corev1.PersistentVolume) error {
	if nfsPVC != nil {
		err := f.createFakePVC(nfsPVC)
		if err != nil {
			return err
		}
	}
	if backendPVC != nil {
		err := f.createFakePVC(backendPVC)
		if err != nil {
			return err
		}

	}
	if nfsPV != nil {
		err := f.createFakePV(nfsPV)
		if err != nil {
			return err
		}
	}
	if backendPV != nil {
		err := f.createFakePV(backendPV)
		if err != nil {
			return err
		}

	}
	return nil
}

func TestCollectCreateEvents(t *testing.T) {
	f := newFixture()
	tests := map[string]struct {
		nfsPVC        *corev1.PersistentVolumeClaim
		nfsPV         *corev1.PersistentVolume
		backendPVC    *corev1.PersistentVolumeClaim
		backendPV     *corev1.PersistentVolume
		dataType      collectorinterface.DataType
		isErrExpected bool
	}{
		"when all nfs volume resources exist in the system": {
			nfsPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pvc1",
					Namespace:         "ns1",
					CreationTimestamp: metav1.Now(),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "pv1",
				},
			},
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv1",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc1",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv1",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv1",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv1",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
			dataType: collectorinterface.JSONDataType,
		},
		"when nfs pvc doesn't exist": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv2",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc2",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv2",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv2",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv2",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
			dataType: collectorinterface.JSONDataType,
		},
		"when backend PV doesn't exist": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv3",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc3",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv3",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv3",
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimLost,
				},
			},
			isErrExpected: true,
			dataType:      collectorinterface.JSONDataType,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			err := f.preCreateResources(test.nfsPVC, test.backendPVC, test.nfsPV, test.backendPV)
			if err != nil {
				t.Fatalf("%q test failed expected error not to occur during pre-resource creation but got error %v", name, err)
			}
			nfsVolume := &nfsVolume{
				clientset:          f.clientset,
				pvcLister:          f.pvcInformer.Lister(),
				pvLister:           f.pvInformer.Lister(),
				pvObj:              test.nfsPV,
				nfsServerNamespace: "openebs",
				annotationPrefix:   "nfs.",
				dataType:           test.dataType,
			}
			str, err := nfsVolume.CollectCreateEvents()
			if test.isErrExpected && err == nil {
				t.Fatalf("%q test failed expected error to occur but got nil", name)
			}
			if !test.isErrExpected && err != nil {
				t.Fatalf("%q test failed expected error not to occur but got %v", name, err)
			}
			if !test.isErrExpected {
				data := &NFSCreateVolumeData{}
				err := json.Unmarshal([]byte(str), data)
				if err != nil {
					t.Fatalf("%q test failed expected error not to occur during unmarshal of data error: %v", name, err)
				}
				defaultTime := metav1.Time{}
				// Check creation timestamp on NFSPV
				if data.VolumeProvisioned.NFSPV.CreationTimestamp == defaultTime {
					t.Fatalf("%q test failed expected nfs PV should have creation timestamp", name)
				}
				//Backend PV should exist
				if data.VolumeProvisioned.BackingPV == nil {
					t.Fatalf("%q test failed expected backend PV should exist", name)
				}
				//Backend PVC should exist
				if data.VolumeProvisioned.BackingPVC == nil {
					t.Fatalf("%q test failed expected backend PV should exist", name)
				}
			}
		})
	}
}

func TestCollectDeleteEvents(t *testing.T) {
	f := newFixture()
	tests := map[string]struct {
		nfsPVC        *corev1.PersistentVolumeClaim
		nfsPV         *corev1.PersistentVolume
		backendPVC    *corev1.PersistentVolumeClaim
		backendPV     *corev1.PersistentVolume
		dataType      collectorinterface.DataType
		isErrExpected bool
	}{
		"when all nfs volume resources exist in the system with deletion timestamp": {
			nfsPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pvc1",
					Namespace:         "ns1",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "pv1",
				},
			},
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv1",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc1",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv1",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv1",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv1",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
			dataType: collectorinterface.JSONDataType,
		},
		"when nfs pvc doesn't exist in cluster": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv2",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc2",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv2",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv2",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv2",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
			dataType: collectorinterface.JSONDataType,
		},
		"when backend PV doesn't exist": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv3",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc3",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv3",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv3",
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimLost,
				},
			},
			isErrExpected: true,
			dataType:      collectorinterface.JSONDataType,
		},
		"when nfs pv is not marked for deletion": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv4",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc4",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv4",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv4",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv4",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
			isErrExpected: true,
			dataType:      collectorinterface.JSONDataType,
		},
		"when all nfs volume resources exist in the system but datatype is not supported": {
			nfsPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pvc5",
					Namespace:         "ns1",
					CreationTimestamp: metav1.Now(),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "pv5",
				},
			},
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv5",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc5",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv5",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv5",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv5",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
			dataType:      collectorinterface.YAMLDataType,
			isErrExpected: true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			err := f.preCreateResources(test.nfsPVC, test.backendPVC, test.nfsPV, test.backendPV)
			if err != nil {
				t.Fatalf("%q test failed expected error not to occur during pre-resource creation but got error %v", name, err)
			}
			nfsVolume := &nfsVolume{
				clientset:          f.clientset,
				pvcLister:          f.pvcInformer.Lister(),
				pvLister:           f.pvInformer.Lister(),
				pvObj:              test.nfsPV,
				nfsServerNamespace: "openebs",
				annotationPrefix:   "nfs.",
				dataType:           test.dataType,
			}
			str, err := nfsVolume.CollectDeleteEvents()
			if test.isErrExpected && err == nil {
				t.Fatalf("%q test failed expected error to occur but got nil", name)
			}
			if !test.isErrExpected && err != nil {
				t.Fatalf("%q test failed expected error not to occur but got %v", name, err)
			}
			if !test.isErrExpected {
				data := &NFSDeleteVolumeData{}
				err := json.Unmarshal([]byte(str), data)
				if err != nil {
					t.Fatalf("%q test failed expected error not to occur during unmarshal of data error: %v", name, err)
				}
				defaultTime := metav1.Time{}
				// Check creation timestamp on NFSPV
				if data.VolumeDeleted.NFSPV.CreationTimestamp == defaultTime {
					t.Fatalf("%q test failed expected nfs PV should have creation timestamp", name)
				}
				if data.VolumeDeleted.NFSPV.DeletionTimestamp == nil {
					t.Fatalf("%q test failed expected nfs PV should have deletion timestamp", name)
				}
				//Backend PV should exist
				if data.VolumeDeleted.BackingPV == nil {
					t.Fatalf("%q test failed expected backend PV should exist", name)
				}
				//Backend PVC should exist
				if data.VolumeDeleted.BackingPVC == nil {
					t.Fatalf("%q test failed expected backend PV should exist", name)
				}
			}
		})
	}
}

func TestAnnotateCreateEvent(t *testing.T) {
	f := newFixture()
	tests := map[string]struct {
		nfsPV         *corev1.PersistentVolume
		isErrExpected bool
	}{
		"when PV is requested to annotate with create event": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv1",
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
					CreationTimestamp: metav1.Now(),
					Annotations: map[string]string{
						"pv-created-by/provisioner": "nfs.openebs.io",
					},
				},
			},
		},
		"when PV is already annotated with create event": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv2",
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
					CreationTimestamp: metav1.Now(),
					Annotations: map[string]string{
						"pv-created-by/provisioner":           "nfs.openebs.io",
						"nfs.events.openebs.io/volume-create": "sent",
					},
				},
			},
		},
		"when PV create event is pending": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv3",
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
					CreationTimestamp: metav1.Now(),
					Annotations: map[string]string{
						"pv-created-by/provisioner":           "nfs.openebs.io",
						"nfs.events.openebs.io/volume-create": "pending",
					},
				},
			},
		},
		"when PV doesn't have any annotations": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv4",
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
					CreationTimestamp: metav1.Now(),
				},
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			_, err := f.clientset.CoreV1().PersistentVolumes().Create(context.TODO(), test.nfsPV, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("%q test failed expected error while creating PV %s", name, test.nfsPV.Name)
			}
			nfsVolume := &nfsVolume{
				clientset:        f.clientset,
				pvObj:            test.nfsPV,
				annotationPrefix: "nfs.",
			}
			updatedPV, err := nfsVolume.AnnotateCreateEvent(test.nfsPV.DeepCopy())
			if test.isErrExpected && err == nil {
				t.Fatalf("%q test failed expected error to occur but got nil", name)
			}
			if !test.isErrExpected && err != nil {
				t.Fatalf("%q test failed expected error not to occur but got %v", name, err)
			}
			if !test.isErrExpected {
				if test.nfsPV.Annotations == nil {
					test.nfsPV.Annotations = map[string]string{}
				}
				test.nfsPV.Annotations[nfsVolume.annotationPrefix+collectorinterface.VolumeCreateEventAnnotation] = collectorinterface.OpenebsEventSentAnnotationValue
				if !reflect.DeepEqual(test.nfsPV, updatedPV) {
					t.Fatalf("%q test failed expected no diff but got \n%s", name, cmp.Diff(test.nfsPV, updatedPV))
				}
			}

		})
	}
}

func TestAnnotateDeleteEvent(t *testing.T) {
	f := newFixture()
	tests := map[string]struct {
		nfsPV         *corev1.PersistentVolume
		isErrExpected bool
	}{
		"when PV is requested to annotate with delete event": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv1",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
					Annotations: map[string]string{
						"pv-created-by/provisioner":           "nfs.openebs.io",
						"nfs.events.openebs.io/volume-create": "sent",
					},
				},
			},
		},
		"when PV is already annotated with delete event": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv2",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
					Annotations: map[string]string{
						"pv-created-by/provisioner":           "nfs.openebs.io",
						"nfs.events.openebs.io/volume-delete": "sent",
					},
				},
			},
		},
		"when PV delete event is pending": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv3",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
					Annotations: map[string]string{
						"pv-created-by/provisioner":           "nfs.openebs.io",
						"nfs.events.openebs.io/volume-create": "pending",
					},
				},
			},
		},
		"when PV doesn't have any annotations": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv4",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
				},
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			_, err := f.clientset.CoreV1().PersistentVolumes().Create(context.TODO(), test.nfsPV, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("%q test failed expected error while creating PV %s", name, test.nfsPV.Name)
			}
			nfsVolume := &nfsVolume{
				clientset:        f.clientset,
				pvObj:            test.nfsPV,
				annotationPrefix: "nfs.",
			}
			updatedPV, err := nfsVolume.AnnotateDeleteEvent(test.nfsPV.DeepCopy())
			if test.isErrExpected && err == nil {
				t.Fatalf("%q test failed expected error to occur but got nil", name)
			}
			if !test.isErrExpected && err != nil {
				t.Fatalf("%q test failed expected error not to occur but got %v", name, err)
			}
			if !test.isErrExpected {
				if test.nfsPV.Annotations == nil {
					test.nfsPV.Annotations = map[string]string{}
				}
				updatedPV.CreationTimestamp = test.nfsPV.CreationTimestamp
				test.nfsPV.Annotations[nfsVolume.annotationPrefix+collectorinterface.VolumeDeleteEventAnnotation] = collectorinterface.OpenebsEventSentAnnotationValue
				if !reflect.DeepEqual(test.nfsPV, updatedPV) {
					t.Fatalf("%q test failed expected no diff but got \n%s", name, cmp.Diff(test.nfsPV, updatedPV))
				}
			}

		})
	}
}

func RemoveEventFinalizer(t *testing.T) {
	f := newFixture()
	tests := map[string]struct {
		nfsPVC        *corev1.PersistentVolumeClaim
		nfsPV         *corev1.PersistentVolume
		backendPVC    *corev1.PersistentVolumeClaim
		backendPV     *corev1.PersistentVolume
		isErrExpected bool
	}{
		"when all nfs volume resources exist in the system": {
			nfsPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pvc1",
					Namespace:         "ns1",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "pv1",
				},
			},
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv1",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc1",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv1",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv1",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv1",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
		},
		"when nfs pvc doesn't exist": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv2",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc2",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv2",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv2",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv2",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
			},
		},
		"when backend PV doesn't exist": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv3",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc3",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv3",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv3",
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimLost,
				},
			},
			isErrExpected: false,
		},
		"when backend backend resources doesn't have finalizers": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv4",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc4",
						Namespace: "ns1",
					},
				},
			},
			backendPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "nfs-pv4",
					Namespace:         "openebs",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "backend-pv4",
				},
			},
			backendPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "backend-pv4",
					CreationTimestamp: metav1.Now(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
				},
			},
			isErrExpected: false,
		},
		"when backend backend resources doesn't exist": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv5",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
						"nfs.events.openebs.io/finalizer",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc5",
						Namespace: "ns1",
					},
				},
			},
			isErrExpected: false,
		},
		"when backend backend resources doesn't exist and nfspv doesn't have finalizer": {
			nfsPV: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pv6",
					CreationTimestamp: metav1.Now(),
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
					Finalizers: []string{
						"kubernetes.io/pv-protection",
					},
				},
				Spec: corev1.PersistentVolumeSpec{
					ClaimRef: &corev1.ObjectReference{
						Name:      "pvc6",
						Namespace: "ns1",
					},
				},
			},
			isErrExpected: false,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			err := f.preCreateResources(test.nfsPVC, test.backendPVC, test.nfsPV, test.backendPV)
			if err != nil {
				t.Fatalf("%q test failed expected error not to occur during pre-resource creation but got error %v", name, err)
			}
			nfsVolume := &nfsVolume{
				clientset:          f.clientset,
				pvcLister:          f.pvcInformer.Lister(),
				pvLister:           f.pvInformer.Lister(),
				pvObj:              test.nfsPV,
				nfsServerNamespace: "openebs",
				annotationPrefix:   "nfs.",
			}
			err = nfsVolume.RemoveEventFinalizer()
			if test.isErrExpected && err == nil {
				t.Fatalf("%q test failed expected error to occur but got nil", name)
			}
			if !test.isErrExpected && err != nil {
				t.Fatalf("%q test failed expected error not to occur but got %v", name, err)
			}
			if !test.isErrExpected {
				eventFinalizer := nfsVolume.annotationPrefix + collectorinterface.VolumeEventsFinalizer
				// Event Finalizer shouldn't exist after removal
				if test.nfsPV != nil {
					pv, err := f.clientset.CoreV1().PersistentVolumes().Get(context.TODO(), test.nfsPV.Name, metav1.GetOptions{})
					if err != nil {
						t.Fatalf("error shouldn't occur while fetching PV %s error: %v", test.nfsPV.Name, err)
					}
					// If finalizer exist fail the test case
					isFinalizerRemoved := removeFinalizer(&pv.ObjectMeta, eventFinalizer)
					if isFinalizerRemoved {
						t.Fatalf("event finalizer shouldn't exist on PV %s", pv.Name)
					}
				}

				if test.backendPVC != nil {
					pvc, err := f.clientset.CoreV1().PersistentVolumeClaims(test.backendPVC.Namespace).Get(context.TODO(), test.nfsPVC.Name, metav1.GetOptions{})
					if err != nil {
						t.Fatalf("error shouldn't occur while fetching PVC %s error: %v", test.nfsPVC.Name, err)
					}
					// If finalizer exist fail the test case
					isFinalizerRemoved := removeFinalizer(&pvc.ObjectMeta, eventFinalizer)
					if isFinalizerRemoved {
						t.Fatalf("event finalizer shouldn't exist on PVC %s/%s", pvc.Namespace, pvc.Name)
					}
				}

				if test.backendPV != nil {
					pv, err := f.clientset.CoreV1().PersistentVolumes().Get(context.TODO(), test.backendPV.Name, metav1.GetOptions{})
					if err != nil {
						t.Fatalf("error shouldn't occur while fetching PV %s error: %v", test.backendPV.Name, err)
					}
					// If finalizer exist fail the test case
					isFinalizerRemoved := removeFinalizer(&pv.ObjectMeta, eventFinalizer)
					if isFinalizerRemoved {
						t.Fatalf("event finalizer shouldn't exist on PV %s", pv.Name)
					}
				}
			}
		})
	}
}
