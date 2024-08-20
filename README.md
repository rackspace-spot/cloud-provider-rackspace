# Cloud Provider Rackspace

This repository holds plugins related to Kubernetes and Rackspace OpenStack Public Cloud integration.

## Deploying

The controller is available as a Docker container at:
- docker.io/ospc/rackspace-cloud-controller-manager

### Steps

- Create a secret containing Rackspace authentication and configuration for your account.  You can find an example config file in [`manifests/cloud-config`](/manifests/cloud-config).

    ```shell
    kubectl create secret -n kube-system generic rackspace-cloud-config --from-file=cloud-config
    ```

- Create RBAC resources

    ```shell
    kubectl apply -f https://raw.githubusercontent.com/os-pc/cloud-provider-rackspace/master/manifests/cloud-controller-manager-roles.yaml
    kubectl apply -f https://raw.githubusercontent.com/os-pc/cloud-provider-rackspace/master/manifests/cloud-controller-manager-role-bindings.yaml
    ```

- Create the rackspace-cloud-controller-manager deamonset. You will likely want to set a unique cluster name to avoid trampling load balancers of other clusters on your account.

    ```shell
    curl -o rackspace-cloud-controller-manager-ds.yaml
    $(EDITOR) rackspace-cloud-controller-manager-ds.yaml
    # replace "your-cluster" in --cluster-name=your-cluster with your cluster name
    kubectl apply -f rackspace-cloud-controller-manager-ds.yaml
    ```

- Waiting for all the pods in kube-system namespace up and running.

# Versioning

This project does not use semver but follows the Kubernetes project with major.minor numbers
based on the Kubernetes version supported and the patch being the release of this project. So
v1.18.0 would be the 1st version built against Kubernetes 1.18 APIs while v1.20.4 would be
the 5th version built against Kubernetes 1.20 APIs.

# Development

This controller is developed and tested against Rackspace OpenStack Public Cloud using Go 1.16.
Pull requests are welcome.

## How to run locally. 

1. In order to test your changes you will need a kubernetes cluster. If you don't have a kubernetes cluster then create one.
2. Obtain the kubeconfig for your kubernetes cluster. We will later use this to create a Kubernetes resource for our testing.
5. Scale down the cloud-provider deployment for your kubernetes cluster to zero.
6. Set following environment variables CLOUD_CONFIG, KUBECONFIG , AUTHENTICATION_KUBECONFIG, CLUSTER_NAME to appropriate values. 
    * CLOUD_CONFIG - location where cloud.conf file is stored.
    * KUBECONFIG , AUTHENTICATION_KUBECONFIG -  Where the kubeconfig of your VCP for your cloudspace.
    * CLUSTER_NAME - your VCP name

7. Run ``` bash run.sh ```.  This will run cloud-provider in local while pointing to the API server of your kubernetes cluster.
8. Open up another terminal and create required resources in your kubernetes cluster to test your changes.

## License

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
