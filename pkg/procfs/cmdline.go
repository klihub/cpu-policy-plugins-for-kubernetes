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

package procfs

import (
	"sync"
	"strings"
	"io/ioutil"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	KernelCmdlinePath  = "/proc/cmdline"  // default path to kernel command line
	IsolatedCpusOption = "isolcpus"       // option for isolating CPUs
)

// Kernel command line
type KernelCmdline struct {
	path    string                        // command line path
	cmdline string                        // full command line
	options map[string]string             // a kernel option ('name=value' on command line)
	flags   map[string]struct{}           // a kernel flag ('flag' on command line)
}

// kernel command line singleton, associated parsing error
var once sync.Once
var cmdline *KernelCmdline
var cmdlerr error

// Read and parse the kernel command line from default location.
func GetKernelCmdline() (*KernelCmdline, error) {
	once.Do(func () {
		cmdline, cmdlerr = ParseKernelCmdline(KernelCmdlinePath)
	})

	return cmdline, cmdlerr
}

// Read and parse the kernel command line.
func ParseKernelCmdline(path string) (*KernelCmdline, error) {
	kcl := &KernelCmdline{
		path: path,
	}

	if err := kcl.parse(); err != nil {
		return nil, err
	}

	return kcl, nil
}

// Read the kernel command line.
func (kcl *KernelCmdline) read() error {
	if buf, err := ioutil.ReadFile(kcl.path); err != nil {
		return err
	} else {
		kcl.cmdline = strings.Trim(string(buf), " \n")
		return nil
	}
}

// Parse the kernel command line, reading it if necessary.
func (kcl *KernelCmdline) parse() error {
	if kcl.cmdline == "" {
		if err := kcl.read(); err != nil {
			return err
		}
	}

	if kcl.options == nil {
		kcl.options = make(map[string]string)
	}
	if kcl.flags == nil {
		kcl.flags = make(map[string]struct{})
	}

	for _, opt := range strings.Split(kcl.cmdline, " ") {
		if opt = strings.Trim(opt, " \t"); opt == "" {
			continue
		}
		if kv := strings.SplitN(opt, "=", 2); len(kv) == 2 {
			kcl.options[kv[0]] = kv[1]
		} else {
			kcl.flags[kv[0]] = struct{}{}
		}
	}

	return nil
}

// Get the value of the given kernel command line option.
func (kcl *KernelCmdline) OptionValue(key string) (string, bool) {
	key, ok := kcl.options[key]
	return key, ok
}

// Get the given kernel command line flag.
func (kcl *KernelCmdline) Flag(key string) bool {
	_, present := kcl.flags[key]
	return present
}

// Get the kernel-isolated CPUs as a cpuset.
func (kcl *KernelCmdline) IsolatedCPUSet() (cpuset.CPUSet, error) {
	cpus, ok := kcl.OptionValue(IsolatedCpusOption)
	if !ok {
		return cpuset.NewCPUSet(), nil
	}
	
	return cpuset.Parse(cpus)
}

// Get the kernel-isolated CPUs as a slice of CPU ids.
func (kcl *KernelCmdline) IsolatedCpus() ([]int, error) {
	cset, err := kcl.IsolatedCPUSet()
	if err != nil {
		return []int{}, err
	}

	return cset.ToSlice(), nil
}

