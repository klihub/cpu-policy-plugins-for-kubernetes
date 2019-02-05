/*
Copyright 2018 The Kubernetes Authors.

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

package metrics

import (
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpumanager/stub"

	types "github.com/klihub/cpu-policy-plugins-for-kubernetes/pkg/apis/cpupools.intel.com/v1draft1"
	poolapi "github.com/klihub/cpu-policy-plugins-for-kubernetes/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	logPrefix = "[cpu-policy/metrics] " // log message prefix
)

type Metrics struct {
	nodeName  string             // node name
	clientset *poolapi.Clientset // clientset for REST API
	namespace string             // namespace
	metric    *types.Metric      // pending metrics to be published
}

// our logger instance
var log = stub.NewLogger(logPrefix)

// Create an interface for publishing pool metrics data as a CRD.
func NewMetrics(nodeName, namespace string, cs *poolapi.Clientset) *Metrics {
	return &Metrics{
		nodeName:  nodeName,
		clientset: cs,
		namespace: namespace,
	}
}

func (m *Metrics) UpdatePool(name string, shared, exclusive cpuset.CPUSet, capacity, usage int64) error {
	log.Info("pool %s: updating metrics to <shared=%s, pinned=%s, capacity=%d, usage=%d>",
		name, shared, exclusive, capacity, usage)

	// first see if the Metric object is already present
	metric, err := m.clientset.CpupoolsV1draft1().Metrics(m.namespace).Get(m.nodeName, metav1.GetOptions{})

	if err != nil {
		metric = &types.Metric{
			ObjectMeta: metav1.ObjectMeta{Name: m.nodeName},
			Spec:       types.MetricSpec{Pools: []types.Pool{}},
		}
		metric, err = m.clientset.CpupoolsV1draft1().Metrics(m.namespace).Create(metric)
		if err != nil {
			log.Error("failed to create metrics CRD: %s", err.Error())
			return err
		}
	}

	updated := []types.Pool{}
	pool := &types.Pool{
		PoolName: name,
		Exclusive: exclusive.String(),
		Shared: shared.String(),
		Capacity: capacity,
		Usage: usage,
	}
	for _, p := range metric.Spec.Pools {
		if p.PoolName != name {
			updated = append(updated, p)
		} else {
			updated = append(updated, *pool);
			pool = nil
		}
	}
	if pool != nil {
		updated = append(updated, *pool)
	}
	metric.Spec.Pools = updated

	m.metric = metric

	return nil
}

// Published current gathered metrics for pools.
func (m *Metrics) Publish() error {
	log.Info("publishing pool metrics")

	if _, err := m.clientset.CpupoolsV1draft1().Metrics(m.namespace).Update(m.metric); err != nil {
		log.Error("failed to publish pool metrics: %s", err.Error())
		return err
	}

	m.metric = nil

	return nil
}

