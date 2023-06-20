/*
Copyright 2023 The Kubernetes Authors.

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

package openstack

import (
	"github.com/gophercloud/gophercloud"
	cloudprovider "k8s.io/cloud-provider"
)

// InstancesV2 encapsulates an implementation of InstancesV2 for OpenStack.
type InstancesV2 struct {
	compute          *gophercloud.ServiceClient
	network          *gophercloud.ServiceClient
	region           string
	regionProviderID bool
	networkingOpts   NetworkingOpts
}

// InstancesV2 returns an implementation of InstancesV2 for OpenStack.
func (os *OpenStack) InstancesV2() (cloudprovider.InstancesV2, bool) {
	// PF9: Not supported, using older Rackspace OSPC
	/*if !os.useV1Instances {
		return os.instancesv2()
	}
	*/
	return nil, false
}
