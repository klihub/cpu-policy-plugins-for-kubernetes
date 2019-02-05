/*
Copyright The Kubernetes Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package versioned

import (
	cpupoolsv1draft1 "github.com/klihub/cpu-policy-plugins-for-kubernetes/pkg/client/clientset/versioned/typed/cpupools.intel.com/v1draft1"
	discovery "k8s.io/client-go/discovery"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	CpupoolsV1draft1() cpupoolsv1draft1.CpupoolsV1draft1Interface
	// Deprecated: please explicitly pick a version if possible.
	Cpupools() cpupoolsv1draft1.CpupoolsV1draft1Interface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*discovery.DiscoveryClient
	cpupoolsV1draft1 *cpupoolsv1draft1.CpupoolsV1draft1Client
}

// CpupoolsV1draft1 retrieves the CpupoolsV1draft1Client
func (c *Clientset) CpupoolsV1draft1() cpupoolsv1draft1.CpupoolsV1draft1Interface {
	return c.cpupoolsV1draft1
}

// Deprecated: Cpupools retrieves the default version of CpupoolsClient.
// Please explicitly pick a version.
func (c *Clientset) Cpupools() cpupoolsv1draft1.CpupoolsV1draft1Interface {
	return c.cpupoolsV1draft1
}

// Discovery retrieves the DiscoveryClient
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

// NewForConfig creates a new Clientset for the given config.
func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}
	var cs Clientset
	var err error
	cs.cpupoolsV1draft1, err = cpupoolsv1draft1.NewForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfig(&configShallowCopy)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a new Clientset for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	var cs Clientset
	cs.cpupoolsV1draft1 = cpupoolsv1draft1.NewForConfigOrDie(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClientForConfigOrDie(c)
	return &cs
}

// New creates a new Clientset for the given RESTClient.
func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.cpupoolsV1draft1 = cpupoolsv1draft1.New(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClient(c)
	return &cs
}
