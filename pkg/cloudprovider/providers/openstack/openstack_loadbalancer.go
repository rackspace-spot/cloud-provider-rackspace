/*
Copyright 2016 The Kubernetes Authors.
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
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions"
	neutronports "github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/pagination"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	v1service "github.com/os-pc/cloud-provider-rackspace/pkg/api/v1/service"
	cpoerrors "github.com/os-pc/cloud-provider-rackspace/pkg/util/errors"
	openstackutil "github.com/os-pc/cloud-provider-rackspace/pkg/util/openstack"

	"github.com/os-pc/gocloudlb/accesslists"
	"github.com/os-pc/gocloudlb/loadbalancers"
	lbnodes "github.com/os-pc/gocloudlb/nodes"
	"github.com/os-pc/gocloudlb/virtualips"
)

// Note: when creating a new Loadbalancer (VM), it can take some time before it is ready for use,
// this timeout is used for waiting until the Loadbalancer provisioning status goes to ACTIVE state.
const (
	// loadbalancerActive* is configuration of exponential backoff for
	// going into ACTIVE loadbalancer provisioning status. Starting with 1
	// seconds, multiplying by 1.2 with each step and taking 19 steps at maximum
	// it will time out after 128s, which roughly corresponds to 120s
	loadbalancerActiveInitDelay = 1 * time.Second
	loadbalancerActiveFactor    = 1.2
	loadbalancerActiveSteps     = 19

	// loadbalancerDelete* is configuration of exponential backoff for
	// waiting for delete operation to complete. Starting with 1
	// seconds, multiplying by 1.2 with each step and taking 13 steps at maximum
	// it will time out after 32s, which roughly corresponds to 30s
	loadbalancerDeleteInitDelay = 1 * time.Second
	loadbalancerDeleteFactor    = 1.2
	loadbalancerDeleteSteps     = 13

	activeStatus  = "ACTIVE"
	errorStatus   = "ERROR"
	deletedStatus = "DELETED"

	// ServiceAnnotationLoadBalancerInternal defines whether or not to create an internal loadbalancer. Default: false.
	ServiceAnnotationLoadBalancerInternal             = "service.beta.kubernetes.io/openstack-internal-load-balancer"
	ServiceAnnotationLoadBalancerConnLimit            = "loadbalancer.openstack.org/connection-limit"
	ServiceAnnotationLoadBalancerProxyEnabled         = "loadbalancer.openstack.org/proxy-protocol"
	ServiceAnnotationLoadBalancerTimeoutClientData    = "loadbalancer.openstack.org/timeout-client-data"
	ServiceAnnotationLoadBalancerTimeoutMemberConnect = "loadbalancer.openstack.org/timeout-member-connect"
	ServiceAnnotationLoadBalancerTimeoutMemberData    = "loadbalancer.openstack.org/timeout-member-data"
	ServiceAnnotationLoadBalancerTimeoutTCPInspect    = "loadbalancer.openstack.org/timeout-tcp-inspect"
	ServiceAnnotationLoadBalancerXForwardedFor        = "loadbalancer.openstack.org/x-forwarded-for"
	// ServiceAnnotationLoadBalancerEnableHealthMonitor defines whether or not to create health monitor for the load balancer
	// pool, if not specified, use 'create-monitor' config. The health monitor can be created or deleted dynamically.
	ServiceAnnotationLoadBalancerEnableHealthMonitor = "loadbalancer.openstack.org/enable-health-monitor"

	DefaultBatch = 10
)

// CloudLb is a LoadBalancer implementation for Rackspace Cloud LoadBalancer API
type CloudLb struct {
	LoadBalancer
}

func networkExtensions(client *gophercloud.ServiceClient) (map[string]bool, error) {
	seen := make(map[string]bool)

	pager := extensions.List(client)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		exts, err := extensions.ExtractExtensions(page)
		if err != nil {
			return false, err
		}
		for _, ext := range exts {
			seen[ext.Alias] = true
		}
		return true, nil
	})

	return seen, err
}

func getNodesByLBID(client *gophercloud.ServiceClient, id uint64) ([]lbnodes.Node, error) {
	var nodes []lbnodes.Node
	err := lbnodes.List(client, id, lbnodes.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		nodesList, err := lbnodes.ExtractNodes(page)
		if err != nil {
			return false, err
		}
		nodes = append(nodes, nodesList...)

		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

// Check if a node exists
func nodeExists(nodes []lbnodes.Node, addr string, port int) bool {
	for _, node := range nodes {
		if node.Address == addr && node.Port == int32(port) {
			return true
		}
	}

	return false
}

func popNode(nodes []lbnodes.Node, addr string, port int) []lbnodes.Node {
	for i, node := range nodes {
		if node.Address == addr && node.Port == int32(port) {
			nodes[i] = nodes[len(nodes)-1]
			nodes = nodes[:len(nodes)-1]
		}
	}

	return nodes
}

func waitLoadbalancerActiveStatus(client *gophercloud.ServiceClient, loadbalancerID uint64) (string, error) {
	backoff := wait.Backoff{
		Duration: loadbalancerActiveInitDelay,
		Factor:   loadbalancerActiveFactor,
		Steps:    loadbalancerActiveSteps,
	}

	var provisioningStatus string
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		loadbalancer, err := loadbalancers.Get(client, loadbalancerID).Extract()
		if err != nil && cpoerrors.IsPendingUpdate(err) {
			klog.V(6).Infof("LoadBalancer %d: Waiting for status %s but got HTTP %v",
				loadbalancerID, activeStatus, err)
			return false, nil
		} else if err != nil {
			return false, err
		}

		klog.V(6).Infof("LoadBalancer %d: Waiting for status %s, currently %s",
			loadbalancerID, activeStatus, loadbalancer.Status)
		provisioningStatus = loadbalancer.Status
		if loadbalancer.Status == activeStatus {
			return true, nil
		} else if loadbalancer.Status == errorStatus {
			return true, fmt.Errorf("loadbalancer has gone into ERROR state")
		} else {
			return false, nil
		}
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("loadbalancer failed to go into ACTIVE provisioning status within allotted time")
	}
	return provisioningStatus, err
}

func waitLoadbalancerDeleted(client *gophercloud.ServiceClient, loadbalancerID uint64) error {
	backoff := wait.Backoff{
		Duration: loadbalancerDeleteInitDelay,
		Factor:   loadbalancerDeleteFactor,
		Steps:    loadbalancerDeleteSteps,
	}
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		loadbalancer, err := loadbalancers.Get(client, loadbalancerID).Extract()
		if err != nil {
			if cpoerrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		if loadbalancer.Status == deletedStatus {
			return true, nil
		}
		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("loadbalancer failed to delete within the allotted time")
	}

	return err
}

func toLBProtocol(protocol corev1.Protocol) string {
	switch protocol {
	case corev1.ProtocolTCP:
		return "TCP_CLIENT_FIRST"
	default:
		return string(protocol)
	}
}

func createLBCreateVips(service *corev1.Service) ([]virtualips.CreateOpts, error) {
	internalAnnotation := false
	var lbType string
	switch internalAnnotation {
	case false:
		lbType = "PUBLIC"
	case true:
		lbType = "SERVICENET"
	}

	var ret []virtualips.CreateOpts
	ret = append(ret, virtualips.CreateOpts{Type: lbType})

	return ret, nil
}

func cloneLBCreateVips(lb *loadbalancers.LoadBalancer) []virtualips.CreateOpts {
	var ret []virtualips.CreateOpts

	for _, vip := range lb.VirtualIps {
		ret = append(ret, virtualips.CreateOpts{ID: vip.ID})
	}

	return ret
}

func (lbaas *CloudLb) createLoadBalancer(name string, vips []virtualips.CreateOpts, port corev1.ServicePort, keepClientIP bool, accessLists []accesslists.CreateOpts) (*loadbalancers.LoadBalancer, error) {
	createOpts := loadbalancers.CreateOpts{
		Name:       name,
		Protocol:   toLBProtocol(port.Protocol),
		Port:       port.Port,
		VirtualIps: vips,
		Nodes:      []lbnodes.CreateOpts{},
		AccessList: accessLists,
	}

	if keepClientIP {
		klog.V(4).Infof("Forcing to use 'HTTP' protocol for listener because %s annotation is set", ServiceAnnotationLoadBalancerXForwardedFor)
		createOpts.Protocol = "HTTP"
	}

	loadbalancer, err := loadbalancers.Create(lbaas.lb, createOpts).Extract()
	if err != nil {
		return nil, fmt.Errorf("error creating loadbalancer %v: %v", createOpts, err)
	}
	return loadbalancer, nil
}

func (lbaas *CloudLb) deleteLoadBalancers(lbs []loadbalancers.LoadBalancer) error {
	klog.V(2).Infof("total available number of loadbalancers %d: ", len(lbs))
	for _, loadbalancer := range lbs {
		// delete loadbalancer
		klog.V(4).Infof("Deleting load balancer %d: ", loadbalancer.ID)
		err := loadbalancers.Delete(lbaas.lb, loadbalancer.ID).ExtractErr()
		if err != nil && !cpoerrors.IsNotFound(err) {
			return err
		}
		err = waitLoadbalancerDeleted(lbaas.lb, loadbalancer.ID)
		if err != nil {
			return fmt.Errorf("failed to delete loadbalancer: %v", err)
		}
	}

	return nil
}

// GetLoadBalancer returns whether the specified load balancer exists and its status
func (lbaas *CloudLb) GetLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service) (*corev1.LoadBalancerStatus, bool, error) {
	// no ports, why are we here?
	if len(service.Spec.Ports) == 0 {
		return nil, false, nil
	}

	name := lbaas.GetLoadBalancerName(ctx, clusterName, service)
	lbs, err := openstackutil.GetLoadBalancersByName(lbaas.lb, name)
	if err != nil {
		return nil, false, err
	}

	loadbalancer, err := openstackutil.GetLoadBalancerByPort(lbs, service.Spec.Ports[0])
	if err == openstackutil.ErrNotFound {
		return nil, false, nil
	}
	if loadbalancer == nil {
		return nil, false, err
	}

	status := &corev1.LoadBalancerStatus{}

	if len(loadbalancer.VirtualIps) > 0 {
		status.Ingress = []corev1.LoadBalancerIngress{{IP: loadbalancer.VirtualIps[0].Address}}
	}

	return status, true, nil
}

// GetLoadBalancerName returns the constructed load balancer name.
func (lbaas *CloudLb) GetLoadBalancerName(ctx context.Context, clusterName string, service *corev1.Service) string {
	name := fmt.Sprintf("kube_%s_%s_%s", clusterName, service.Namespace, service.Name)
	return cutString(name)
}

// cutString makes sure the string length doesn't exceed 128, which is the name maximum in Rackspace Cloud Load Balancer API.
func cutString(original string) string {
	ret := original
	if len(original) > 128 {
		ret = original[:128]
	}
	return ret
}

// remove a matching item from the supplied list
func popLB(lbs []loadbalancers.LoadBalancer, rm *loadbalancers.LoadBalancer) []loadbalancers.LoadBalancer {
	// find the load balancer in the list
	for i, lb := range lbs {
		if lb.ID == rm.ID {
			// put the one on the end over the top of this one
			lbs[i] = lbs[len(lbs)-1]
			// shrink the slice
			lbs = lbs[:len(lbs)-1]
			break
		}
	}

	return lbs
}

// The LB needs to be configured with instance addresses on the same
// subnet as the LB (aka opts.SubnetID).  The correct address for
// Rackspace is going to be the node's InternalIP since this is
// the ServiceNet address.
func nodeAddressForLB(node *corev1.Node) (string, error) {
	addrs := node.Status.Addresses
	if len(addrs) == 0 {
		return "", ErrNoAddressFound
	}

	for _, addr := range addrs {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}

	return "", ErrNoAddressFound
}

// getStringFromServiceAnnotation searches a given v1.Service for a specific annotationKey and either returns the annotation's value or a specified defaultSetting
func getStringFromServiceAnnotation(service *corev1.Service, annotationKey string, defaultSetting string) string {
	klog.V(4).Infof("getStringFromServiceAnnotation(%v, %v, %v)", service, annotationKey, defaultSetting)
	if annotationValue, ok := service.Annotations[annotationKey]; ok {
		// if there is an annotation for this setting, set the "setting" var to it
		// annotationValue can be empty, it is working as designed
		// it makes possible for instance provisioning loadbalancer without floatingip
		klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, annotationValue)
		return annotationValue
	}
	// if there is no annotation, set "settings" var to the value from cloud config
	klog.V(4).Infof("Could not find a Service Annotation; falling back on cloud-config setting: %v = %v", annotationKey, defaultSetting)
	return defaultSetting
}

func getIntFromServiceAnnotation(service *corev1.Service, annotationKey string) (int, bool) {
	intString := getStringFromServiceAnnotation(service, annotationKey, "")
	if len(intString) > 0 {
		annotationValue, err := strconv.Atoi(intString)
		if err == nil {
			klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, annotationValue)
			return annotationValue, true
		}
	}
	return 0, false
}

// getBoolFromServiceAnnotation searches a given v1.Service for a specific annotationKey and either returns the annotation's value or a specified defaultSetting
func getBoolFromServiceAnnotation(service *corev1.Service, annotationKey string, defaultSetting bool) (bool, error) {
	klog.V(4).Infof("getBoolFromServiceAnnotation(%v, %v, %v)", service, annotationKey, defaultSetting)
	if annotationValue, ok := service.Annotations[annotationKey]; ok {
		returnValue := false
		switch annotationValue {
		case "true":
			returnValue = true
		case "false":
			returnValue = false
		default:
			return returnValue, fmt.Errorf("unknown %s annotation: %v, specify \"true\" or \"false\" ", annotationKey, annotationValue)
		}

		klog.V(4).Infof("Found a Service Annotation: %v = %v", annotationKey, returnValue)
		return returnValue, nil
	}
	klog.V(4).Infof("Could not find a Service Annotation; falling back to default setting: %v = %v", annotationKey, defaultSetting)
	return defaultSetting, nil
}

// getPorts gets all the filtered ports.
func getPorts(network *gophercloud.ServiceClient, listOpts neutronports.ListOpts) ([]neutronports.Port, error) {
	allPages, err := neutronports.List(network, listOpts).AllPages()
	if err != nil {
		return []neutronports.Port{}, err
	}
	allPorts, err := neutronports.ExtractPorts(allPages)
	if err != nil {
		return []neutronports.Port{}, err
	}

	return allPorts, nil
}

func (lbaas *CloudLb) ensureLoadBalancerNodes(lbID uint64, port corev1.ServicePort, nodes []*corev1.Node) error {
	memberNodes, err := getNodesByLBID(lbaas.lb, lbID)
	if err != nil && !cpoerrors.IsNotFound(err) {
		return fmt.Errorf("error getting load balancer nodes %d: %v", lbID, err)
	}
	var addNodes []lbnodes.CreateOpts
	for _, node := range nodes {
		addr, err := nodeAddressForLB(node)
		if err != nil {
			if err == ErrNotFound {
				// Node failure, do not create member
				klog.Warningf("Failed to add node %s to LB %d: %v", node.Name, lbID, err)
				continue
			} else {
				return fmt.Errorf("error getting address for node %s: %v", node.Name, err)
			}
		}

		if !nodeExists(memberNodes, addr, int(port.NodePort)) {
			klog.V(6).Infof("adding %s:%d to load balancer %d", addr, port.NodePort, lbID)
			addNodes = append(addNodes, lbnodes.CreateOpts{
				Port:      int32(port.NodePort),
				Condition: "ENABLED",
				Address:   addr,
			})
		} else {
			// After all node members have been processed, remaining members are deleted as obsolete.
			klog.V(6).Infof("%s:%d is part of load balancer %d", addr, port.NodePort, lbID)
			memberNodes = popNode(memberNodes, addr, int(port.NodePort))
		}
	}

	if len(addNodes) > 0 {
		klog.V(4).Infof("Adding nodes to load balancer %d", lbID)
		_, createErr := lbnodes.Create(lbaas.lb, lbID, addNodes).Extract()
		if createErr != nil {
			return fmt.Errorf("error adding nodes to load balancer: %d, %v", lbID, createErr)
		}
	} else {
		klog.V(4).Infof("No nodes need to be added to load balancer %d", lbID)
	}

	provisioningStatus, err := waitLoadbalancerActiveStatus(lbaas.lb, lbID)
	if err != nil {
		return fmt.Errorf("timeout when waiting for loadbalancer to be ACTIVE after creating node members, current provisioning status %s", provisioningStatus)
	}

	klog.V(4).Infof("Ensured load balancer %d has member nodes", lbID)

	// Delete obsolete nodes for this pool
	for _, node := range memberNodes {
		klog.V(4).Infof("Deleting obsolete node %d for loadbalancer %s address %s", node.ID, lbID, node.Address)
		err := lbnodes.Delete(lbaas.lb, lbID, node.ID).ExtractErr()
		if err != nil && !cpoerrors.IsNotFound(err) {
			return fmt.Errorf("error deleting obsolete node %d for load balancer %d address %s: %v", node.ID, lbID, node.Address, err)
		}
		provisioningStatus, err := waitLoadbalancerActiveStatus(lbaas.lb, lbID)
		if err != nil {
			return fmt.Errorf("timeout when waiting for loadbalancer to be ACTIVE after deleting member, current provisioning status %s", provisioningStatus)
		}
	}

	return nil
}

// TODO: This code currently ignores 'region' and always creates a
// loadbalancer in only the current OpenStack region.  We should take
// a list of regions (from config) and query/create loadbalancers in
// each region.

// EnsureLoadBalancer creates a new load balancer or updates the existing one.
func (lbaas *CloudLb) EnsureLoadBalancer(ctx context.Context, clusterName string, apiService *corev1.Service, nodes []*corev1.Node) (*corev1.LoadBalancerStatus, error) {
	serviceName := fmt.Sprintf("%s/%s", apiService.Namespace, apiService.Name)
	klog.V(4).Infof("EnsureLoadBalancer(%s, %s)", clusterName, serviceName)

	if len(nodes) == 0 {
		return nil, fmt.Errorf("there are no available nodes for LoadBalancer service %s", serviceName)
	}

	ports := apiService.Spec.Ports
	if len(ports) == 0 {
		return nil, fmt.Errorf("no ports provided to openstack load balancer")
	}

	var err error

	// Check for TCP & UDP protocol on each port
	for _, port := range ports {
		if (port.Protocol != corev1.ProtocolTCP) && (port.Protocol != corev1.ProtocolUDP) {
			return nil, fmt.Errorf("only TCP or UDP LoadBalancer protocol is supported for openstack load balancers")
		}
	}

	var sourceRangesCIDRs []accesslists.CreateOpts
	sourceRanges, err := v1service.GetLoadBalancerSourceRanges(apiService)
	if err != nil {
		return nil, fmt.Errorf("failed to get source ranges for loadbalancer service %s: %v", serviceName, err)
	}
	klog.V(2).Infof("Source ranges for Service %s: %+v", serviceName, sourceRanges)
	if !v1service.IsOnlyAllowAll(sourceRanges) {
		for _, sr := range sourceRanges.StringSlice() {
			sourceRangesCIDRs = append(sourceRangesCIDRs, accesslists.CreateOpts{Address: sr, Type: accesslists.Allow})
		}
		sourceRangesCIDRs = append(sourceRangesCIDRs, accesslists.CreateOpts{Address: v1service.DefaultLoadBalancerSourceRanges, Type: accesslists.Deny})
	}

	/* gonna handle this later
	affinity := apiService.Spec.SessionAffinity
	var persistence *v2pools.SessionPersistence
	switch affinity {
	case corev1.ServiceAffinityNone:
		persistence = nil
	case corev1.ServiceAffinityClientIP:
		persistence = &v2pools.SessionPersistence{Type: "SOURCE_IP"}
	default:
		return nil, fmt.Errorf("unsupported load balancer affinity: %v", affinity)
	}
	*/

	// return status
	status := &corev1.LoadBalancerStatus{}

	name := lbaas.GetLoadBalancerName(ctx, clusterName, apiService)
	lbs, err := openstackutil.GetLoadBalancersByName(lbaas.lb, name)
	if err != nil {
		return nil, fmt.Errorf("error getting loadbalancers for Service %s: %v", serviceName, err)
	}

	// if we already have one load balancer for this service, we need to copy its
	// settings to ensure we share IPs for the other ports in the service
	klog.Infof("Source ranges CIDRs for Service %s: %+v", serviceName, sourceRangesCIDRs)
	var vips []virtualips.CreateOpts
	if len(lbs) > 0 {
		vips = cloneLBCreateVips(&lbs[0])
		vipIds := make(map[int]uint64)
		for i, vip := range vips {
			vipIds[i] = vip.ID
		}
		klog.V(2).Infof("Using existing VIPs %v for Service %s", vipIds, serviceName)
	} else {
		klog.V(2).Infof("No VIPs for Service %s yet, creating new VIP", serviceName)
		vips, err = createLBCreateVips(apiService)
		if err != nil {
			return nil, fmt.Errorf("error defining virtual IPs for Service %s: %v", serviceName, err)
		}
	}

	keepClientIP, err := getBoolFromServiceAnnotation(apiService, ServiceAnnotationLoadBalancerXForwardedFor, false)
	if err != nil {
		return nil, fmt.Errorf("error getting %s annotation for Service %s: %v", ServiceAnnotationLoadBalancerXForwardedFor, serviceName, err)
	}

	for _, port := range ports {
		loadbalancer, err := openstackutil.GetLoadBalancerByPort(lbs, port)
		klog.V(2).Infof("LoadBalancer %s for port %d: %+v", name, port.Port, loadbalancer)
		if err == openstackutil.ErrNotFound {
			klog.V(2).Infof("Creating loadbalancer %s for port %d", name, port.Port)

			loadbalancer, err = lbaas.createLoadBalancer(name, vips, port, keepClientIP, sourceRangesCIDRs)
			if err != nil {
				return nil, fmt.Errorf("error creating loadbalancer %s: %v", name, err)
			}
			// since we created our load balancer here, we need to replace vip settings
			// for any future ones created
			vips = cloneLBCreateVips(loadbalancer)
			vipIds := make(map[int]uint64)
			for i, vip := range vips {
				vipIds[i] = vip.ID
			}
			klog.V(2).Infof("Switching to existing VIPs %v for Service %s", vipIds, serviceName)
		} else {
			klog.V(2).Infof("LoadBalancer %s already exists", loadbalancer.Name)
			err = lbaas.ensureLoadBalancerAccesslists(loadbalancer.ID, sourceRangesCIDRs, serviceName)
			if err != nil {
				return nil, fmt.Errorf("error ensuring access lists for load balancer %d: %v", loadbalancer.ID, err)
			}
			lbs = popLB(lbs, loadbalancer)
		}

		provisioningStatus, err := waitLoadbalancerActiveStatus(lbaas.lb, loadbalancer.ID)
		if err != nil {
			return nil, fmt.Errorf("timeout when waiting for loadbalancer to be ACTIVE, current provisioning status %s", provisioningStatus)
		}

		ensureErr := lbaas.ensureLoadBalancerNodes(loadbalancer.ID, port, nodes)
		if ensureErr != nil {
			return nil, ensureErr
		}
		status.Ingress = []corev1.LoadBalancerIngress{{IP: loadbalancer.VirtualIps[0].Address}}
	}

	// Delete obsolete load balancers for this service
	klog.V(2).Infof("Deleting load balancers no longer part of Service %s: ", serviceName)
	err = lbaas.deleteLoadBalancers(lbs)
	if err != nil {
		return nil, err
	}

	//for _, node := range memberNodes {
	/* health monitoring to come later
	monitorID := pool.MonitorID
	enableHealthMonitor, err := getBoolFromServiceAnnotation(apiService, ServiceAnnotationLoadBalancerEnableHealthMonitor, lbaas.opts.CreateMonitor)
	if err != nil {
		return nil, err
	}
	if monitorID == "" && enableHealthMonitor {
		klog.V(4).Infof("Creating monitor for pool %s", loadbalancer.ID)
		monitorProtocol := string(port.Protocol)
		if port.Protocol == corev1.ProtocolUDP {
			monitorProtocol = "UDP-CONNECT"
		}
		monitor, err := v2monitors.Create(lbaas.lb, v2monitors.CreateOpts{
			Name:       cutString(fmt.Sprintf("monitor_%d_%s)", portIndex, name)),
			PoolID:     loadbalancer.ID,
			Type:       monitorProtocol,
			Delay:      int(lbaas.opts.MonitorDelay.Duration.Seconds()),
			Timeout:    int(lbaas.opts.MonitorTimeout.Duration.Seconds()),
			MaxRetries: int(lbaas.opts.MonitorMaxRetries),
		}).Extract()
		if err != nil {
			return nil, fmt.Errorf("error creating LB pool healthmonitor: %v", err)
		}
		provisioningStatus, err := waitLoadbalancerActiveStatus(lbaas.lb, loadbalancer.ID)
		if err != nil {
			return nil, fmt.Errorf("timeout when waiting for loadbalancer to be ACTIVE after creating monitor, current provisioning status %s", provisioningStatus)
		}
		monitorID = monitor.ID
	} else if monitorID != "" && !enableHealthMonitor {
		klog.Infof("Deleting health monitor %s for pool %s", monitorID, loadbalancer.ID)
		err := v2monitors.Delete(lbaas.lb, monitorID).ExtractErr()
		if err != nil {
			return nil, fmt.Errorf("failed to delete health monitor %s for pool %s, error: %v", monitorID, loadbalancer.ID, err)
		}
	}
	*/
	//}

	return status, nil
}

