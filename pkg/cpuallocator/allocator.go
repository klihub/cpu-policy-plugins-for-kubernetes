// Copyright 2019 Intel Corporation. All Rights Reserved.
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

package cpuallocator

import (
	"fmt"
	"sync"
	"sort"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	stub "k8s.io/kubernetes/pkg/kubelet/cm/cpumanager/stub"

	"github.com/klihub/cpu-policy-plugins-for-kubernetes/pkg/sysfs"
)

// CPU allocation preferences
type AllocFlag uint

const (
	AllocIdlePackages AllocFlag = 1 << iota           // prefer idle packages
	AllocIdleNodes                                    // idle NUMA nodes
	AllocIdleCores                                    // prefer idle cores
	AllocDefault = AllocIdlePackages | AllocIdleCores // default flags

	logPrefix = "[pool-policy/cpuallocator] "
)

// CpuAllocator encapsulates state for allocating CPUs.
type CpuAllocator struct {
	sys     *sysfs.System                // sysfs CPU and topology information
	flags   AllocFlag                    // allocation preferences
	from    cpuset.CPUSet                // set of CPUs to allocate from
	cnt     int                          // number of CPUs to allocate
	result  cpuset.CPUSet                // set of CPUs allocated
	offline cpuset.CPUSet                // set of CPUs currently offline

	pkgs    []sysfs.Package              // physical CPU packages, sorted by preference
	cpus    []sysfs.Cpu	                 // CPU cores, sorted by preference
}

// A singleton wrapper around sysfs.System.
type sysfsSingleton struct {
	sync.Once                            // we do discovery only once
	sys *sysfs.System                    // wrapped sysfs.System instance
	err error                            // error during recovery
	cpusets  struct {                    // cached cpusets per
		pkg  map[sysfs.Id]cpuset.CPUSet  // package, 
		node map[sysfs.Id]cpuset.CPUSet  // node, and
		core map[sysfs.Id]cpuset.CPUSet  // CPU core
	}
}

var system sysfsSingleton

// Types of functions for filtering and sorting packages, nodes, and CPU cores.
type IdFilter func (sysfs.Id) bool
type IdSorter func (int, int) bool

// our logger instance
var log = stub.NewLogger(logPrefix)


// Get/discover sysfs.System.
func (s *sysfsSingleton) get() (*sysfs.System, error) {
	s.Do(func () {
		s.sys, s.err = sysfs.DiscoverSystem(sysfs.DiscoverCpuTopology)
		s.cpusets.pkg  = make(map[sysfs.Id]cpuset.CPUSet)
		s.cpusets.node = make(map[sysfs.Id]cpuset.CPUSet)
		s.cpusets.core = make(map[sysfs.Id]cpuset.CPUSet)
	})

	return s.sys, s.err
}

// Get CPUSet for the given package.
func (s *sysfsSingleton) PackageCPUSet(id sysfs.Id) cpuset.CPUSet {
	if cset, ok := s.cpusets.pkg[id]; ok {
		return cset
	} else {
		cset = s.sys.Package(id).CPUSet()
		s.cpusets.pkg[id] = cset
		return cset
	}
}

// Get CPUSet for the given node.
func (s *sysfsSingleton) NodeCPUSet(id sysfs.Id) cpuset.CPUSet {
	if cset, ok := s.cpusets.node[id]; ok {
		return cset
	} else {
		cset = s.sys.Node(id).CPUSet()
		s.cpusets.node[id] = cset
		return cset
	}
}

// Get CPUSet for the given core.
func (s *sysfsSingleton) CoreCPUSet(id sysfs.Id) cpuset.CPUSet {
	if cset, ok := s.cpusets.core[id]; ok {
		return cset
	} else {
		cset = s.sys.Cpu(id).ThreadCPUSet()
		for _, cid := range cset.ToSlice() {
			s.cpusets.core[sysfs.Id(cid)] = cset
		}
		return cset
	}
}

// Pick packages, nodes or CPUs by filtering according to a function.
func (sys *sysfsSingleton) pick(idSlice []sysfs.Id, f IdFilter) []sysfs.Id {
	ids := make([]sysfs.Id, len(idSlice))

	idx := 0
	for _, id := range idSlice {
		if f == nil || f(id) {
			ids[idx] = id
			idx++
		}
	}

	return ids[0:idx]
}

// Create a new CPU allocator.
func NewCpuAllocator(sys *sysfs.System) *CpuAllocator {
	if sys == nil {
		sys, _ = system.get()
	}
	if sys == nil {
		return nil
	}

	a := &CpuAllocator{
		sys: sys,
		flags: AllocDefault,
	}

	return a
}

// Allocate full idle CPU packages.
func (a *CpuAllocator) takeIdlePackages() {
	offline := a.sys.Offlined()

	// pick idle packages
	pkgs := system.pick(a.sys.PackageIds(),
		func (id sysfs.Id) bool {
			cset := system.PackageCPUSet(id).Difference(offline)
			return cset.Intersection(a.from).Equals(cset)
		})

	// sorted by id
	sort.Slice(pkgs,
		func (i, j int) bool {
			return pkgs[i] < pkgs[j]
		})

	// take as many idle packages as we need/can
	for _, id := range pkgs {
		cset := system.PackageCPUSet(id).Difference(offline)
		if a.cnt >= cset.Size() {
			a.result = a.result.Union(cset)
			a.from = a.from.Difference(cset)
			a.cnt -= cset.Size()

			if a.cnt == 0 {
				break
			}
		}
	}
}

