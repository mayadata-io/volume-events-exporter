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
	"fmt"

	"github.com/mayadata-io/volume-events-exporter/pkg/env"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	corev1informer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

var (
	controllerAgentName = "volume-events-controller"
)

// ClientsetListers contains clientset to perform write operations on
// K8s resources and listers to get/list from shared informer's store
type ClientsetListers struct {

	// KubeClientset is a standard kubernetes clientset
	KubeClientset kubernetes.Interface

	// PVCLister can list/get PersistentVolumeClaim from the shared informer's store
	PVCLister corev1listers.PersistentVolumeClaimLister

	// PVLister can list/get PersistentVolumes from the shared informer's  store
	PVLister corev1listers.PersistentVolumeLister

	// Recorder is an event recorder for recording Event resources to Kubernetes API.
	Recorder record.EventRecorder
}

// PVMetricsController implements volume metrics controller service
type PVMetricsController struct {
	// ClientsetListers holds clientset and listers which are required to
	// interact with Kube-APIServer
	ClientsetListers

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface

	// NFSServerNamespace states the namespace of NFSServer deployment
	NFSServerNamespace string

	pvcListerSynced cache.InformerSynced
	pvListerSynced  cache.InformerSynced
}

// PVMetricsControllerConfig helps to create new instance
// of PVMetricsController
type PVMetricsControllerConfig struct {
	// KubeClientset will initilize kubernetes clientset interface
	KubeClientset kubernetes.Interface

	// PVInformer is required to initilize PersistentVolume lister
	PVInformer corev1informer.PersistentVolumeInformer

	// PVCInformer is required to initilize PersistentVolumeClaim lister
	PVCInformer corev1informer.PersistentVolumeClaimInformer
}

// NewPVMetricsController will create new instantance of PVMetricsController
func NewPVMetricsController(config PVMetricsControllerConfig) *PVMetricsController {
	eventBroadcaster := record.NewBroadcaster()
	// eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: config.KubeClientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	pvMetricController := &PVMetricsController{
		ClientsetListers: ClientsetListers{
			KubeClientset: config.KubeClientset,
			PVCLister:     config.PVCInformer.Lister(),
			PVLister:      config.PVInformer.Lister(),
			Recorder:      recorder,
		},
		workqueue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		NFSServerNamespace: env.GetNFSServerNamespace(),
		pvcListerSynced:    config.PVCInformer.Informer().HasSynced,
		pvListerSynced:     config.PVInformer.Informer().HasSynced,
	}
	config.PVInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    pvMetricController.addPV,
		UpdateFunc: pvMetricController.updatePV,
		DeleteFunc: pvMetricController.deletePV,
	})
	return pvMetricController
}

func (pController *PVMetricsController) addPV(obj interface{}) {
	pvObj, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		runtime.HandleError(fmt.Errorf("Couldn't get PV object %#v", obj))
		return
	}
	klog.V(4).Infof("Queuing PV %s for add event", pvObj.Name)
	pController.enqueue(pvObj)
}

func (pController *PVMetricsController) updatePV(oldObj, newObj interface{}) {
	//TODO: Undo below comments if required
	pvObj, ok := newObj.(*corev1.PersistentVolume)
	if !ok {
		runtime.HandleError(fmt.Errorf("Couldn't get PV object %#v", newObj))
		return
	}
	klog.V(4).Infof("Queuing PV %s for update event", pvObj.Name)
	pController.enqueue(pvObj)
}

func (pController *PVMetricsController) deletePV(obj interface{}) {
	pvObj, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		runtime.HandleError(fmt.Errorf("Couldn't get PV object %#v", obj))
		return
	}
	klog.V(4).Infof("Queuing PV %s for delete event", pvObj.Name)
	pController.enqueue(pvObj)
}

func (pController *PVMetricsController) enqueue(pv *corev1.PersistentVolume) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(pv); err != nil {
		runtime.HandleError(err)
		return
	}
	pController.workqueue.Add(key)
}
