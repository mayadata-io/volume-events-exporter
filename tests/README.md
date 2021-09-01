# Integration Test

Volume-event-exporter makes use of ginkgo & gomega libraries to implement its integration tests.

To run the integrations test:
1. Copy kubeconfig file into path ~/.kube/config or set the KUBECONFIG env.
2. Run `make sanity-test`

**Note**:
- Integration test will spin up RPC server and will tear down server at end of the test. By default server will run on 9090 port and it can configured with option `--port`.
- To run a specific test following command can be used:
  ```sh
  ginkgo -v -focus="TEST NFS PVC CREATE & DELETE EVENTS" -- -address=172.18.0.1 -port=9091
  ```
  In above `-focus` can have test name

Following parameters can be configured as a arguments to test:
- kubeconfig: Path to kubeconfig to interact with Kubernetes APIServer.
- address: Address on which server will start(Defaults to system IPAddress).
- port: Server port on which requests are served
- type: Defines the type of the RPC server, as of now only RESTfull server is supported.


Available Integration Tests:
| Test Name       | Expected behavior | Test Link  |
| --------------- | ----------------- | ---------- |
| Create NFS PVC with reclaim policy Delete | <ul> <li> Verify whether volume_event_controller sent volume_provisioned JSON data to configured server <ul> <li> nfs_pvc </li> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pv </li> along with creationTimestamp on `nfs_pv` </ul> </ul> </li>| [nfs_sanity_test.go](./nfs_sanity_test.go) |
| Delete NFS PVC with reclaim policy Delete | <ul> <li> Verify whether volume_event_controller sent volume_deprovisioned JSON data to configured server <ul> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pvc </li> along with creation and deletion timestamp on `nfs_pv` </ul> </ul> </li> | [nfs_sanity_test.go](./nfs_sanity_test.go) |
| Create NFS PVC with ReclaimPolicy Retain | <ul> <li> Verify whether volume_event_controller sent volume_provisioned JSON data to configured server <ul> <li> nfs_pvc </li> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pv </li> along with creationTimestamp on `nfs_pv` </ul> </ul> </li> | [nfs_pvc_retain_test.go](./nfs_pvc_retain_test.go) |
| Delete NFS PVC with ReclaimPolicy Retain | <ul> <li> Verify that volume_event_controller shouldn't send event when `nfs_pvc` is deleted </li> </ul> | [nfs_pvc_retain_test.go](./nfs_pvc_retain_test.go) |
| Delete released NFS PV | <ul> <li> Verify whether volume_event_controller sent volume_deprovisioned JSON data to configured server <ul> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pv </li> along with creation and deletion timestamp on `nfs_pv` </ul> </li> </ul> | [nfs_pvc_retain_test.go](./nfs_pvc_retain_test.go) |
| Create NFS PVC with invalid backend SC | <ul> <li> Verify that volume_event_controller **SHOULD NOT** send any create event data to configured server </li> </ui>| [nfs_pvc_invalid_backend_sc_test.go](./nfs_pvc_invalid_backend_sc_test.go) |
| Delete NFS PVC with invalid backend SC | <ul> <li> Verify that volume_event_controller **SHOULD NOT** send any delete event data to configured server </li> </ui>| [nfs_pvc_invalid_backend_sc_test.go](./nfs_pvc_invalid_backend_sc_test.go) |
| Disable sidecar and create PVC with reclaim policy Delete. Once the NFS PVC is bounded start sidecar  | <ul> <li> Verify whether volume_event_controller sent following volume_provisioned JSON data to configured server after enabiling volume_event_exporter <ul> <li> nfs_pvc </li> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pv </li> along with creationTimestamp on `nfs_pv` </ul> </ul> </li>| [nfs_pvc_creation_scaling_down_sidecar_test.go](./nfs_pvc_creation_scaling_down_sidecar_test.go) |
| Stop the volume-event-exporter sidecar and Create NFS PVC with ReclaimPolicy Retain. Once NFS PVC is bound, start the volume-event-exporter sidecar | <ul> Verify whether volume_event_controller sent volume_provisioned JSON data to configured server <li> nfs_pvc </li> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pvc </li> along with creation timestamp on `nfs_pv` </ul> | [nfs_create_retain_pv_disable_exporter_test.go](./nfs_create_retain_pv_disable_exporter_test.go) |
| Disable sidecar then provision & de-provision NFS PVC with reclaim policy Delete. Once the NFS PVC is deleted start sidecar  | <ul> <li> Verify whether volume_event_controller sent following volume_provisioned JSON data to configured server after enabling volume_event_exporter <ul> <li> nfs_pvc </li> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pv </li> along with creationTimestamp on `nfs_pv` </ul> </li> <li> verify whether volume_event_controller sent following volume_deprovisioned JSON data to configured server after enabling volume_event_exporter <ul> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pv </li> along with creation and deletion timestamp on `nfs_pv` </ul> </li> </ul> | [nfs_pvc_create_delete_disabling_sidecar_test.go](./nfs_pvc_create_delete_disabling_sidecar_test.go) |
| Stop the volume-event-exporter sidecar and Create NFS PVC with ReclaimPolicy Retain. Delete NFS PVC. Start the volume-event-exporter sidecar | <ul> Verify whether volume_event_controller send volume_provisioned JSON data to configured server <li> nfs_pvc </li> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pvc </li> along with creation timestamp on `nfs_pv` </ul> <ul>Verify provisioner doesn’t send volume_deleted json.</ul> | [nfs_delete_retain_pv_disable_exporter_test.go](./nfs_delete_retain_pv_disable_exporter_test.go) |
| Stop the volume-events-exporter sidecar and delete the released NFS PV. Start the volume-events-exporter sidecar | <ul> Verify provisioner send volume_deleted json having information of <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pvc </li> along with creation and deletion timestamp on `nfs_pv` </ul> | [nfs_delete_released_pv_disable_exporter_test.go](./nfs_delete_released_pv_disable_exporter_test.go) |
