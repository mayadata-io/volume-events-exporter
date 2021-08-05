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

package helper

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// GetPatchData will serialize given objects and return patch data
// that can be passed to StrategicMergePatch
func GetPatchData(oldObj, newObj interface{}) ([]byte, []byte, error) {
	oldData, err := json.Marshal(oldObj)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal old object failed: %v", err)
	}
	newData, err := json.Marshal(newObj)
	if err != nil {
		return nil, nil, fmt.Errorf("mashal new object failed: %v", err)
	}
	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, oldObj)
	if err != nil {
		return nil, nil, fmt.Errorf("CreateTwoWayMergePatch failed: %v", err)
	}
	return patchBytes, oldData, nil
}

// RemoveFinalizer will remove specified finalizer from provided meta object
func RemoveFinalizer(objectMeta *metav1.ObjectMeta, finalizer string) bool {
	originalFinalizers := objectMeta.GetFinalizers()

	var finalizerCopy []string
	var isFinalizerExist bool
	for _, curFinalizer := range originalFinalizers {
		if curFinalizer == finalizer {
			isFinalizerExist = true
			continue
		}
		finalizerCopy = append(finalizerCopy, curFinalizer)
	}
	objectMeta.Finalizers = finalizerCopy
	return isFinalizerExist
}
