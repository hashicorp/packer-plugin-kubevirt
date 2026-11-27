#!/usr/bin/env bash
#
# Apache License 2.0 (see LICENSE or http://www.apache.org/licenses/LICENSE-2.0)
#
# Copyright Red Hat, Inc.
#
# Adapted from https://github.com/kubevirt/kubevirt.core/blob/main/hack/e2e-setup.sh

ARGCOUNT=$#
DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
ARCH=""
case $(uname -m) in
x86_64) ARCH="amd64" ;;
aarch64) ARCH="arm64" ;;
esac

set_default_params() {
  BIN_DIR=${BIN_DIR:-$DIR/../bin}

  KIND=${KIND:-$BIN_DIR/kind}
  KIND_VERSION=${KIND_VERSION:-v0.33.0}
  KIND_NODE_IMAGE=${KIND_NODE_IMAGE:-kindest/node:v1.35.8}

  KUBECTL=${KUBECTL:-$BIN_DIR/kubectl}
  KUBECTL_VERSION=${KUBECTL_VERSION:-v1.35.10}

  KUBEVIRT_VERSION=${KUBEVIRT_VERSION:-v1.10.0}
  KUBEVIRT_CDI_VERSION=${KUBEVIRT_CDI_VERSION:-v1.66.1}

  CLUSTER_NAME=${CLUSTER_NAME:-kind}
}

detect_cri() {
  PODMAN_SOCKET=${PODMAN_SOCKET:-"/run/podman/podman.sock"}

  if [ "${CRI}" = "podman" ]; then
    _cri_socket=$(detect_podman_socket)
    _cri_bin="podman --remote --url=unix://$_cri_socket"
  elif [ "${CRI}" = "docker" ]; then
    _cri_bin=docker
    _cri_socket="/var/run/docker.sock"
  else
    _cri_socket=$(detect_podman_socket)
    if [ -n "$_cri_socket" ]; then
      _cri_bin="podman --remote --url=unix://$_cri_socket"
      echo >&2 "selecting podman as container runtime"
    elif docker ps >/dev/null 2>&1; then
      _cri_bin=docker
      _cri_socket="/var/run/docker.sock"
      echo >&2 "selecting docker as container runtime"
    else
      echo >&2 "no working container runtime found. Neither docker nor podman seems to work."
      exit 1
    fi
  fi
}

detect_podman_socket() {
  if curl --unix-socket "${PODMAN_SOCKET}" http://d/v3.0.0/libpod/info >/dev/null 2>&1; then
    echo "${PODMAN_SOCKET}"
  fi
}

# Downloads to a temporary file and only moves it into place on success, so a
# failed download never leaves a broken binary that later runs would reuse.
download_binary() {
  local url=$1
  local dest=$2

  mkdir -p "$(dirname "${dest}")"
  if ! curl -fL -o "${dest}.tmp" "${url}"; then
    rm -f "${dest}.tmp"
    echo >&2 "failed to download ${url}"
    exit 1
  fi
  chmod +x "${dest}.tmp"
  mv "${dest}.tmp" "${dest}"
}

install_kind() {
  if [ ! -f "${KIND}" ]; then
    echo "Installing kind"
    download_binary "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${ARCH}" "${KIND}"

    echo "Successfully installed kind at ${KIND}:"
    ${KIND} version
  fi
}

install_kubectl() {
  if [ ! -f "${KUBECTL}" ]; then
    echo "Installing kubectl"
    download_binary "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl" "${KUBECTL}"

    echo "Successfully installed kubectl at ${KUBECTL}:"
    ${KUBECTL} version --client
  fi
}

configure_inotify_limits() {
  echo "Configuring inotify limits"
  sudo sysctl fs.inotify.max_user_instances=512
  sudo sysctl fs.inotify.max_user_watches=1048576
}

create_cluster() {
  echo "Creating cluster with kind"
  DOCKER_HOST=unix://${_cri_socket} ${KIND} create cluster --wait 2m --name "${CLUSTER_NAME}" --image "${KIND_NODE_IMAGE}"

  echo "Waiting for the network to be ready"
  ${KUBECTL} wait --for=condition=ready pods --namespace=kube-system -l k8s-app=kube-dns --timeout=2m

  echo "K8S cluster is up:"
  ${KUBECTL} get nodes -o wide
}

