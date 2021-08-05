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
	annotationProcessEventKey    = "events.openebs.io/required"
	eventRequiredAnnotationValue = "true"
)

// processVolumeEvents reconciles PersistentVolume and will
// send volume information to configured callback URL only if volume
// is marked to send volume information
func (pController *PVEventController) processVolumeEvents(key string) (bool, error) {
	klog.V(4).Infof("Started syncing PV: %s to send send metrics information", key)

	// Convert the key string into a distinct namespace and name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return false, nil
	}

	// Get PV resource with above name
	pvObj, err := pController.kubeClientset.CoreV1().PersistentVolumes().Get(context.TODO(), name, metav1.GetOptions{})
	if k8serror.IsNotFound(err) {
		runtime.HandleError(fmt.Errorf("PV %q has been deleted", key))
		return false, nil
	}
	if err != nil {
		return false, err
	}

	err = pController.sync(pvObj)
	if err != nil {
		pController.recorder.Event(pvObj, corev1.EventTypeWarning, "EventInformation", err.Error())
	}

	// Something went wrong let's retry after sometime
	return true, err
}

// sync will send volume create and delete information to configured REST services
// NOTE: It will ensure to send event information only once
func (pController *PVEventController) sync(pvObj *corev1.PersistentVolume) error {
	klog.V(4).Infof("Reconciling PV %s to send volume events", pvObj.Name)
	if !shouldSendEvent(pvObj) {
		// If no action is required then return from here
		return nil
	}

	klog.Infof("Got PV %s to send volume events", pvObj.Name)
	eventSender, err := pController.getEventSender(pvObj)
	if err != nil {
		return err
	}

	// Send create event information
	err = pController.sendCreateEvent(eventSender, pvObj)
	if err != nil {
		return err
	}

	// Send delete event information
	err = pController.sendDeleteEvent(eventSender, pvObj)
	if err != nil {
		return err
	}

	return nil
}

// sendCreateEvent will push create volume event to configured server
// NOTE: If event is already sent then sendCreateEvent will return nil
func (pController *PVEventController) sendCreateEvent(
	eventSender collectorinterface.EventsSender,
	pvObj *corev1.PersistentVolume) error {

	// Send create information
	if !isCreateVolumeEventSent(pvObj) {
		// Get create event related data
		data, err := eventSender.CollectCreateEvents()
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
		pController.recorder.Event(pvObj, corev1.EventTypeNormal, "EventInformation", "Exported volume create information")
		klog.Infof("Successfully sent create volume %s event to server", pvObj.Name)
	}
	return nil
}

// sendDeleteEvent will push delete volume events to configured REST server.
// If response from server is `OK` then sendDeleteEvent will remove event
// finalizer(events.openebs.io/finalizer) on all corresponding resources
// NOTE: If event is already sent then following func will only remove finalizers
//       from dependent resource
func (pController *PVEventController) sendDeleteEvent(
	eventSender collectorinterface.EventsSender,
	pvObj *corev1.PersistentVolume) error {
	if pvObj.DeletionTimestamp != nil {
		if !isDeleteVolumeEventSent(pvObj) {
			// Get delete event related data
			data, err := eventSender.CollectDeleteEvents()
			if err != nil {
				return errors.Wrapf(err, "failed to get delete event data of volume %s", pvObj.Name)
			}

			// Send delete event data
			err = eventSender.Send(data)
			if err != nil {
				return errors.Wrapf(err, "failed to send delete event data of volume %s to server", pvObj.Name)
			}

			// Annotate resource saying delete event is sent to REST server
			_, err = eventSender.AnnotateDeleteEvent(pvObj)
			if err != nil {
				return errors.Wrapf(err, "failed to annotate volume %s with delete event information", pvObj.Name)
			}
			pController.recorder.Event(pvObj, corev1.EventTypeNormal, "EventInformation", "Exported volume delete information")
			klog.Infof("Successfully sent delete volume %s event to server", pvObj.Name)
		}

		err := eventSender.RemoveEventFinalizer()
		if err != nil {
			return errors.Wrapf(err, "failed to remove finalizers on volume %s", pvObj.Name)
		}
	}
	return nil
}

// getEventSender will return event sender which implements all the methods of event sender interface
func (pController *PVEventController) getEventSender(pvObj *corev1.PersistentVolume) (collectorinterface.EventsSender, error) {
	// Add more types based on underlying volume type
	casType, isCASTypeExist := pvObj.Labels[OpenEBSCASLabelKey]
	if !isCASTypeExist {
		// If volume is provisioned via CSI
		if pvObj.Spec.CSI != nil {
			casType = pvObj.Spec.CSI.VolumeAttributes[OpenEBSCASLabelKey]
		}
	}

	if casType == "" {
		return nil, errors.Errorf("CAS type is not found on volume %s", pvObj.Name)
	}
	switch casType {
	case nfspv.OpenEBSNFSCASLabelValue:
		return tokenauth.NewTokenClient(
			nfspv.NewNFSVolume(
				pController.kubeClientset,
				pController.pvcLister,
				pController.pvLister,
				pvObj,
				collectorinterface.JSONDataType)), nil
	}
	return nil, errors.Errorf("event sender is not available for volume %s of CAS type %s", pvObj.Name, casType)
}

// shouldSendEvent will return true based on following conditions:
// 1. Retrun true if volume requires event to be exported(based on annotation "events.openebs.io/required": "true")
// 2. Retrun true if deletion timestamp is set and create event is not yet send
// 3. else return false
func shouldSendEvent(pvObj *corev1.PersistentVolume) bool {
	if value, isExist := pvObj.Annotations[annotationProcessEventKey]; !isExist || value != eventRequiredAnnotationValue {
		return false
	}

	// If create volume events is not yet send then return true
	if !isCreateVolumeEventSent(pvObj) {
		return true
	}
	return pvObj.DeletionTimestamp != nil
}

// isCreateVolumeEventSent will return true if volume has
// suffix(event.openebs.io/volume-create) in annotations and value is sent
func isCreateVolumeEventSent(pvObj *corev1.PersistentVolume) bool {
	for key, value := range pvObj.Annotations {
		if strings.HasSuffix(key, collectorinterface.VolumeCreateEventAnnotation) {
			return value == collectorinterface.OpenebsEventSentAnnotationValue
		}
	}
	return false
}

// isDeleteVolumeEventSent will return true if volume has
// suffix(event.openebs.io/volume-delete) in annotations and value is sent
func isDeleteVolumeEventSent(pvObj *corev1.PersistentVolume) bool {
	for key, value := range pvObj.Annotations {
		if strings.HasSuffix(key, collectorinterface.VolumeDeleteEventAnnotation) {
			return value == collectorinterface.OpenebsEventSentAnnotationValue
		}
	}
	return false
}
