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
	"sort"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpumanager/stub"

	types "github.com/klihub/cpu-policy-plugins-for-kubernetes/pkg/apis/cpupools.intel.com/v1draft1"
	poolapi "github.com/klihub/cpu-policy-plugins-for-kubernetes/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	logPrefix = "[cpu-policy/metrics] " // log message prefix
)

// Cached per pool metrics data.
type pool struct {
	name      string                    // pool name
	shared    cpuset.CPUSet             // shared subset of pool CPUs
	exclusive cpuset.CPUSet             // exclusively allocated CPUs
	capacity  int64                     // total pool CPU capacity
	usage     int64                     // pool CPU capacity in use
}

// Metrics client and pool data.
type Metrics struct {
	clientset *poolapi.Clientset        // clientset for talking to REST API
	nodename  string                    // our node name
	namespace string                    // metrics CRD namespace
	metric    *types.Metric             // metrics CRD on the server
	pools     map[string]*pool          // cached pool metrics data
}

// our logger instance
var log = stub.NewLogger(logPrefix)

// Create an object for publishing pool metrics data as a CRD.
func NewMetrics(nodename, namespace string, cs *poolapi.Clientset) *Metrics {
	m := &Metrics{
		clientset: cs,
		nodename:  nodename,
		namespace: namespace,
		pools:     make(map[string]*pool),
	}

	if err := m.syncOrCreate(); err != nil {
		log.Error("failed to fetch or create metrics CRD: %s", err.Error())
	}

	return m
}

// Update metrics for the given pool.
func (m *Metrics) UpdatePool(name string, shared, exclusive cpuset.CPUSet, capacity, usage int64) error {
	m.pools[name] = &pool{
		name:      name,
		shared:    shared.Clone(),
		exclusive: exclusive.Clone(),
		capacity:  capacity,
		usage:     usage,
	}

	log.Info("updated pool %s: <shared=%s, pinned=%s, capacity=%d, usage=%d>",
		name, shared, exclusive, capacity, usage)

	return nil
}

// Delete metrics for the given pool.
func (m *Metrics) DeletePool(name string) bool {
	_, found := m.pools[name]
	if found {
		delete(m.pools, name)
		log.Info("deleted pool %s", name)
	}

	return found
}

// Delete metrics for all pools.
func (m *Metrics) DeleteAllPools() {
	m.pools = make(map[string]*pool)
}

// Publish current gathered metrics for pools.
func (m *Metrics) Publish() error {
	return m.update()
}

// Fetch metrics CRD from server if it exists, otherwise create CRD on the server.
func (m *Metrics) syncOrCreate() error {
	if m.fetch() == nil {
		return nil
	}

	return m.create()
}

// Create metrics CRD on the server.
func (m *Metrics) create() error {
	metric, err := m.clientset.
		CpupoolsV1draft1().
		Metrics(m.namespace).
		Create(
		&types.Metric{
			ObjectMeta: metav1.ObjectMeta{Name: m.nodename},
			Spec:       types.MetricSpec{Pools: []types.Pool{}},
		})

	if err != nil {
		return err
	}

	m.metric = metric
	log.Info("created pool metrics CRD")

	return nil
}

// Fetch metrics CRD from the server.
func (m *Metrics) fetch() error {
	metric, err := m.clientset.
		CpupoolsV1draft1().
		Metrics(m.namespace).
		Get(m.nodename, metav1.GetOptions{})

	if err != nil {
		return err
	}

	m.metric = metric

	for _, p := range metric.Spec.Pools {
		m.pools[p.PoolName] = &pool{
			name:      p.PoolName,
			shared:    cpuset.MustParse(p.Shared),
			exclusive: cpuset.MustParse(p.Exclusive),
			capacity:  p.Capacity,
			usage:     p.Usage,
		}

		log.Info("fetched pool %s metrics: <shared=%s, pinned=%s, capacity=%d, usage=%d>",
			p.PoolName, p.Shared, p.Exclusive, p.Capacity, p.Usage)
	}

	return nil
}

// Update metrics CRD on the server from cached pool metrics.
func (m *Metrics) update() error {
	var idx int

	pools := make([]string, len(m.pools))
	idx = 0
	for name, _ := range m.pools {
		pools[idx] = name
		idx++
	}
	sort.Strings(pools)

	updates := make([]types.Pool, len(pools))
	idx = 0
	for _, pool := range pools {
		updates[idx] = types.Pool{
			PoolName:  m.pools[pool].name,
			Shared:    m.pools[pool].shared.String(),
			Exclusive: m.pools[pool].exclusive.String(),
			Capacity:  m.pools[pool].capacity,
			Usage:     m.pools[pool].usage,
		}
		idx++
	}
	m.metric.Spec.Pools = updates

	metric, err := m.clientset.CpupoolsV1draft1().
		Metrics(m.namespace).
		Update(m.metric)

	if err != nil {
		log.Error("failed to publish pool metrics: %s", err.Error())
		return err
	}

	log.Info("published/updated pool metrics CRD")
	m.metric = metric
	return nil
}

