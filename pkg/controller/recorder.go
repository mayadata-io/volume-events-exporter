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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// Recorder is a wrapper over EventRecorder which helps to
// control event generation
type Recorder struct {
	record.EventRecorder
	// generateEvents will control the event generation based on its value
	generateEvents bool
}

// Event is a wrapper over original Event which will help to generate events based on flag
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
