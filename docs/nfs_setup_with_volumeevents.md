# QuickStart

## Prerequisites
Before installing nfs-provisioner make sure your Kubernetes cluster meets the following prerequisites:

1. Kubernetes version 1.18
2. NFS Client is installed on all nodes that will run a pod that mounts an `openebs-rwx` volume.
   Here's how to prepare an NFS client on some common Operating Systems:

   | OPERATING SYSTEM |  How to install NFS Client package                                |
   | ---------------- | -------------------------------------------------------- |
   | RHEL/CentOS/Fedora  |run *sudo yum install nfs-utils -y*      |
   | Ubuntu/Debian   |run *sudo apt install nfs-common -y*     |
   | MacOS     |Should work out of the box |

## Install

### Install NFS Provisioner through kubectl

To install NFS Provisioner along with volume-event-exporter, apply below yaml
```yaml
# Create the OpenEBS namespace
apiVersion: v1
kind: Namespace
metadata:
  name: openebs
---
# Create Maya Service Account
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openebs-maya-operator
  namespace: openebs
---
# Define Role that allows operations on K8s pods/deployments
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: openebs-maya-operator
rules:
- apiGroups: ["*"]
  resources: ["nodes", "nodes/proxy"]
  verbs: ["*"]
- apiGroups: ["*"]
  resources: ["namespaces", "services", "pods", "pods/exec", "deployments", "deployments/finalizers", "replicationcontrollers", "replicasets", "events", "endpoints", "configmaps", "secrets", "jobs", "cronjobs"]
  verbs: ["*"]
- apiGroups: ["*"]
  resources: ["statefulsets", "daemonsets"]
  verbs: ["*"]
- apiGroups: ["*"]
  resources: ["resourcequotas", "limitranges"]
  verbs: ["list", "watch"]
- apiGroups: ["*"]
  resources: ["ingresses", "horizontalpodautoscalers", "verticalpodautoscalers", "poddisruptionbudgets", "certificatesigningrequests"]
  verbs: ["list", "watch"]
- apiGroups: ["*"]
  resources: ["storageclasses", "persistentvolumeclaims", "persistentvolumes"]
  verbs: ["*"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: [ "get", "list", "create", "update", "delete", "patch"]
- apiGroups: ["openebs.io"]
  resources: [ "*"]
  verbs: ["*"]
- nonResourceURLs: ["/metrics"]
  verbs: ["get"]
---
# Bind the Service Account with the Role Privileges.
# TODO: Check if default account also needs to be there
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: openebs-maya-operator
subjects:
- kind: ServiceAccount
  name: openebs-maya-operator
  namespace: openebs
roleRef:
  kind: ClusterRole
  name: openebs-maya-operator
  apiGroup: rbac.authorization.k8s.io
---
# Create openebs-nfs-provisioner deployment to provision
# dynamic NFS volumes
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openebs-nfs-provisioner
  namespace: openebs
  labels:
    name: openebs-nfs-provisioner
    openebs.io/component-name: openebs-nfs-provisioner
    openebs.io/version: dev
spec:
  selector:
    matchLabels:
      name: openebs-nfs-provisioner
      openebs.io/component-name: openebs-nfs-provisioner
  replicas: 1
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        name: openebs-nfs-provisioner
        openebs.io/component-name: openebs-nfs-provisioner
        openebs.io/version: dev
    spec:
      serviceAccountName: openebs-maya-operator
      containers:
      - name: openebs-provisioner-nfs
        imagePullPolicy: IfNotPresent
        image: openebs/provisioner-nfs:ci
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: OPENEBS_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        # OPENEBS_SERVICE_ACCOUNT provides the service account of this pod as
        # environment variable
        - name: OPENEBS_SERVICE_ACCOUNT
          valueFrom:
            fieldRef:
              fieldPath: spec.serviceAccountName
        - name: OPENEBS_IO_ENABLE_ANALYTICS
          value: "true"
        - name: OPENEBS_IO_NFS_SERVER_USE_CLUSTERIP
          value: "true"
        - name: OPENEBS_IO_INSTALLER_TYPE
          value: "openebs-operator-nfs"
        - name: OPENEBS_IO_NFS_HOOK_CONFIGMAP
          value: "hook-config"
        # OPENEBS_IO_NFS_SERVER_NS defines the namespace for nfs-server deployment
        - name: OPENEBS_IO_NFS_SERVER_NS
          value: "openebs"
        # OPENEBS_IO_NFS_SERVER_IMG defines the nfs-server-alpine image name to be used
        # while creating nfs volume
        - name: OPENEBS_IO_NFS_SERVER_IMG
          value: openebs/nfs-server-alpine:ci
        livenessProbe:
          exec:
            command:
            - sh
            - -c
            - test `pgrep "^provisioner-nfs.*"` = 1
          initialDelaySeconds: 30
          periodSeconds: 60
      - name: volume-events-collector
        imagePullPolicy: IfNotPresent
        image: mayadataio/volume-events-exporter:ci
        args:
          - "--leader-election=false"
          - "--generate-k8s-events=true"
        env:
        # OPENEBS_IO_NFS_SERVER_NS defines the namespace of nfs-server deployment
        - name: OPENEBS_IO_NFS_SERVER_NS
          value: "openebs"
        # CALLBACK_URL defines the server address to POST volume events information.
        # It must be a valid address
        # NOTE: Update the below URL
        - name: CALLBACK_URL
          value: "http://127.0.0.1:9000/event-server"
        # CALLBACK_TOKEN defines the authentication token required to interact with server.
        # NOTE: Update the below token value
        - name: CALLBACK_TOKEN
          value: ""
        # RESYNC_INTERVAL defines how frequently controller has to look for volumes defaults
        # to 60 seconds. If activity of provisioning & de-provisioning is less then set it
        # to some higher value
        #- name: RESYNC_INTERVAL
        #  value: "120"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: hook-config
  namespace: openebs
data:
  config: |
    hooks:
      addOrUpdateEntriesOnCreateVolumeEvent:
        backendPV:
          finalizers:
          - nfs.events.openebs.io/finalizer
        backendPVC:
          finalizers:
          - nfs.events.openebs.io/finalizer
        nfsPV:
          annotations:
            events.openebs.io/required: "true"
          finalizers:
          - nfs.events.openebs.io/finalizer
        name: createHook
    version: 1.0.0
```

