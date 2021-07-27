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

package pvcontroller

import (
	"context"
	"fmt"
	"strings"

	collectorinterface "github.com/mayadata-io/volume-events-exporter/pkg/collectorinterface"
	"github.com/mayadata-io/volume-events-exporter/pkg/collectorinterface/tokenauth"
	"github.com/mayadata-io/volume-events-exporter/pkg/nfspv"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	eventRequiredAnnotationKey   = "events.openebs.io/required"
	eventRequiredAnnotationValue = "true"
)

// syncToSendVolumeEvents reconciles PersistentVolume and will
// send volume information to configured callback URL if it is required
// send (or) volume event information is not yet sent
func (pController *PVMetricsController) syncToSendVolumeEvents(key string) error {
	klog.V(4).Infof("Started syncing PV: %s to send send metrics information", key)

	// Convert the name string into a distinct namespace and name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get PV resource with above name
	pvObj, err := pController.KubeClientset.CoreV1().PersistentVolumes().Get(context.TODO(), name, metav1.GetOptions{})
	if k8serror.IsNotFound(err) {
		runtime.HandleError(fmt.Errorf("PV %q has been deleted", key))
		return nil
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating cache store
	err = pController.sync(pvObj.DeepCopy())
	// TODO: If events needs to be generated undo the comments
	// if err != nil {
	// 	pController.Recorder.Event(pvObj, corev1.EventTypeWarning, "EventInformation", err.Error())
	// }
	return err
}

func (pController *PVMetricsController) sync(pvObj *corev1.PersistentVolume) error {
	klog.V(4).Infof("Reconciling PV %s to send volume events", pvObj.Name)
	if canSkipEventCollection(pvObj) {
		return nil
	}

	klog.Infof("Got PV %s to send volume events", pvObj.Name)
	eventSender, err := pController.getEventSender(pvObj)
	if err != nil {
		return err
	}
	if eventSender == nil {
		return nil
	}

	// Send create information
	if !isCreateVolumeEventSent(pvObj) {
		// Get create event related data
		data, err := eventSender.CollectCreateEventData()
		if err != nil {
			return errors.Wrapf(err, "failed to get create event data of volume %s", pvObj.Name)
		}

		// Send create event data
		err = eventSender.Send(data)
		if err != nil {
			return errors.Wrapf(err, "failed to send create event data of volume %s to server", pvObj.Name)
		}

		_, err = eventSender.AnnotateCreateEvent(pvObj)
		if err != nil {
			return err
		}
		klog.Infof("Successfully sent create volume %s event to server", pvObj.Name)
	}

	// Send delete information
	if pvObj.DeletionTimestamp != nil {
		if !isDeleteVolumeEventSent(pvObj) {
			// Get delete event related data
			data, err := eventSender.CollectDeleteEventData()
			if err != nil {
				return errors.Wrapf(err, "failed to get delete event data of volume %s", pvObj.Name)
			}

			// Send delete event data
			err = eventSender.Send(data)
			if err != nil {
				return errors.Wrapf(err, "failed to send delete event data of volume %s to server", pvObj.Name)
			}

			_, err = eventSender.AnnotateDeleteEvent(pvObj)
			if err != nil {
				return errors.Wrapf(err, "failed to annotate volume %s with delete event information", pvObj.Name)
			}
			klog.Infof("Successfully sent delete volume %s event to server", pvObj.Name)
		}
		err = eventSender.RemoveEventFinalizer()
		if err != nil {
			return errors.Wrapf(err, "failed to remove finalizers on volume %s", pvObj.Name)
		}
	}
	return nil
}

func (pController *PVMetricsController) getEventSender(pvObj *corev1.PersistentVolume) (collectorinterface.EventsSender, error) {
	var eventFinalizer string
	for _, finalizer := range pvObj.Finalizers {
		if strings.HasSuffix(finalizer, collectorinterface.OpenebsEventFinalizerSuffix) {
			eventFinalizer = finalizer
			break
		}
	}
	if eventFinalizer == "" {
		klog.Warningf("unable to find volume %s type from finalizers %v", pvObj.Name, pvObj.Finalizers)
		return nil, nil
	}

	if value, ok := pvObj.Labels[nfspv.OpenEBSNFSLabelKey]; ok && value == "true" {
		return tokenauth.NewTokenClient(
			nfspv.NewNFSVolume(
				pController.KubeClientset,
				pController.PVCLister,
				pController.PVLister,
				pvObj,
				pController.NFSServerNamespace)), nil
	}
	eventSenderType := eventFinalizer[:len(eventFinalizer)-len(collectorinterface.OpenebsEventFinalizerSuffix)]
	return nil, errors.Errorf("event sender is not available for volume type %s", eventSenderType)
}

// canSkipEventCollection will return true based on following conditions:
// 1. Retrun true if volume doesn't require event to be exported(based on annotation "events.openebs.io/required": "true")
// 2. Retrun true if deletion timestamp is not set and create event already sent
// 3. else return false
func canSkipEventCollection(pvObj *corev1.PersistentVolume) bool {
	if value, isExist := pvObj.Annotations[eventRequiredAnnotationKey]; !isExist || value != eventRequiredAnnotationValue {
		return true
	}
	return pvObj.DeletionTimestamp == nil && isCreateVolumeEventSent(pvObj)
}

// isCreateVolumeEventSent will return true if volume has
// suffix(event.openebs.io/volume-create) in annotations and value is sent
func isCreateVolumeEventSent(pvObj *corev1.PersistentVolume) bool {
	for key, value := range pvObj.Annotations {
		if strings.HasSuffix(key, collectorinterface.OpenebsCreateAnnotationSuffix) {
			return value == collectorinterface.OpenebsSentAnnotationValue
		}
	}
	return false
}

// isDeleteVolumeEventSent will return true if volume has
// suffix(event.openebs.io/volume-delete) in annotations and value is sent
func isDeleteVolumeEventSent(pvObj *corev1.PersistentVolume) bool {
	for key, value := range pvObj.Annotations {
		if strings.HasSuffix(key, collectorinterface.OpenebsDeleteAnnotationSuffix) {
			return value == collectorinterface.OpenebsSentAnnotationValue
		}
	}
	return false
}
