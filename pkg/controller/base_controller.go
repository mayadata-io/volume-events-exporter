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
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

const (
	// OpenEBSCASLabelKey holds value of OpenEBS CAS label key
	// This key is helpful to identify type of the volume
	OpenEBSCASLabelKey = "openebs.io/cas-type"
)

type controller struct {
	// name of the controller
	name string

	// numWorker number of worker for controller
	numWorker int

	// workqueue for controller
	workQueue workqueue.RateLimitingInterface

	// init function to be executed before reconcile
	init func() error

	// reconcile is main function, which process the event
	reconcile func(key string) (bool, error)

	// reconcilePeriod represent interval at which reconciliation will be executed, default value is 1s
	reconcilePeriod time.Duration

	// syncPeriod represent interval at which sync function will be executed
	syncPeriod time.Duration

	// sync function, which is executed at interval of syncPeriod
	sync func()

	cacheSyncWaiters []cache.InformerSynced
}

func newController(name string, numWorker int) *controller {
	return &controller{
		name:            name,
		workQueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), name),
		numWorker:       numWorker,
		reconcilePeriod: time.Second,
	}
}

func (c *controller) Run(ctx context.Context) error {
	// check if controller implemented reconcile function or not
	if c.reconcile == nil {
		return errors.Errorf("no reconcile function provided")
	}

	if c.sync != nil && c.syncPeriod == 0 {
		return errors.Errorf("sync period not set")
	}

	// if controller has init function then execute it
	if c.init != nil {
		err := c.init()
		if err != nil {
			return errors.Wrapf(err, "init got failed")
		}
	}

	klog.Infof("Starting %s controller", c.name)
	defer klog.Infof("Shutting down %s controller", c.name)

	// wait for caches to sync
	if len(c.cacheSyncWaiters) > 0 {
		klog.Info("Waiting for caches to sync")
		if !cache.WaitForCacheSync(ctx.Done(), c.cacheSyncWaiters...) {
			return errors.New("timed out waiting for caches to sync")
		}
		klog.Info("All caches synced")
	}

	// Waitgroup for starting controller goroutines.
	var wg sync.WaitGroup

	defer func() {
		klog.Info("Waiting for all workers to shutdown")

		c.workQueue.ShutDown()

		// wait for all the go routines
		wg.Wait()

		klog.Info("All workers are down")
	}()

	wg.Add(c.numWorker)
	for i := 0; i < c.numWorker; i++ {
		go func() {
			wait.Until(c.runWorker, c.reconcilePeriod, ctx.Done())
			wg.Done()
		}()
	}

	if c.sync != nil {
		wg.Add(1)
		go func() {
			wait.Until(c.sync, c.syncPeriod, ctx.Done())
			wg.Done()
		}()
	}

	<-ctx.Done()
	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *controller) processNextWorkItem() bool {
	obj, shutdown := c.workQueue.Get()

	if shutdown {
		return false
	}

	// always call Done on this key so the workqueue knows we have finished
	// processing this item. If any error occurs we re-add this key to workqueue
	// with rate-limiting.
	// if we don't want to process this key further then call Forget for this key
	defer c.workQueue.Done(obj)

	key, ok := obj.(string)
	if !ok {
		c.workQueue.Forget(key)
	}

	shouldRequeue, err := c.reconcile(key)
	if err == nil {
		if shouldRequeue {
			c.workQueue.Add(obj)
			return true
		}
		c.workQueue.Forget(key)
		return true
	}

	klog.Errorf("Failed to handle key %s error: %s", key, err.Error())
	c.workQueue.AddRateLimited(key)
	return true
}

func (c *controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		klog.Errorf("Error while fetching namespace & name information from object %T error: %s", obj, err.Error())
		return
	}

	c.workQueue.Add(key)
}
