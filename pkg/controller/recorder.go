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
