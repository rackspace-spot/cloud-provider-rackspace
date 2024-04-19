GOFMT_FILES ?= $$(find . -name '*.go' |grep -v vendor)
GOTEST      ?= $$(go list ./... |grep -v 'vendor')

VERSION     ?= $(shell git describe --exact-match 2> /dev/null || \
               git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)

TARGET      := rackspace-cloud-controller-manager
LDFLAGS     := "-w -s -X 'github.com/os-pc/cloud-provider-rackspace/pkg/version.Version=${VERSION}'"
REGISTRY    := rackspacengpc

default: build

fmtcheck:
	@echo "gofmt -s -d *.go"
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

fmt:
	gofmt -s -w $(GOFMT_FILES)

test: fmtcheck
	go test $(TEST)

build: fmtcheck
	GCO_ENABLED=0 GOOS=linux go build \
		-ldflags $(LDFLAGS) \
		-o $(TARGET) \
		cmd/openstack-cloud-controller-manager/main.go

clean:
	rm -rf $(TARGET)

image: build
	docker build -t $(REGISTRY)/$(TARGET):${VERSION} .
push: image
	docker push $(REGISTRY)/$(TARGET):${VERSION}
