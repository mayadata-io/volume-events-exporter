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

package collectorinterface

import (
	corev1 "k8s.io/api/core/v1"
)

type DataType string

const (
	JSONDataType DataType = "JSON"
	YAMLDataType DataType = "YAML"
)

type EventsSender interface {
	// Send will push given event information to configured server
	// NOTE: Send should convert data into required format before sending
	//		 to server
	Send(data string) error
	VolumeEventCollector
}

type VolumeEventCollector interface {
	// CollectCreateEventData should return data required for create event
	CollectCreateEventData() (string, error)
	// CollectDeleteEventData should return data required for delete event
	CollectDeleteEventData() (string, error)
	// RemoveEventFinalizer should remove the finalizer on all dependent resources
	RemoveEventFinalizer() error
	// AnnotateCreateEvent will set create event annotation on PersistentVolume object
	AnnotateCreateEvent(pvObj *corev1.PersistentVolume) (*corev1.PersistentVolume, error)
	// AnnotateDeleteEvent will set delete event annotation on PersistentVolume object
	AnnotateDeleteEvent(pvObj *corev1.PersistentVolume) (*corev1.PersistentVolume, error)
}