- Apply above yaml via kubectl `kubectl apply -f <above.yaml>`

Above command will install NFS Provisioner along with volume-event-exporter(as a sidecar) to export volume events to external service. Service location can be configured by updating values of `CALLBACK_URL` env and token(if applicable, for authentication) via `CALLBACK_TOKEN`.


## Provision NFS Volume

To provision NFS volume, create an NFS StorageClass with the required backend StorageClass. Example StorageClass YAML is:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: openebs-volume-event-rwx
  annotations:
    openebs.io/cas-type: nfsrwx
    cas.openebs.io/config: |
      - name: NFSServerType
        value: "kernel"
      # Replace `openebs-hostpath` with corresponding backend StorageClass
      - name: BackendStorageClass
        value: "openebs-hostpath"
provisioner: openebs.io/nfsrwx
reclaimPolicy: Delete
```

Above storageclass is using *openebs-hostpath* Storageclass as BackendStorageclass. Please update with relavent backend storageclass.

Once the Storageclass is successfully created, you can provision a volume by creating a PVC with the above storageclass. Sample PVC YAML is as below:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-pvc
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: "openebs-volume-event-rwx"
  resources:
    requests:
      storage: 1Gi
```

To check the binding of PVC, run below command:
```sh
kubectl get pvc <PVC-NAME> -n <PVC-NAMESPACE>
```

Below is the sample output:
```sh
NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
nfs-pvc   Bound    pvc-5dc44d4f-3141-40dd-85df-fa6544644f49   1Gi        RWX            openebs-rwx    17s
```

In above example, provisioner has created NFS PV named *pvc-b5d6caae-831c-4a4e-97d8-ddfe3ca9a646*.

To verify whether volume-event-exporter exported create volume information, check the events of NFS pv
```sh
kubectl describe <PV-NAME>

Events:
Type    Reason            Age    From                      Message
----    ------            ----   ----                      -------
Normal  EventInformation  2m20s  volume-events-controller  Exported volume create information
```

## Delete NFS Volume

Since NFS PV is dynamically provisioned, you can delete NFS PV by deleting PVC.

- To delete created PVC [Provision NFS Volume](#provision-nfs-volume)
  ```sh
  kubectl delete pvc <PVC-NAME> -n <PVC-NAMESPACE>

  persistentvolumeclaim "nfs-pvc" deleted
  ```

- To verify whether volume-event-exporter exported delete volume information, check the events of NFS pv
  ```sh
  kubectl get events
  LAST SEEN   TYPE     REASON                    OBJECT                                                      MESSAGE
  2s          Normal   EventInformation          persistentvolume/pvc-5dc44d4f-3141-40dd-85df-fa6544644f49   Exported volume delete information
  ```
