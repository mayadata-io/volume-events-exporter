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

const (
	// VolumeCreateEventAnnotation holds annotation key which represents
	// status of volume creation event
	VolumeCreateEventAnnotation = "event.openebs.io/volume-create"
	// VolumeDeleteEventAnnotation holds annotation key which represents
	// status of volume deletion event
	VolumeDeleteEventAnnotation = "event.openebs.io/volume-delete"
	// OpenebsEventSentAnnotationValue holds annotation value which states
	// corresponding volume event was sent to server
	OpenebsEventSentAnnotationValue = "sent"
	// VolumeEventsFinalizer holds finalizer value to ensure delivery of volume
	// events
	VolumeEventsFinalizer = "events.openebs.io/finalizer"
)

type DataType string

const (
	JSONDataType DataType = "JSON"
	YAMLDataType DataType = "YAML"
)
