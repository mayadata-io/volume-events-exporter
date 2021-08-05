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

package cmd

import (
	"context"
	"flag"
	"sync"

	"github.com/mayadata-io/volume-events-exporter/pkg/controller"
	"github.com/mayadata-io/volume-events-exporter/pkg/signals"
	leader "github.com/openebs/api/v2/pkg/kubernetes/leaderelection"
	"github.com/pkg/errors"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	leaderElectionLockName = "volume-events-exporter"

	// volumeEventControllerWorkers states no.of Go routines to
	// process volume events
	volumeEventControllerWorkers = 1
)

// Command line flags
var (
	kubeconfig              = flag.String("kubeconfig", "", "Path for kube config")
	leaderElection          = flag.Bool("leader-election", false, "Enables leader election.")
	leaderElectionNamespace = flag.String("leader-election-namespace", "", "The namespace where the leader election resource exists. Defaults to the pod namespace if not set.")
)

const (
	// NumThreads defines number of worker threads for resource watcher.
	NumThreads = 1
	// NumRoutinesThatFollow is for handling golang waitgroups.
	NumRoutinesThatFollow = 1
)

// StartVolumeEventsController will start volume events collector controller
func StartVolumeEventsController() error {
	klog.InitFlags(nil)
	flag.Parse()

	cfg, err := getClusterConfig(*kubeconfig)
	if err != nil {
		return errors.Wrap(err, "error building kubeconfig")
	}

	// Building Kubernetes Clientset
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "error building kubernetes clientset")
	}

	// NewSharedInformerFactory constructs a new instance of k8s sharedInformerFactory.
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, controller.GetSyncInterval())
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pController := controller.NewPVEventController(kubeClient, pvInformer, pvcInformer, volumeEventControllerWorkers)

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()
	var wg sync.WaitGroup

	run := func(ctx context.Context) {

		// Start registered informers
		kubeInformerFactory.Start(stopCh)

		wg.Add(1)
		// Start PV controller to send volume event information
		go func() {
			_ = pController.Run(ctx)
			wg.Done()
		}()
	}

	if !*leaderElection {
		// Create a context which can be cancled
		ctx, cancelFn := context.WithCancel(context.TODO())
		run(ctx)

		// When process receives shutdown signal trigger run cancel func
		<-stopCh
		cancelFn()
	} else {
		le := leader.NewLeaderElection(kubeClient, leaderElectionLockName, run)
		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}
		if err := le.Run(); err != nil {
			return errors.Wrapf(err, "failed to initialize leader election")
		}
	}

	wg.Wait()
	return nil
}

// getClusterConfig return the config for k8s.
func getClusterConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	klog.V(4).Info("Kubeconfig flag is empty... fetching incluster config")
	return rest.InClusterConfig()
}
