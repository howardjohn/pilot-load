package simulation

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"istio.io/istio/pkg/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/simulation/util"
)

type PodStartupSimulation struct {
	Config model.StartupConfig
}

func (a *PodStartupSimulation) GetConfig() any {
	return a.Config
}

func (a *PodStartupSimulation) createPod() (*v1.Pod, error) {
	p := &v1.Pod{}
	if err := yaml.Unmarshal([]byte(a.Config.Spec), p); err != nil {
		return nil, err
	}
	p.Namespace = a.Config.Namespace
	p.Name = "startup-" + util.GenUID()
	if p.Labels == nil {
		p.Labels = map[string]string{}
	}
	p.Labels["sidecar.istio.io/inject"] = fmt.Sprint(a.Config.Inject)
	return p, nil
}

type result struct {
	podName string
	// Apply -> Accessible in API server (typically ~instant)
	read time.Duration
	// Apply -> Init container started
	initStart time.Duration
	// Apply -> Init container ended
	initEnd time.Duration
	// Apply -> main container started
	start time.Duration
	// Apply -> pod ready (from status). There may be large delays in reporting readiness to the pod
	// status, so statusReady can be a lot lower than ready.
	statusReady time.Duration
	// Apply -> pod ready (observed)
	ready time.Duration
}

const cleanupDelay = time.Second * 0

func (a *PodStartupSimulation) runWorker(ctx model.Context, report chan result) {
	work := func() (res result) {
		pod, err := a.createPod()
		if err != nil {
			log.Fatal(err)
		}
		t0 := time.Now()
		if err := kube.Apply(ctx.Client, pod); err != nil {
			log.Warnf("pod creation failed: %v", err)
			return
		}
		defer func() {
			if cleanupDelay > 0 {
				go func() {
					time.Sleep(cleanupDelay)
					if err := kube.Delete(ctx.Client, pod); err != nil {
						log.Warnf("pod cleanup: %v", err)
					}
				}()
			} else {
				if err := kube.Delete(ctx.Client, pod); err != nil {
					log.Warnf("pod cleanup: %v", err)
				}
			}
		}()
		for {
			log.Debugf("lookup %v", pod.Name)
			kpod, err := ctx.Client.Kube().CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
			if err != nil {
				// We got a real error, exit
				if !errors.IsNotFound(err) {
					if strings.Contains(err.Error(), "context canceled") {
						return
					}
					log.Warnf("pod lookup failed: %v", err)
					return
				}
				// Try again
				continue
			}
			if res.read == 0 {
				res.read = time.Since(t0)
				res.podName = kpod.Name
			}
			// TODO: why do we base this on k8s reported time but readiness based on our observed time?
			start, end := GetInitContainerTimes(kpod, "istio-init")
			if !start.IsZero() {
				res.initStart = start.Sub(t0)
			}
			if !end.IsZero() {
				res.initEnd = end.Sub(t0)
			}
			cStart := GetContainerTimes(kpod, "app")
			if !cStart.IsZero() {
				res.start = cStart.Sub(t0)
			}
			statusReady := GetPodReadyTime(kpod)
			if res.statusReady == 0 && !statusReady.IsZero() {
				res.statusReady = statusReady.Sub(t0)
			}
			if IsPodReady(kpod) {
				res.ready = time.Since(t0)
				return
			}
			time.Sleep(time.Millisecond * 50)
		}
	}
	for {
		if util.IsDone(ctx) {
			return
		}
		res := work()
		select {
		case <-ctx.Done():
			return
		case report <- res:
		}
		time.Sleep(a.Config.Cooldown)
	}
}

