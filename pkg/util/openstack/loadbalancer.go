/*
Copyright 2019 The Kubernetes Authors.
Copyright 2021 Rackspace US, Inc.

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
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/apiversions"
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/listeners"
	loadbalancerv2 "github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/pools"
	"github.com/gophercloud/gophercloud/pagination"
	version "github.com/hashicorp/go-version"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	cpoerrors "github.com/os-pc/cloud-provider-rackspace/pkg/util/errors"

	"github.com/os-pc/gocloudlb/loadbalancers"
)

const (
	OctaviaFeatureTags   = 0
	OctaviaFeatureVIPACL = 1

	loadbalancerActiveInitDelay = 1 * time.Second
	loadbalancerActiveFactor    = 1.2
	loadbalancerActiveSteps     = 19

	activeStatus = "ACTIVE"
	errorStatus  = "ERROR"
)

var (
	octaviaVersion string

	// ErrNotFound is used to inform that the object is missing
	ErrNotFound = errors.New("failed to find object")

	// ErrMultipleResults is used when we unexpectedly get back multiple results
	ErrMultipleResults = errors.New("multiple results where only one expected")
)

// getOctaviaVersion returns the current Octavia API version.
func getOctaviaVersion(client *gophercloud.ServiceClient) (string, error) {
	if octaviaVersion != "" {
		return octaviaVersion, nil
	}

	defaultVer := "0.0"
	allPages, err := apiversions.List(client).AllPages()
	if err != nil {
		return defaultVer, err
	}
	versions, err := apiversions.ExtractAPIVersions(allPages)
	if err != nil {
		return defaultVer, err
	}
	if len(versions) == 0 {
		return defaultVer, fmt.Errorf("API versions for Octavia not found")
	}

	klog.V(4).Infof("Found Octavia API versions: %v", versions)

	// The current version is always the last one in the list
	octaviaVersion = versions[len(versions)-1].ID
	klog.V(4).Infof("The current Octavia API version: %v", octaviaVersion)

	return octaviaVersion, nil
}

// IsOctaviaFeatureSupported returns true if the given feature is supported in the deployed Octavia version.
func IsOctaviaFeatureSupported(client *gophercloud.ServiceClient, feature int) bool {
	octaviaVer, err := getOctaviaVersion(client)
	if err != nil {
		klog.Warningf("Failed to get current Octavia API version: %v", err)
		return false
	}

	currentVer, _ := version.NewVersion(octaviaVer)

	switch feature {
	case OctaviaFeatureVIPACL:
		verACL, _ := version.NewVersion("v2.12")
		if currentVer.GreaterThanOrEqual(verACL) {
			return true
		}
	case OctaviaFeatureTags:
		verTags, _ := version.NewVersion("v2.5")
		if currentVer.GreaterThanOrEqual(verTags) {
			return true
		}
	default:
		klog.Warningf("Feature %d not recognized", feature)
	}

	return false
}

func waitLoadbalancerActive(client *gophercloud.ServiceClient, loadbalancerID uint64) error {
	backoff := wait.Backoff{
		Duration: loadbalancerActiveInitDelay,
		Factor:   loadbalancerActiveFactor,
		Steps:    loadbalancerActiveSteps,
	}

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		loadbalancer, err := loadbalancers.Get(client, loadbalancerID).Extract()
		if err != nil {
			return false, err
		}
		if loadbalancer.Status == activeStatus {
			return true, nil
		} else if loadbalancer.Status == errorStatus {
			return true, fmt.Errorf("loadbalancer has gone into ERROR state")
		} else {
			return false, nil
		}
	})

	return err
}

// UpdateListener updates a listener and wait for the lb active
func UpdateListener(client *gophercloud.ServiceClient, lbID uint64, listenerID string, opts listeners.UpdateOpts) error {
	if _, err := listeners.Update(client, listenerID, opts).Extract(); err != nil {
		return err
	}

	if err := waitLoadbalancerActive(client, lbID); err != nil {
		return fmt.Errorf("failed to wait for load balancer ACTIVE after updating listener: %v", err)
	}

	return nil
}

// CreateListener creates a new listener
func CreateListener(client *gophercloud.ServiceClient, lbID uint64, opts listeners.CreateOpts) (*listeners.Listener, error) {
	listener, err := listeners.Create(client, opts).Extract()
	if err != nil {
		return nil, err
	}

	if err := waitLoadbalancerActive(client, lbID); err != nil {
		return nil, fmt.Errorf("failed to wait for load balancer ACTIVE after creating listener: %v", err)
	}

	return listener, nil
}

// GetLoadbalancerByName get the load balancer which is in valid status by the given name.
func GetLoadBalancersByName(client *gophercloud.ServiceClient, name string) ([]loadbalancers.LoadBalancer, error) {
	var allLoadbalancers, validLBs []loadbalancers.LoadBalancer
	var listOpts loadbalancerv2.ListOpts
	var oldMarker string

	// Query all the LBs by using the markers.
	// Set last LB ID as marker.
	for {
		var tempLbs []loadbalancers.LoadBalancer
		var err error

		err = loadbalancers.List(client, listOpts).EachPage(func(LbsInCurrentPage pagination.Page) (bool, error) {
			tempLbs, err = loadbalancers.ExtractLoadBalancers(LbsInCurrentPage)
			if err != nil {
				return false, err
			}

			return false, nil
		})
		if err != nil {
			return nil, err
		}

		if len(tempLbs) == 0 {
			break
		} else {
			marker := strconv.FormatUint(tempLbs[len(tempLbs)-1].ID, 10)
			if marker == oldMarker {
				break
			}

			// On the first loop iteration, add all the LBs. Alternatively, if the oldMarker differs
			// from the first LB ID in the newly retrieved LBs, there's no need to remove it.
			// However, if the oldMarker matches the first LB ID in the newly retrieved LBs, it can be removed.
			// Determine the starting index for appending load balancers.
			startIndex := 0
			if oldMarker != "" && oldMarker == strconv.FormatUint(tempLbs[0].ID, 10) {
				startIndex = 1
			}

			// Append the load balancers from the determined start index
			allLoadbalancers = append(allLoadbalancers, tempLbs[startIndex:]...)

			// Set the oldMarker with new marker & also update the marker with new marker for querying.
			oldMarker = marker
			listOpts.Marker = marker
		}
	}

	for _, lb := range allLoadbalancers {
		// All the Status could be found here https://docs.rackspace.com/docs/cloud-load-balancers/v1/api-reference/load-balancers
		if lb.Name == name && (lb.Status != "DELETED" && lb.Status != "PENDING_DELETE") {
			validLBs = append(validLBs, lb)
		}
	}

	return validLBs, nil
}

// GetLoadBalancerByPort gets the specific load balancer from a list of load balancers
func GetLoadBalancerByPort(lbs []loadbalancers.LoadBalancer, port corev1.ServicePort) (*loadbalancers.LoadBalancer, error) {
	for _, lb := range lbs {
		if lb.Port == port.Port {
			return &lb, nil
		}
	}

	return nil, ErrNotFound
}

// GetListenerByName gets a listener by its name, raise error if not found or get multiple ones.
func GetListenerByName(client *gophercloud.ServiceClient, name string, lbID string) (*listeners.Listener, error) {
	opts := listeners.ListOpts{
		Name:           name,
		LoadbalancerID: lbID,
	}
	pager := listeners.List(client, opts)
	var listenerList []listeners.Listener

	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		v, err := listeners.ExtractListeners(page)
		if err != nil {
			return false, err
		}
		listenerList = append(listenerList, v...)
		if len(listenerList) > 1 {
			return false, ErrMultipleResults
		}
		return true, nil
	})
	if err != nil {
		if cpoerrors.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(listenerList) == 0 {
		return nil, ErrNotFound
	}

	return &listenerList[0], nil
}

// GetPoolByName gets a pool by its name, raise error if not found or get multiple ones.
func GetPoolByName(client *gophercloud.ServiceClient, name string, lbID string) (*pools.Pool, error) {
	var listenerPools []pools.Pool

	opts := pools.ListOpts{
		Name:           name,
		LoadbalancerID: lbID,
	}
	err := pools.List(client, opts).EachPage(func(page pagination.Page) (bool, error) {
		v, err := pools.ExtractPools(page)
		if err != nil {
			return false, err
		}
		listenerPools = append(listenerPools, v...)
		if len(listenerPools) > 1 {
			return false, ErrMultipleResults
		}
		return true, nil
	})
	if err != nil {
		if cpoerrors.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if len(listenerPools) == 0 {
		return nil, ErrNotFound
	} else if len(listenerPools) > 1 {
		return nil, ErrMultipleResults
	}

	return &listenerPools[0], nil
}

// DeleteLoadbalancer deletes a loadbalancer with all its child objects.
func DeleteLoadbalancer(client *gophercloud.ServiceClient, lbID uint64) error {
	err := loadbalancers.Delete(client, lbID).ExtractErr()
	if err != nil && !cpoerrors.IsNotFound(err) {
		return fmt.Errorf("error deleting loadbalancer %d: %v", lbID, err)
	}

	return nil
}
