/*
Copyright 2016 The Kubernetes Authors.

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

// The external controller manager is responsible for running controller loops that
// are cloud provider dependent. It uses the API to listen to new events on resources.

package main

import (
	"flag"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/options"
	cliflag "k8s.io/component-base/cli/flag"
	_ "k8s.io/component-base/metrics/prometheus/restclient" // for client metric registration
	_ "k8s.io/component-base/metrics/prometheus/version"    // for version metric registration
	"k8s.io/klog/v2"
	_ "k8s.io/kubernetes/pkg/features" // add the kubernetes feature gates

	"github.com/os-pc/cloud-provider-rackspace/pkg/cloudprovider/providers/openstack"
	"github.com/os-pc/cloud-provider-rackspace/pkg/version"
)

func checkCurrentLogLevel() {
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "v" {
			klog.Infof("Current log level: %s\n", f.Value)

		}
	})
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Initialize klog
	klog.InitFlags(nil)
	defer klog.Flush()

	// Register the custom flags explicitly
	pflag.String("cloud-config", "", "Path to the cloud provider configuration file")
	pflag.String("cluster-name", "", "The name of the Kubernetes cluster")
	pflag.Bool("use-service-account-credentials", true, "Use service account credentials")
	pflag.String("bind-address", "127.0.0.1", "The address to bind to")
	pflag.String("cloud-provider", "openstack", "The cloud provider name")
	pflag.String("kubeconfig", "", "Path to the kubeconfig file")
	pflag.String("authentication-kubeconfig", "", "Path to the authentication kubeconfig file")

	// Integrate goflag with pflag
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	pflag.Parse()

	// Parse the combined flags
	if err := goflag.CommandLine.Parse([]string{}); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing flags: %v\n", err)
		os.Exit(1)
	}
	checkCurrentLogLevel()
	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}
	fss := cliflag.NamedFlagSets{}
	command := app.NewCloudControllerManagerCommand(ccmOptions, cloudInitializer, app.DefaultInitFuncConstructors, fss, wait.NeverStop)
	// Ensure flags from the openstack package are added
	openstack.AddExtraFlags(pflag.CommandLine)

	// Normalize and add the goflag set
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	klog.V(1).Infof("openstack-cloud-controller-manager version: %s", version.Version)

	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cloudInitializer(config *config.CompletedConfig) cloudprovider.Interface {
	cloudConfig := config.ComponentConfig.KubeCloudShared.CloudProvider
	// initialize cloud provider with the cloud provider name and config file provided
	cloud, err := cloudprovider.InitCloudProvider(cloudConfig.Name, cloudConfig.CloudConfigFile)
	if err != nil {
		klog.Fatalf("Cloud provider could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("Cloud provider is nil")
	}

	if !cloud.HasClusterID() {
		if config.ComponentConfig.KubeCloudShared.AllowUntaggedCloud {
			klog.Warning("detected a cluster without a ClusterID.  A ClusterID will be required in the future.  Please tag your cluster to avoid any future issues")
		} else {
			klog.Fatalf("no ClusterID found.  A ClusterID is required for the cloud provider to function properly.  This check can be bypassed by setting the allow-untagged-cloud option")
		}
	}
	return cloud
}