func IsAccessListModified(currentAccessLists []accesslists.NetworkItem, newAccessLists []accesslists.CreateOpts) bool {
	klog.V(2).Infof("Checking if access lists are modified, current: %+v, new: %+v", currentAccessLists, newAccessLists)
	// Check if the current access lists are different from the new access lists
	if len(currentAccessLists) != len(newAccessLists) {
		return true
	}

	// Check if the addresses in the current access lists are different from the new access lists
	accessListMap := make(map[string]struct{}, len(newAccessLists))
	for _, networkItem := range newAccessLists {
		accessListMap[networkItem.Address] = struct{}{}
	}

	for _, networkItem := range currentAccessLists {
		if _, found := accessListMap[networkItem.Address]; !found {
			return true
		}
	}

	// Check if the addresses in the new access lists are different from the current access lists
	activeAccessListMap := make(map[string]struct{}, len(currentAccessLists))
	for _, networkItem := range currentAccessLists {
		activeAccessListMap[networkItem.Address] = struct{}{}
	}

	for _, networkItem := range newAccessLists {
		if _, found := activeAccessListMap[networkItem.Address]; !found {
			return true
		}
	}

	return false
}

func (lbaas *CloudLb) ensureLoadBalancerAccesslists(lbID uint64, sourceRangesCIDRs []accesslists.CreateOpts, serviceName string) error {
	// Get the current access lists for the load balancer
	accessLists, err := accesslists.Get(lbaas.lb, lbID).Extract()
	if err != nil && !cpoerrors.IsNotFound(err) {
		return fmt.Errorf("error getting access lists for load balancer %d: %v", lbID, err)
	}
	klog.V(2).Infof("Current access lists for load balancer %d: %v", lbID, accessLists)

	if IsAccessListModified(accessLists, sourceRangesCIDRs) {
		// We don't need to delete the access lists if there is nothing to update
		// so that we can avoid the error as this won't allow the below POST to succeed:
		// {"code":422,"message":"Load Balancer '<lb-id>' has a status of 'PENDING_UPDATE' and is considered immutable."}
		if len(accessLists) != 0 {
			klog.V(2).Infof("Access lists for load balancer %d are modified, updating access lists", lbID)
			err := accesslists.DeleteAll(lbaas.lb, lbID).ExtractErr()
			if err != nil && !cpoerrors.IsNotFound(err) {
				return fmt.Errorf("error deleting access lists for load balancer %d: %v", lbID, err)
			}
		}

		// If the source ranges CIDRs are empty, we will not update the access lists
		if len(sourceRangesCIDRs) == 0 {
			klog.V(2).Infof("No source ranges CIDRs for Service %s, skipping access lists update & deleting anything if exists", serviceName)
			return nil
		}

		klog.V(2).Infof("Adding access lists %v to load balancer %d", sourceRangesCIDRs, lbID)
		err = accesslists.Create(lbaas.lb, lbID, sourceRangesCIDRs).ExtractErr()
		if err != nil && !cpoerrors.IsNotFound(err) {
			return fmt.Errorf("error adding access lists for load balancer %d: %v", lbID, err)
		}

	}

	return nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
func (lbaas *CloudLb) UpdateLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) error {
	serviceName := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	klog.V(4).Infof("UpdateLoadBalancer(%v, %s, %v)", clusterName, serviceName, nodes)
	ports := service.Spec.Ports
	if len(ports) == 0 {
		return fmt.Errorf("no ports provided to openstack load balancer")
	}

	name := lbaas.GetLoadBalancerName(ctx, clusterName, service)
	lbs, err := openstackutil.GetLoadBalancersByName(lbaas.lb, name)
	if err != nil {
		return err
	}

	// Check for adding/removing members associated with each port
	for _, port := range ports {
		loadbalancer, err := openstackutil.GetLoadBalancerByPort(lbs, port)
		if loadbalancer == nil {
			return fmt.Errorf("loadbalancer does not exist for Service %s", serviceName)
		}

		provisioningStatus, err := waitLoadbalancerActiveStatus(lbaas.lb, loadbalancer.ID)
		if err != nil {
			return fmt.Errorf("timeout when waiting for loadbalancer to be ACTIVE, current provisioning status %s", provisioningStatus)
		}

		ensureErr := lbaas.ensureLoadBalancerNodes(loadbalancer.ID, port, nodes)
		if ensureErr != nil {
			return ensureErr
		}
	}

	return nil
}

// EnsureLoadBalancerDeleted deletes the specified load balancer
func (lbaas *CloudLb) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *corev1.Service) error {
	serviceName := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	klog.V(4).Infof("EnsureLoadBalancerDeleted(%s, %s)", clusterName, serviceName)
	name := lbaas.GetLoadBalancerName(ctx, clusterName, service)
	lbs, err := openstackutil.GetLoadBalancersByName(lbaas.lb, name)
	if err != nil {
		return err
	}

	if len(lbs) == 0 {
		return nil
	}

	return lbaas.deleteLoadBalancers(lbs)
}
