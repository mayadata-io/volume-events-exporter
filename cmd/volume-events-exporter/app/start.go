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

package app

import (
	"context"
	"flag"
	"os"
	"strconv"
	"sync"
	"time"

	pvcontroller "github.com/mayadata-io/volume-events-exporter/pkg/controller/pvcontroller"
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
	// sharedInformerInterval is used to sync watcher controller.
	sharedInformerInterval = 30 * time.Second
	leaderElectionLockName = "volume-events-exporter"
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

// Start will start volume metrics collector controller
func Start() error {
	klog.InitFlags(nil)
	err := flag.Set("logtostderr", "true")
	if err != nil {
		return errors.Wrap(err, "failed to set logtostderr flag")
	}
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
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, getSyncInterval())
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	pController := pvcontroller.NewPVMetricsController(pvcontroller.PVMetricsControllerConfig{
		KubeClientset: kubeClient,
		PVInformer:    pvInformer,
		PVCInformer:   pvcInformer,
	})

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	run := func(context.Context) {
		var wg sync.WaitGroup
		wg.Add(3)

		// Start PV informer
		go func() {
			pvInformer.Informer().Run(stopCh)
			wg.Done()
		}()

		// Start PVC informer
		go func() {
			pvcInformer.Informer().Run(stopCh)
			wg.Done()
		}()

		// Start PV controller to send volume event information
		go func() {
			pController.Run(NumThreads, stopCh)
			wg.Done()
		}()

		wg.Wait()
	}

	if !*leaderElection {
		run(context.TODO())
	} else {
		le := leader.NewLeaderElection(kubeClient, leaderElectionLockName, run)
		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}
		if err := le.Run(); err != nil {
			klog.Fatalf("failed to initialize leader election: %v", err)
		}
	}
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

// getSyncInterval gets the resync interval from environment variable.
// If missing or zero then default to SharedInformerInterval otherwise
// return the obtained value
func getSyncInterval() time.Duration {
	resyncInterval, err := strconv.Atoi(os.Getenv("RESYNC_INTERVAL"))
	if err != nil || resyncInterval == 0 {
		klog.Warningf("Incorrect resync interval %q obtained from env, defaulting to %q seconds", resyncInterval, sharedInformerInterval)
		return sharedInformerInterval
	}
	return time.Duration(resyncInterval) * time.Second
}
