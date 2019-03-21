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

package sysfs

import (
	"fmt"
	"strconv"
	"path/filepath"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	stub "k8s.io/kubernetes/pkg/kubelet/cm/cpumanager/stub"
)

const (
	SysfsPath       = "/sys"                       // default sysfs mount point
	systemCpuPath   = "devices/system/cpu"         // sysfs system cpu subdir
	systemNodePath  = "devices/system/node"        // sysfs system node subdir
	logPrefix       = "[sysfs] "
)

type DiscoveryFlag uint

const (
	DiscoverCpuTopology DiscoveryFlag = 1 << iota  // discover CPU topology
	DiscoverMemTopology                            // discover memory (NUMA) topology
	DiscoverCache                                  // discover CPU cache details
	DiscoverNone        DiscoveryFlag = 0          // zero flag
	DiscoverAll         DiscoveryFlag = 0xffffffff // full system discovery
	DiscoverDefault     DiscoveryFlag = DiscoverCpuTopology
)

// System devices
type System struct {
	flags    DiscoveryFlag                // system discovery flags
	path     string                       // sysfs mount point
	packages map[Id]*Package              // physical packages
	nodes    map[Id]*Node                 // NUMA nodes
	cpus     map[Id]*Cpu                  // CPUs
	cache    map[Id]*Cache                // Cache
	offline  IdSet                        // offlined CPUs
}

// A physical package (a collection of CPUs).
type Package struct {
	id    Id                              // package id
	cpus  IdSet                           // CPUs in this package
	nodes IdSet                           // nodes in this package
}

// A NUMA node.
type Node struct {
	path     string                       // sysfs path
	id       Id                           // node id
	cpus     IdSet                        // cpus in this node
	distance []int                        // distance/cost to other NUMA nodes
	memory   IdSet                        // memory in this node
}

// A CPU (core).
type Cpu struct {
	path    string                        // sysfs path
	id      Id                            // CPU id
	pkg     Id                            // package id
	node    Id                            // node id
	threads IdSet                         // sibling/hyper-threads
	freq    CpuFreq                       // CPU frequencies
	online  bool                          // whether this CPU is online
}

// CPU frequencu range
type CpuFreq struct {
	min uint64                            // minimum frequency (kHz)
	max uint64                            // maximum frequency (kHz)
	all []uint64                          // discrete set of frequencies if applicable/known
}

// CPU cache.
//   Notes: cache-discovery is forced off now (by forcibly clearing the related discovery bit)
//      Can't seem to make sense of the cache information exposed under sysfs. The cache ids
//      do not seem to be unique, which IIUC is contrary to the documentation.

type CacheType string

const (
	DataCache CacheType = "Data"                    // data cache
	InstructionCache CacheType = "Instruction"      // instruction cache
	UnifiedCache CacheType = "Unified"              // unified cache
)

type Cache struct {
	id    Id                              // cache id
	kind  CacheType                       // cache type
	size  uint64                          // cache size
	level uint8                           // cache level
	cpus  IdSet	                          // CPUs sharing this cache
}


// our logger instance
var log = stub.NewLogger(logPrefix)

// Discover running system details.
func DiscoverSystem(args ...DiscoveryFlag) (*System, error) {
	var flags DiscoveryFlag

	if len(args) < 1 {
		flags = DiscoverDefault
	} else {
		flags = DiscoverNone
		for _, flag := range args {
			flags |= flag
		}
	}

	sys := &System{
		path: SysfsPath,
		offline: NewIdSet(),
	}

	if err := sys.Discover(flags); err != nil {
		return nil, err
	}

	return sys, nil
}

// Perform system/hardware discovery.
func (sys *System) Discover(flags DiscoveryFlag) error {
	sys.flags |= (flags &^ DiscoverCache)

	if (sys.flags & (DiscoverCpuTopology | DiscoverCache)) != 0 {
		if err := sys.discoverCpus(); err != nil {
			return err
		}
		if err := sys.discoverNodes(); err != nil {
			return err
		}
		if err := sys.discoverPackages(); err != nil {
			return err
		}
	}

	if (sys.flags & DiscoverMemTopology) != 0 {
		if err := sys.discoverNodes(); err != nil {
			return err
		}
	}

	return nil
}

