// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cluster

import (
	"fmt"

	"go.uber.org/atomic"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/kube/controllers"
	istiolog "istio.io/istio/pkg/log"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
)

type ReconcilerFn func(key types.NamespacedName) error

// Queue defines an abstraction around Kubernetes' workqueue.
// Items enqueued are deduplicated; this generally means relying on ordering of events in the queue is not feasible.
type Queue struct {
	queue       workqueue.TypedRateLimitingInterface[any]
	initialSync *atomic.Bool
	name        string
	workers     int
	maxAttempts int
	workFn      func(key any) error
	log         *istiolog.Scope
}

// WithName sets a name for the queue. This is used for logging
func WithName(name string) func(q *Queue) {
	return func(q *Queue) {
		q.name = name
	}
}

// WithWorkers sets a name for the queue. This is used for logging
func WithWorkers(count int) func(q *Queue) {
	return func(q *Queue) {
		q.workers = count
	}
}

// WithRateLimiter allows defining a custom rate limiter for the queue
func WithRateLimiter(r workqueue.TypedRateLimiter[any]) func(q *Queue) {
	return func(q *Queue) {
		q.queue = workqueue.NewTypedRateLimitingQueue[any](r)
	}
}

// WithMaxAttempts allows defining a custom max attempts for the queue. If not set, items will not be retried
func WithMaxAttempts(n int) func(q *Queue) {
	return func(q *Queue) {
		q.maxAttempts = n
	}
}

// WithReconciler defines the handler function to handle items in the queue.
func WithReconciler(f ReconcilerFn) func(q *Queue) {
	return func(q *Queue) {
		q.workFn = func(key any) error {
			return f(key.(types.NamespacedName))
		}
	}
}

// WithGenericReconciler defines the handler function to handle items in the queue that can handle any type
func WithGenericReconciler(f func(key any) error) func(q *Queue) {
	return func(q *Queue) {
		q.workFn = func(key any) error {
			return f(key)
		}
	}
}

// NewQueue creates a new queue
func NewQueue(name string, options ...func(*Queue)) Queue {
	q := Queue{
		name:        name,
		initialSync: atomic.NewBool(false),
	}
	for _, o := range options {
		o(&q)
	}
	if q.queue == nil {
		q.queue = workqueue.NewTypedRateLimitingQueue[any](workqueue.DefaultTypedControllerRateLimiter[any]())
	}
	q.log = istiolog.WithLabels("controller", q.name)
	return q
}

// Add an item to the queue.
func (q Queue) Add(item any) {
	q.queue.Add(item)
}

// AddObject takes an Object and adds the types.NamespacedName associated.
func (q Queue) AddObject(obj controllers.Object) {
	q.queue.Add(config.NamespacedName(obj))
}

// Run the queue. This is synchronous, so should typically be called in a goroutine.
func (q Queue) Run(stop <-chan struct{}) {
	defer q.queue.ShutDown()
	q.log.Infof("starting")
	q.queue.Add(defaultSyncSignal)
	for range q.workers {
		go func() {
			// Process updates until we return false, which indicates the queue is terminated
			for q.processNextItem() {
			}
		}()
	}
	<-stop
	q.log.Infof("stopped")
}

// syncSignal defines a dummy signal that is enqueued when .Run() is called. This allows us to detect
// when we have processed all items added to the queue prior to Run().
type syncSignal struct{}

// defaultSyncSignal is a singleton instanceof syncSignal.
var defaultSyncSignal = syncSignal{}

// HasSynced returns true if the queue has 'synced'. A synced queue has started running and has
// processed all events that were added prior to Run() being called Warning: these items will be
// processed at least once, but may have failed.
func (q Queue) HasSynced() bool {
	return q.initialSync.Load()
}

// processNextItem is the main workFn loop for the queue
func (q Queue) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := q.queue.Get()
	if quit {
		// We are done, signal to exit the queue
		return false
	}

	// We got the sync signal. This is not a real event, so we exit early after signaling we are synced
	if key == defaultSyncSignal {
		q.log.Debugf("synced")
		q.initialSync.Store(true)
		return true
	}

	q.log.Debugf("handling update: %v", formatKey(key))

	// 'Done marks item as done processing' - should be called at the end of all processing
	defer q.queue.Done(key)

	err := q.workFn(key)
	if err != nil {
		retryCount := q.queue.NumRequeues(key) + 1
		if retryCount < q.maxAttempts {
			q.log.Errorf("error handling %v, retrying (retry count: %d): %v", formatKey(key), retryCount, err)
			q.queue.AddRateLimited(key)
			// Return early, so we do not call Forget(), allowing the rate limiting to backoff
			return true
		}
		q.log.Errorf("error handling %v, and retry budget exceeded: %v", formatKey(key), err)
	}
	// 'Forget indicates that an item is finished being retried.' - should be called whenever we do not want to backoff on this key.
	q.queue.Forget(key)
	return true
}

func formatKey(key any) string {
	if t, ok := key.(types.NamespacedName); ok {
		if len(t.Namespace) > 0 {
			return t.String()
		}
		// because we use namespacedName for non namespace scope resource as well
		return t.Name
	}
	if t, ok := key.(controllers.Event); ok {
		key = t.Latest()
	}
	if t, ok := key.(controllers.Object); ok {
		if len(t.GetNamespace()) > 0 {
			return t.GetNamespace() + "/" + t.GetName()
		}
		return t.GetName()
	}
	res := fmt.Sprint(key)
	if len(res) >= 50 {
		return res[:50]
	}
	return res
}
