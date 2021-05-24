# Cloud Provider Rackspace

This repository holds plugins related to Kubernetes and Rackspace OpenStack Public Cloud integration.

## Usage

The controller is available as a Docker container at:
- docker.io/ospc/rackspace-cloud-controller-manager
- docker.pkg.github.com/os-pc/cloud-provider-rackspace/rackspace-cloud-controller-manager

// more to come

# Versioning

This project does not use semver but follows the Kubernetes project with major.minor numbers
based on the Kubernetes version supported and the patch being the release of this project. So
v1.18.0 would be the 1st version built against Kubernetes 1.18 APIs while v1.20.4 would be
the 5th version built against Kubernetes 1.20 APIs.

# Development

This controller is developed and tested against Rackspace OpenStack Public Cloud using Go 1.16.
Pull requests are welcome.

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
