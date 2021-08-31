# Integration Test

Volume-event-exporter makes use of ginkgo & gomega libraries to implement its integration tests.

To run the integrations test:
1. Copy kubeconfig file into path ~/.kube/config or set the KUBECONFIG env.
2. Run `make sanity-test`

**Note**:
- Integration test will spin up RPC server and will tear down server at end of the test. By default server will run on 9090 port and it can configured with option `--port`.
- To run a specific test following command can be used:
  ```sh
  ginkgo -v -focus="TEST NFS PVC CREATE & DELTE EVENTS" -- -address=172.18.0.1 -port=9091
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
| Create NFS PVC with reclaim policy Delete | <ul> Verify whether volume_event_controller sent volume_provisioned JSON data to configured server <li> nfs_pvc </li> <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pv </li> along with creationTimestamp on `nfs_pv` </ul>| [nfs_sanity_test.go](./nfs_sanity_test.go) |
| Delete NFS PVC with reclaim policy Delete | <ul> Verify whether volume_event_controller sent volume_deprovisioned JSON data to configured server <li> nfs_pv </li> <li> backend_pvc </li> <li> backend_pvc </li> along with creation and deletion timestamp on `nfs_pv` </ul> | [nfs_sanity_test.go](./nfs_sanity_test.go) |