// Change the online state of a set of CPUs. Return the set that was changed. Nil set implies all CPUs.
func (sys *System) SetCpusOnline(online bool, cpus IdSet) (IdSet, error) {
	var entries []string

	if cpus == nil {
		entries, _ = filepath.Glob(filepath.Join(sys.path, systemCpuPath, "cpu[0-9]*"))
	} else {
		entries = make([]string, cpus.Size())
		for idx, id := range cpus.Members() {
			entries[idx] = sys.path + "/" + systemCpuPath + "/cpu" + strconv.Itoa(int(id))
		}
	}

	desired := map[bool]int{false: 0, true: 1}[online]
	changed := NewIdSet()

	for _, entry := range entries {
		var current int

		id := getEnumeratedId(entry)
		if id <= 0 {
			continue
		}

		if _, err := writeSysfsEntry(entry, "online", desired, &current); err != nil {
			return nil, sysfsError(entry, "failed to set online to %d: %v", desired, err)
		}

		if desired != current {
			changed.Add(id)
			if cpu, found := sys.cpus[id]; found {
				cpu.online = online
				
				if online {
					sys.offline.Del(id)
				} else {
					sys.offline.Add(id)
				}
			}
		}
	}

	return changed, nil
}

// Set CPU frequency scaling limits for the given CPUs. Nil set implies all CPUs.
func (sys *System) SetCpuFrequencyLimits(min, max uint64, cpus IdSet) error {
	if cpus == nil {
		cpus = NewIdSet(sys.CpuIds()...)
	}

	for _, id := range cpus.Members() {
		if cpu, ok := sys.cpus[id]; ok {
			if err := cpu.SetFrequencyLimits(min, max); err != nil {
				return err
			}
		}
	}

	return nil
}

// Get the ids of all packages present in the system.
func (sys *System) PackageIds() []Id {
	ids := make([]Id, len(sys.packages))
	idx := 0
	for id, _ := range sys.packages {
		ids[idx] = id
		idx++
	}
	return ids
}

// Get the ids of all NUMA nodes present in the system.
func (sys *System) NodeIds() []Id {
	ids := make([]Id, len(sys.nodes))
	idx := 0
	for id, _ := range sys.nodes {
		ids[idx] = id
		idx++
	}
	return ids
}

// Get the ids of all CPUs present in the system.
func (sys *System) CpuIds() []Id {
	ids := make([]Id, len(sys.cpus))
	idx := 0
	for id, _ := range sys.cpus {
		ids[idx] = id
		idx++
	}
	return ids
}

// Get the ids of all CPUs present in the system as a CPUSet.
func (sys *System) CPUSet() cpuset.CPUSet {
	return NewIdSet(sys.CpuIds()...).CPUSet()
}

// Get the package with a given package id.
func (sys *System) Package(id Id) *Package {
	return sys.packages[id]
}

// Get the node with a given node id.
func (sys *System) Node(id Id) *Node {
	return sys.nodes[id]
}

// Get the CPU with a given CPU id.
func (sys *System) Cpu(id Id) *Cpu {
	return sys.cpus[id]
}

// Get the set of offlined CPUs.
func (sys *System) Offlined() cpuset.CPUSet {
	return sys.offline.CPUSet()
}

// Dump System as a string.
func (sys *System) String() string {
	dump := ""
	for id, pkg := range sys.packages {
		dump += fmt.Sprintf("package #%d:\n", id)
		dump += fmt.Sprintf("   cpus: %s\n", pkg.cpus)
		dump += fmt.Sprintf("  nodes: %s\n", pkg.nodes)
	}

	for id, node := range sys.nodes {
		dump += fmt.Sprintf("node #%d:\n", id)
		dump += fmt.Sprintf("      cpus: %s\n", node.cpus)
		dump += fmt.Sprintf("    memory: %s\n", node.memory)
		dump += fmt.Sprintf("  distance: %v\n", node.distance)
	}

	for id, cpu := range sys.cpus {
		dump += fmt.Sprintf("CPU #%d:\n", id)
		dump += fmt.Sprintf("      pkg: %d\n", cpu.pkg)
		dump += fmt.Sprintf("     node: %d\n", cpu.node)
		dump += fmt.Sprintf("  threads: %s\n", cpu.threads)
		dump += fmt.Sprintf("     freq: %d - %d\n", cpu.freq.min, cpu.freq.max)
	}

	dump += fmt.Sprintf("offline CPUs: %s", sys.offline)

	for id, cch := range sys.cache {
		dump += fmt.Sprintf("cache #%d:\n",  id)
		dump += fmt.Sprintf("   type: %v\n", cch.kind)
		dump += fmt.Sprintf("   size: %d\n", cch.size)
		dump += fmt.Sprintf("  level: %d\n", cch.level)
		dump += fmt.Sprintf("   CPUs: %s\n", cch.cpus)
	}

	return dump
}