deploy_kubevirt() {
  echo "Deploying KubeVirt"
  ${KUBECTL} create -f "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-operator.yaml"
  ${KUBECTL} create -f "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-cr.yaml"

  if ! is_nested_virt_enabled; then
    echo "Configuring Kubevirt to use emulation"
    ${KUBECTL} patch kubevirt kubevirt --namespace kubevirt --type=merge --patch '{"spec":{"configuration":{"developerConfiguration":{"useEmulation":true}}}}'
  fi

  echo "Waiting for KubeVirt to be ready"
  ${KUBECTL} wait --for=condition=Available kubevirt kubevirt --namespace=kubevirt --timeout=5m

  echo "Successfully deployed KubeVirt:"
  ${KUBECTL} get pods -n kubevirt
}

is_nested_virt_enabled() {
  local kvm_nested="unknown"
  if [ -f "/sys/module/kvm_intel/parameters/nested" ]; then
    kvm_nested=$(cat /sys/module/kvm_intel/parameters/nested)
  elif [ -f "/sys/module/kvm_amd/parameters/nested" ]; then
    kvm_nested=$(cat /sys/module/kvm_amd/parameters/nested)
  fi
  [ "$kvm_nested" == "1" ] || [ "$kvm_nested" == "Y" ] || [ "$kvm_nested" == "y" ]
}

deploy_kubevirt_cdi() {
  echo "Deploying KubeVirt containerized-data-importer"
  ${KUBECTL} apply -f "https://github.com/kubevirt/containerized-data-importer/releases/download/${KUBEVIRT_CDI_VERSION}/cdi-operator.yaml"
  ${KUBECTL} apply -f "https://github.com/kubevirt/containerized-data-importer/releases/download/${KUBEVIRT_CDI_VERSION}/cdi-cr.yaml"

  echo "Waiting for KubeVirt containerized-data-importer to be ready"
  ${KUBECTL} wait --for=condition=Available cdi cdi --timeout=5m

  echo "Successfully deployed KubeVirt containerized-data-importer:"
  ${KUBECTL} get pods -n cdi
}

cleanup() {
  DOCKER_HOST=unix://${_cri_socket} ${KIND} delete cluster --name "${CLUSTER_NAME}"
}

usage() {
  echo "$0 [--install-kind] [--install-kubectl] [--configure-inotify-limits] [--create-cluster] [--deploy-kubevirt] [--deploy-kubevirt-cdi] [--cleanup]"
}

set_default_options() {
  OPT_INSTALL_KIND=false
  OPT_INSTALL_KUBECTL=false
  OPT_CONFIGURE_INOTIFY_LIMITS=false
  OPT_CREATE_CLUSTER=false
  OPT_DEPLOY_KUBEVIRT=false
  OPT_DEPLOY_KUBEVIRT_CDI=false
  OPT_CLEANUP=false
}

parse_args() {
  while [ "$1" != "" ]; do
    case "$1" in
    --install-kind) OPT_INSTALL_KIND=true ;;
    --install-kubectl) OPT_INSTALL_KUBECTL=true ;;
    --configure-inotify-limits) OPT_CONFIGURE_INOTIFY_LIMITS=true ;;
    --create-cluster) OPT_CREATE_CLUSTER=true ;;
    --deploy-kubevirt) OPT_DEPLOY_KUBEVIRT=true ;;
    --deploy-kubevirt-cdi) OPT_DEPLOY_KUBEVIRT_CDI=true ;;
    --cleanup) OPT_CLEANUP=true ;;
    -v | --verbose)
      set -x
      ;;
    --help)
      usage
      exit
      ;;
    *)
      usage
      exit 1
      ;;
    esac
    shift
  done
}

set_default_params
set_default_options
parse_args "$@"

detect_cri

set -euo pipefail

if [ "${ARGCOUNT}" -eq "0" ]; then
  OPT_INSTALL_KIND=true
  OPT_INSTALL_KUBECTL=true
  OPT_CREATE_CLUSTER=true
  OPT_DEPLOY_KUBEVIRT=true
  OPT_DEPLOY_KUBEVIRT_CDI=true
fi

if [ "${OPT_INSTALL_KIND}" == true ]; then
  install_kind
fi

if [ "${OPT_INSTALL_KUBECTL}" == true ]; then
  install_kubectl
fi

if [ "${OPT_CLEANUP}" == true ]; then
  cleanup
  exit 0
fi

if [ "${OPT_CONFIGURE_INOTIFY_LIMITS}" == true ]; then
  configure_inotify_limits
fi

if [ "${OPT_CREATE_CLUSTER}" == true ]; then
  create_cluster
fi

if [ "${OPT_DEPLOY_KUBEVIRT}" == true ]; then
  deploy_kubevirt
fi

if [ "${OPT_DEPLOY_KUBEVIRT_CDI}" == true ]; then
  deploy_kubevirt_cdi
fi