func (a *PodStartupSimulation) Run(ctx model.Context) error {
	c := make(chan result)
	wg := sync.WaitGroup{}
	for i := 0; i < a.Config.Concurrency; i++ {
		wg.Add(1)
		go func() {
			a.runWorker(ctx, c)
			wg.Done()
		}()
		time.Sleep(a.Config.Cooldown)
	}

	results := []result{}
	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			fmt.Printf("Avg:\tscheduled:%-9v\tinit:%-9v\tready:%-9v\tfull ready:%-9v\tcomplete:%-9v\n",
				avg(results, func(r result) time.Duration { return r.initStart }),
				avg(results, func(r result) time.Duration { return r.start - r.initStart }),
				avg(results, func(r result) time.Duration { return r.statusReady - r.start }),
				avg(results, func(r result) time.Duration { return r.ready - r.start }),
				avg(results, func(r result) time.Duration { return r.ready }),
			)
			fmt.Printf("Max:\tscheduled:%-9v\tinit:%-9v\tready:%-9v\tfull ready:%-9v\tcomplete:%-9v\n",
				max(results, func(r result) time.Duration { return r.initStart }),
				max(results, func(r result) time.Duration { return r.start - r.initStart }),
				max(results, func(r result) time.Duration { return r.statusReady - r.start }),
				max(results, func(r result) time.Duration { return r.ready - r.start }),
				max(results, func(r result) time.Duration { return r.ready }),
			)
			fmt.Println(`
scheduled: time until first container starts
init: time between first container start and init container completion
ready: time until kubelet said it marked it Ready after main container started
full ready: time until it was actually observed as Ready after main container started
complete: time until it was actually observed as Ready\n`)
			wg.Wait()
			return nil
		case report := <-c:
			results = append(results, report)
			log.Infof(
				"Report:\tscheduled:%-9v\tinit:%-9v\tready:%-9v\tfull ready:%-9v\tcomplete:%-9v\tname:%-9v",
				report.initStart.Truncate(time.Millisecond),
				(report.start - report.initStart).Truncate(time.Millisecond),
				report.statusReady.Truncate(time.Millisecond),
				(report.ready - report.start).Truncate(time.Millisecond),
				report.ready.Truncate(time.Millisecond),
				report.podName,
			)
		}
	}
}

func (a *PodStartupSimulation) Cleanup(ctx model.Context) error {
	return nil
}

var _ model.Simulation = &PodStartupSimulation{}

// copy from kubernetes/pkg/api/v1/pod/utils.go
func IsPodReady(pod *v1.Pod) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

// IsPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func IsPodReadyConditionTrue(status v1.PodStatus) bool {
	condition := GetPodReadyCondition(status)
	return condition != nil && condition.Status == v1.ConditionTrue
}

func GetPodReadyCondition(status v1.PodStatus) *v1.PodCondition {
	_, condition := GetPodCondition(&status, v1.PodReady)
	return condition
}

func GetPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []v1.PodCondition, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

func GetInitContainerTimes(pod *v1.Pod, container string) (start time.Time, end time.Time) {
	if pod == nil {
		return
	}
	for _, c := range pod.Status.InitContainerStatuses {
		if c.Name != container {
			continue
		}
		if c.State.Terminated != nil {
			return c.State.Terminated.StartedAt.Time, c.State.Terminated.FinishedAt.Time
		}
	}
	return
}

func GetPodReadyTime(pod *v1.Pod) time.Time {
	c := GetPodReadyCondition(pod.Status)
	if c == nil {
		return time.Time{}
	}
	if c.Status == v1.ConditionTrue {
		return c.LastTransitionTime.Time
	}
	return time.Time{}
}

func GetContainerTimes(pod *v1.Pod, container string) (start time.Time) {
	if pod == nil {
		return
	}
	for _, c := range pod.Status.ContainerStatuses {
		if c.Name != container {
			continue
		}
		if c.State.Running != nil {
			return c.State.Running.StartedAt.Time
		}
	}
	return
}

func avg(res []result, f func(result) time.Duration) time.Duration {
	if len(res) == 0 {
		return 0
	}
	s := time.Duration(0)
	for _, t := range res {
		s += f(t)
	}
	return (s / time.Duration(len(res))).Truncate(time.Millisecond)
}

func max(res []result, f func(result) time.Duration) time.Duration {
	s := time.Duration(0)
	for _, t := range res {
		g := f(t)
		if g > s {
			s = g
		}
	}
	return s.Truncate(time.Millisecond)
}