// Dump System as a string using logging.
func (sys *System) DumpLog() {
	for id, pkg := range sys.packages {
		log.Info("package #%d:", id)
		log.Info("   cpus: %s", pkg.cpus)
		log.Info("  nodes: %s", pkg.nodes)
	}

	for id, node := range sys.nodes {
		log.Info("node #%d:", id)
		log.Info("      cpus: %s", node.cpus)
		log.Info("    memory: %s", node.memory)
		log.Info("  distance: %v", node.distance)
	}

	for id, cpu := range sys.cpus {
		log.Info("CPU #%d:", id)
		log.Info("      pkg: %d", cpu.pkg)
		log.Info("     node: %d", cpu.node)
		log.Info("  threads: %s", cpu.threads)
		log.Info("     freq: %d - %d", cpu.freq.min, cpu.freq.max)
	}

	log.Info("offline CPUs: %s", sys.offline)

	for id, cch := range sys.cache {
		log.Info("cache #%d:",  id)
		log.Info("   type: %v", cch.kind)
		log.Info("   size: %d", cch.size)
		log.Info("  level: %d", cch.level)
		log.Info("   CPUs: %s", cch.cpus)
	}
}

// Discover CPUs present in the system.
func (sys *System) discoverCpus() error {
	if sys.cpus != nil {
		return nil
	}

	sys.cpus = make(map[Id]*Cpu)

	if offline, err := sys.SetCpusOnline(true, nil); err != nil {
		return fmt.Errorf("failed to set CPUs online: %v", err)
	} else {
		defer sys.SetCpusOnline(false, offline)
	}

	entries, _ := filepath.Glob(filepath.Join(sys.path, systemCpuPath, "cpu[0-9]*"))
	for _, entry := range entries {
		if err := sys.discoverCpu(entry); err != nil {
			return fmt.Errorf("failed to discover cpu for entry %s: %v", entry, err)
		}
	}

	return nil
}

// Discover details of the given CPU.
func (sys *System) discoverCpu(path string) error {
	cpu := &Cpu{path: path, id: getEnumeratedId(path), online: true }

	if _, err := readSysfsEntry(path, "topology/physical_package_id", &cpu.pkg); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "topology/thread_siblings_list", &cpu.threads, ","); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "cpufreq/cpuinfo_min_freq", &cpu.freq.min); err != nil {
		cpu.freq.min = 0
	}
	if _, err := readSysfsEntry(path, "cpufreq/cpuinfo_max_freq", &cpu.freq.max); err != nil {
		cpu.freq.max = 0
	}
	if node, _ := filepath.Glob(filepath.Join(path, "node[0-9]*")); len(node) == 1 {
		cpu.node = getEnumeratedId(node[0])
	}

	sys.cpus[cpu.id] = cpu

	if (sys.flags & DiscoverCache) != 0 {
		entries, _ := filepath.Glob(filepath.Join(path, "cache/index[0-9]*"))
		for _, entry := range entries {
			if err := sys.discoverCache(entry); err != nil {
				return err
			}
		}
	}

	return nil
}

// Return id of this CPU.
func (c *Cpu) Id() Id {
	return c.id
}

// Return package id of this CPU.
func (c *Cpu) PackageId() Id {
	return c.pkg
}

// Return node id of this CPU.
func (c *Cpu) NodeId() Id {
	return c.node
}

// Return the CPUSet for all threads in this core.
func (c *Cpu) ThreadCPUSet() cpuset.CPUSet {
	return c.threads.CPUSet()
}

// Return frequency range for this CPU.
func (c *Cpu) FrequencyRange() CpuFreq {
	return c.freq
}

// Return if this CPU is online.
func (c *Cpu) Online() bool {
	return c.online
}

// Set frequency range limits for this CPU.
func (c *Cpu) SetFrequencyLimits(min, max uint64) error {
	if c.freq.min == 0 {
		return nil
	}

	min /= 1000
	max /= 1000
	if min < c.freq.min && min != 0 {
		min = c.freq.min
	}
	if min > c.freq.max {
		min = c.freq.max
	}
	if max < c.freq.min && max != 0 {
		max = c.freq.min
	}
	if max > c.freq.max {
		max = c.freq.max
	}

	if _, err := writeSysfsEntry(c.path, "cpufreq/scaling_min_freq", min, nil); err != nil {
		return err
	}
	if _, err := writeSysfsEntry(c.path, "cpufreq/scaling_max_freq", max, nil); err != nil {
		return err
	}

	return nil
}

