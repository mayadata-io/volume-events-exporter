#!/bin/bash

# Copyright © 2021 The MayaData Authors

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


mkdir -p /tmp/openebs
kubectl  apply -f https://raw.githubusercontent.com/openebs/dynamic-localpv-provisioner/v2.12.0/deploy/kubectl/hostpath-operator.yaml
wget https://raw.githubusercontent.com/openebs/dynamic-localpv-provisioner/v2.12.0/deploy/kubectl/openebs-hostpath-sc.yaml -O /tmp/openebs-lite-sc.yaml
sed -i  's/value\: \"\/var\/openebs\/local\/\"/value\: \"\/tmp\/openebs\/\"/' /tmp/openebs-lite-sc.yaml

function waitForDeployment() {
	DEPLOY=$1
	NS=$2
	CREATE=true

	if [ $# -eq 3 ] && ! $3 ; then
		CREATE=false
	fi

	for i in $(seq 1 50) ; do
		kubectl get deployment -n ${NS} ${DEPLOY}
		kstat=$?
		if [ $kstat -ne 0 ] && ! $CREATE ; then
			return
		elif [ $kstat -eq 0 ] && ! $CREATE; then
			sleep 3
			continue
		fi

		replicas=$(kubectl get deployment -n ${NS} ${DEPLOY} -o jsonpath='{.status.readyReplicas}')
		if [ "$replicas" == "1" ]; then
			break
		else
			echo "Waiting for ${DEPLOY} to be ready"
			kubectl logs deploy/${DEPLOY} -n ${NS}
			sleep 10
		fi
	done
}

kubectl apply -f /tmp/openebs-lite-sc.yaml
waitForDeployment openebs-localpv-provisioner openebs

sed -i 's/#- name: BackendStorageClass/- name: BackendStorageClass/' deploy/kubectl/openebs-nfs-provisioner.yaml
sed -i 's/#  value: "openebs-hostpath"/  value: "openebs-hostpath"/' deploy/kubectl/openebs-nfs-provisioner.yaml

kubectl  apply -f deploy/kubectl/openebs-nfs-provisioner.yaml

waitForDeployment openebs-nfs-provisioner openebs
kubectl get sc -o yaml
