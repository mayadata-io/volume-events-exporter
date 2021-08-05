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
	"fmt"
	"os"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	corev1informer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

var (
	volumeEventControllerName = "volume-events-controller"
	sharedInformerInterval    = time.Second * 60
)

// PVEventController implements volume events controller service
type PVEventController struct {
	*controller

	// kubeClientset is a standard kubernetes clientset
	kubeClientset kubernetes.Interface

	// pvcLister can list/get PersistentVolumeClaim from the shared informer's store
	pvcLister corev1listers.PersistentVolumeClaimLister

	// pvLister can list/get PersistentVolumes from the shared informer's  store
	pvLister corev1listers.PersistentVolumeLister

	// Recorder is an event recorder for recording Event resources to Kubernetes API.
	recorder *Recorder
}

// Recorder is a wrapper over EventRecorder which helps to
// control event generation
type Recorder struct {
	record.EventRecorder
	// generateEvents will control the event generation based on its value
	generateEvents bool
}

// NewPVEventController will create new instantance of PVEventController
func NewPVEventController(kubeClientset kubernetes.Interface,
	pvInformer corev1informer.PersistentVolumeInformer,
	pvcInformer corev1informer.PersistentVolumeClaimInformer,
	numWorker int,
	generateEvents bool) Controller {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: volumeEventControllerName})
	recorder := &Recorder{
		EventRecorder:  eventRecorder,
		generateEvents: generateEvents,
	}

	pvEventController := &PVEventController{
		controller:    newController(volumeEventControllerName, numWorker),
		kubeClientset: kubeClientset,
		pvcLister:     pvcInformer.Lister(),
		pvLister:      pvInformer.Lister(),
		recorder:      recorder,
	}
	pvEventController.reconcile = pvEventController.processVolumeEvents
	pvEventController.reconcilePeriod = GetSyncInterval()
	pvEventController.cacheSyncWaiters = append(pvEventController.cacheSyncWaiters,
		[]cache.InformerSynced{pvInformer.Informer().HasSynced, pvcInformer.Informer().HasSynced}...)

	// Add event handlers
	pvInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    pvEventController.addPV,
		UpdateFunc: pvEventController.updatePV,
		DeleteFunc: pvEventController.deletePV,
	})
	return pvEventController
}

func (pController *PVEventController) addPV(obj interface{}) {
	pvObj, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("Couldn't get PV object %#v", obj))
		return
	}
	klog.V(4).Infof("Queuing PV %s for add event", pvObj.Name)
	pController.enqueue(pvObj)
}

func (pController *PVEventController) updatePV(oldObj, newObj interface{}) {
	pvObj, ok := newObj.(*corev1.PersistentVolume)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("Couldn't get PV object %#v", newObj))
		return
	}
	klog.V(4).Infof("Queuing PV %s for update event", pvObj.Name)
	pController.enqueue(pvObj)
}

func (pController *PVEventController) deletePV(obj interface{}) {
	pvObj, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("Couldn't get PV object %#v", obj))
		return
	}
	klog.V(4).Infof("Queuing PV %s for delete event", pvObj.Name)
	pController.enqueue(pvObj)
}

// GetSyncInterval gets the resync interval from environment variable.
// If missing or zero then default to SharedInformerInterval otherwise
// return the obtained value
func GetSyncInterval() time.Duration {
	resyncInterval, err := strconv.Atoi(os.Getenv("RESYNC_INTERVAL"))
	if err != nil || resyncInterval == 0 {
		klog.Warningf("Incorrect resync interval %q obtained from env, defaulting to %q seconds", resyncInterval, sharedInformerInterval)
		return sharedInformerInterval
	}
	return time.Duration(resyncInterval) * time.Second
}

// Event is a wrapper over original Event which will help to generate events
func (r *Recorder) Event(object runtime.Object, eventtype, reason, message string) {
	if !r.generateEvents {
		return
	}
	r.EventRecorder.Event(object, eventtype, reason, message)
}

// Eventf is wrapper over original Eventf, it is just like Event, but with Sprintf for the message field.
func (r *Recorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	if !r.generateEvents {
		return
	}
	r.EventRecorder.Eventf(object, eventtype, reason, messageFmt, args...)
}

// AnnotatedEventf is wrapper over original AnnotatedEventd just like eventf, but with annotations attached
func (r *Recorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	if !r.generateEvents {
		return
	}
	r.EventRecorder.AnnotatedEventf(object, annotations, eventtype, reason, messageFmt, args...)
}