// Discover NUMA nodes present in the system.
func (sys *System) discoverNodes() error {
	if sys.nodes != nil {
		return nil
	}

	sys.nodes = make(map[Id]*Node)

	entries, _ := filepath.Glob(filepath.Join(sys.path, systemNodePath, "node[0-9]*"))
	for _, entry := range entries {
		if err := sys.discoverNode(entry); err != nil {
			return fmt.Errorf("failed to discover node for entry %s: %v", entry, err)
		}
	}

	return nil
}

// Discover details of the given NUMA node.
func (sys *System) discoverNode(path string) error {
	node := &Node{path: path, id: getEnumeratedId(path)}

	if _, err := readSysfsEntry(path, "cpulist", &node.cpus, ","); err != nil {
		return err
	}
	if _, err := readSysfsEntry(path, "distance", &node.distance); err != nil {
		return err
	}

	if (sys.flags & DiscoverMemTopology) != 0 {
		node.memory = NewIdSet()

		entries, _ := filepath.Glob(filepath.Join(path, "memory[0-9]*"))
		for _, entry := range entries {
			node.memory.Add(getEnumeratedId(entry))
		}
	}

	sys.nodes[node.id] = node

	return nil
}

// Return id of this node.
func (n *Node) Id() Id {
	return n.id
}

// Return the CPUSet for all cores/threads in this node.
func (n *Node) CPUSet() cpuset.CPUSet {
	return n.cpus.CPUSet()
}

// Return the distance vector for this node.
func (n *Node) Distance() []int {
	return n.distance
}

// Return the distance of this and a given node.
func (n *Node) DistanceFrom(id Id) int {
	if int(id) < len(n.distance) {
		return n.distance[int(id)]
	} else {
		return -1
	}
}

// Discover physical packages (CPU sockets) present in the system.
func (sys *System) discoverPackages() error {
	if sys.packages != nil {
		return nil
	}

	sys.packages = make(map[Id]*Package)

	for _, cpu := range sys.cpus {
		pkg, found := sys.packages[cpu.pkg]
		if !found {
			pkg = &Package{
				id:    cpu.pkg,
				cpus:  NewIdSet(),
				nodes: NewIdSet(),
			}
			sys.packages[cpu.pkg] = pkg
		}
		pkg.cpus.Add(cpu.id)
		pkg.nodes.Add(cpu.node)
	}

	return nil
}

// Return the id of this package.
func (p *Package) Id() Id {
	return p.id
}

// Return the CPUSet for all cores/threads in this package.
func (p *Package) CPUSet() cpuset.CPUSet {
	return p.cpus.CPUSet()
}


// Discover cache associated with the given CPU.
// Notes:
//     I'm not sure how to interpret the cache information under sysfs. This code is now effectively
//     disabled by forcing the associated discovery bit off in the discovery flags.
func (sys *System) discoverCache(path string) error {
	var id Id

	if _, err := readSysfsEntry(path, "id", &id); err != nil {
		return sysfsError(path, "can't read cache id: %v", err)
	}

	if sys.cache == nil {
		sys.cache = make(map[Id]*Cache)
	}

	if _, found := sys.cache[id]; found {
		return nil
	}

	c := &Cache{ id: id }

	if _, err := readSysfsEntry(path, "level", &c.level); err != nil {
		return sysfsError(path, "can't read cache level: %v", err)
	}
	if _, err := readSysfsEntry(path, "shared_cpu_list", &c.cpus, ","); err != nil {
		return sysfsError(path, "can't read shared CPUs: %v", err)
	}
	kind := ""
	if _, err := readSysfsEntry(path, "type", &kind); err != nil {
		return sysfsError(path, "can't read cache type: %v", err)
	}
	switch kind {
	case "Data":        c.kind = DataCache
	case "Instruction": c.kind = InstructionCache
	case "Unified":     c.kind = UnifiedCache
	default:
		return sysfsError(path, "unknown cache type: %s", kind)
	}

	size := ""
	if _, err := readSysfsEntry(path, "size", &size); err != nil {
		return sysfsError(path, "can't read cache size: %v", err)
	}

	base := size[0:len(size)-1]
	suff := size[len(size)-1]
	unit := map[byte]uint64{ 'K': 1 << 10,'M': 1 << 20, 'G': 1 << 30 }

	if val, err := strconv.ParseUint(base, 10, 0); err != nil {
		return sysfsError(path, "can't parse cache size '%s': %v", size, err)
	} else {
		if u, ok := unit[suff]; ok {
			c.size = val * u
		} else {
			c.size = val * 1000 + u - '0'
		}
	}

	sys.cache[c.id] = c

	return nil
}