// Allocate full idle CPU cores.
func (a *CpuAllocator) takeIdleCores() {
	offline := a.sys.Offlined()

	// pick (first id for all) idle cores
	cores := system.pick(a.sys.CpuIds(),
		func (id sysfs.Id) bool {
			cset := system.CoreCPUSet(id).Difference(offline)
			return cset.Intersection(a.from).Equals(cset) && cset.ToSlice()[0] == int(id)
		})

	// sorted by id
	sort.Slice(cores,
		func (i, j int) bool {
			return cores[i] < cores[j]
		})

	// take as many idle cores as we can
	for _, id := range cores {
		cset := system.CoreCPUSet(id).Difference(offline)
		if a.cnt >= cset.Size() {
			a.result = a.result.Union(cset)
			a.from = a.from.Difference(cset)
			a.cnt -= cset.Size()

			if a.cnt == 0 {
				break
			}
		}
	}
}

// Allocate idle CPU hyperthreads.
func (a *CpuAllocator) takeIdleThreads() {
	offline := a.sys.Offlined()

	// pick (first id for all) cores with free capacity
	cores := system.pick(a.sys.CpuIds(),
		func (id sysfs.Id) bool {
			cset := system.CoreCPUSet(id).Difference(offline)
			return cset.Intersection(a.from).Size() != 0 && cset.ToSlice()[0] == int(id)
		})
	// sorted for preference by id, mimicking cpus_assignment.go for now:
	//   IOW, prefer CPUs
	//     - from packages with higher number of CPUs/cores already in a.result
	//     - from packages with fewer remaining free CPUs/cores in a.from
	//     - from cores with fewer remaining free CPUs/cores in a.from
	//     - from packages with lower id
	//     - with lower id
	sort.Slice(cores,
		func (i, j int) bool {
			iCore := cores[i]
			jCore := cores[j]
			iPkg := a.sys.Cpu(iCore).PackageId()
			jPkg := a.sys.Cpu(jCore).PackageId()

			// prefer CPUs from packages with
			//   - higher number of CPUs/cores already in a.result, and
			//   - fewer remaining free CPUs/cores in a.from
			iPkgSet := system.PackageCPUSet(iPkg)
			jPkgSet := system.PackageCPUSet(jPkg)
			if iPkgSet.Intersection(a.result).Size() > jPkgSet.Intersection(a.result).Size() {
				return true
			}
			if iPkgSet.Intersection(a.from).Size() < jPkgSet.Intersection(a.from).Size() {
				return true
			}

			// prefer CPUs from cores with fewer remaining CPUs/cores in a.from
			iCoreSet := system.CoreCPUSet(iCore)
			jCoreSet := system.CoreCPUSet(jCore)
			if iCoreSet.Intersection(a.from).Size() < jCoreSet.Intersection(a.from).Size() {
				return true
			}

			// prefer CPUs
			//   from packages with lower id
			//   with lower id
			return iPkg < jPkg || iCore < jCore
		})

	// take as many idle cores as we can
	for _, id := range cores {
		cset := system.CoreCPUSet(id).Difference(offline)
		if a.cnt >= cset.Size() {
			a.result = a.result.Union(cset)
			a.from = a.from.Difference(cset)
			a.cnt -= cset.Size()

			if a.cnt == 0 {
				break
			}
		}
	}
}

// Perform CPU allocation.
func (a *CpuAllocator) allocate() cpuset.CPUSet {
	if (a.flags & AllocIdlePackages) != 0 {
		a.takeIdlePackages()

		if a.cnt == 0 {
			return a.result
		}
	}

	if (a.flags & AllocIdleCores) != 0 {
		a.takeIdleCores()

		if a.cnt == 0 {
			return a.result
		}
	}

	a.takeIdleThreads()
	if a.cnt == 0 {
		return a.result
	}

	return cpuset.NewCPUSet()
}

func allocateCpus(from *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	var result cpuset.CPUSet
	var err error

	switch {
	case from.Size() < cnt:
		result, err = cpuset.NewCPUSet(), fmt.Errorf("cpuset %s does not have %d CPUs", from, cnt)
	case from.Size() == cnt:
		result, err, *from = from.Clone(), nil, cpuset.NewCPUSet()
	default:
		a := NewCpuAllocator(nil)
		a.from = from.Clone()
		a.cnt = cnt

		result, err, *from = a.allocate(), nil, a.from.Clone()
	}

	return result, err
}

// Allocate a number of CPUs from the given set.
func AllocateCpus(from *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	log.Info("* AllocateCpus(CPUs #%s, %d)", from, cnt)

	result, err := allocateCpus(from, cnt)

	if err == nil {
		log.Info("  => took CPUs: #%s, remaining: #%s", result, *from)
	} else {
		log.Info("  => failed: %v", err)
	}

	return result, err
}

// Release a number of CPUs from the given set.
func ReleaseCpus(from *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	log.Info("* ReleaseCpus(CPUs #%s, %d)", from, cnt)

	result, err := allocateCpus(from, from.Size() - cnt)

	if err == nil {
		log.Info("  => kept CPUs: #%s, gave: #%s", from, result)
	} else {
		log.Info("  => failed: %v", err)
	}

	return result, err
}
