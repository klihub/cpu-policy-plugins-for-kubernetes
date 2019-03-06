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
	"sort"
	"strconv"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	Unknown Id = -1
)

// An integer id, used to identify packages, CPUs, nodes, etc.
type Id int

// An unordered set of integer ids.
type IdSet map[Id]struct{}

// Create a new unordered set of (integer) ids.
func NewIdSet(ids ...Id) IdSet {
	s := make(map[Id]struct{})

	for _, id := range ids {
		s[id] = struct{}{}
	}

	return s
}

// Create a new unordered set from an integer slice.
func NewIdSetFromIntSlice(ids ...int) IdSet {
	s := make(map[Id]struct{})

	for _, id := range ids {
		s[Id(id)] = struct{}{}
	}

	return s
}

// Add an id to the set, return whether it was already present.
func (s IdSet) Add(id Id) bool {
	_, present := s[id]
	if !present {
		s[id] = struct{}{}
	}
	return present
}

// Delete an id from the set, return whether it was present.
func (s IdSet) Del(id Id) bool {
	if s == nil {
		return false
	}
	_, present := s[id]
	if present {
		delete(s, id)
	}
	return present
}

// Return the number of ids in the set.
func (s IdSet) Size() int {
	return len(s)
}

// Test if an id is present in a set.
func (s IdSet) Has(id Id) bool {
	if s == nil {
		return false
	}
	_, present := s[id]
	return present
}

// Return all ids in the set as a randomly ordered slice.
func (s IdSet) Members() []Id {
	if s == nil {
		return []Id{}
	}
	ids := make([]Id, len(s))
	idx := 0
	for id := range s {
		ids[idx] = id
		idx++
	}
	return ids
}

// Return all ids in the set as a sorted slice.
func (s IdSet) SortedMembers() []Id {
	ids := s.Members()
	sort.Slice(ids, func (i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

// Return a cpuset.CPUSet corresponding to an id set.
func (s IdSet) CPUSet() cpuset.CPUSet {
	b := cpuset.NewBuilder()
	for id, _ := range s {
		b.Add(int(id))
	}
	return b.Result()
}

// Return an id set corresponding to a cpuset.CPUSet.
func FromCPUSet(cset cpuset.CPUSet) IdSet {
	return NewIdSetFromIntSlice(cset.ToSlice()...)
}

// Return the set as a string.
func (s IdSet) String() string {
	return s.StringWithSeparator(" ")
}

// Return the set as a string, separated with the given separator.
func (s IdSet) StringWithSeparator(args ...string) string {
	if s == nil || len(s) == 0 {
		return ""
	}

	var sep string

	if len(args) == 1 {
		sep = args[0]
	} else {
		sep = ","
	}

	str := ""
	t := ""
	for _, id := range s.SortedMembers() {
		str = str + t + strconv.Itoa(int(id))
		t = sep
	}

	return str
}

