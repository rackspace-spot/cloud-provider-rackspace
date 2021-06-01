module github.com/os-pc/cloud-provider-rackspace

go 1.13

require (
	github.com/gophercloud/gophercloud v0.7.1-0.20200116010453-5391bb776e58
	github.com/gophercloud/utils v0.0.0-20191020172814-bd86af96d544
	github.com/gorilla/mux v1.7.3
	github.com/hashicorp/go-version v1.2.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.1.2
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/os-pc/gocloudlb v0.0.0-20210529010120-65b17b6d1ffa // indirect
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.4.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.4.0
	github.com/stretchr/testify v1.5.1
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975
	golang.org/x/net v0.0.0-20191126235420-ef20fe5d7933
	golang.org/x/sys v0.0.0-20200122134326-e047566fdf82
	google.golang.org/grpc v1.26.0
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.0
	k8s.io/apiserver v0.18.0
	k8s.io/client-go v0.18.0
	k8s.io/cloud-provider v0.18.0
	k8s.io/component-base v0.18.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.18.0
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	software.sslmate.com/src/go-pkcs12 v0.0.0-20190209200317-47dd539968c4
)

replace (
	github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.0-rc9
	k8s.io/api => k8s.io/api v0.18.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.0
	k8s.io/apiserver => k8s.io/apiserver v0.18.0
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.0
	k8s.io/client-go => k8s.io/client-go v0.18.0
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.0
	k8s.io/code-generator => k8s.io/code-generator v0.18.0
	k8s.io/component-base => k8s.io/component-base v0.18.0
	k8s.io/cri-api => k8s.io/cri-api v0.18.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.0
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.0
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.0
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.0
	k8s.io/kubectl => k8s.io/kubectl v0.18.0
	k8s.io/kubelet => k8s.io/kubelet v0.18.0
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.0
	k8s.io/metrics => k8s.io/metrics v0.18.0
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.0
)
