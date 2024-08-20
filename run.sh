#!/bin/bash

# Check for required environment variables
if [ -z "$CLOUD_CONFIG" ]; then
  echo "Error: CLOUD_CONFIG environment variable is not set."
  exit 1
fi

if [ -z "$KUBECONFIG" ]; then
  echo "Error: KUBECONFIG environment variable is not set."
  exit 1
fi

if [ -z "$AUTHENTICATION_KUBECONFIG" ]; then
  echo "Error: AUTHENTICATION_KUBECONFIG environment variable is not set."
  exit 1
fi

if [ -z "$CLUSTER_NAME" ]; then
  echo "Error: CLUSTER_NAME environment variable is not set."
  exit 1
fi

# Build the program
TARGET=openstack-cloud-controller-manager  # You can set this based on your preference
GCO_ENABLED=0 GOOS=linux go build -ldflags "${LDFLAGS}" -o "${TARGET}" cmd/openstack-cloud-controller-manager/main.go

# Set default values for optional environment variables if not provided
VERBOSITY=${VERBOSITY:-2}
USE_SERVICE_ACCOUNT_CREDENTIALS=${USE_SERVICE_ACCOUNT_CREDENTIALS:-true}
BIND_ADDRESS=${BIND_ADDRESS:-127.0.0.1}
CLOUD_PROVIDER=${CLOUD_PROVIDER:-openstack}

# Run the program
./"${TARGET}" \
    --cloud-config=${CLOUD_CONFIG} \
    --v=${VERBOSITY} \
    --cluster-name=${CLUSTER_NAME} \
    --use-service-account-credentials=${USE_SERVICE_ACCOUNT_CREDENTIALS} \
    --bind-address=${BIND_ADDRESS} \
    --cloud-provider=${CLOUD_PROVIDER} \
    --kubeconfig=${KUBECONFIG} \
    --authentication-kubeconfig=${AUTHENTICATION_KUBECONFIG}
