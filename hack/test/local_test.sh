#!/bin/bash
# This script will test certman-operator in minikube by doing the following:
# 1. Create a test cluster locally.
# 2. Install certman's dependencies and CRDs.
# 3. Create a podman/docker image of the operator.
# 4. Run the new image as a deployment in the cluster.
# 5. Spoof a ClusterDeployment to replicate running in Hive,
#    which will trigger certificates to be generated.
set -o errexit
set -o errtrace
set -o nounset
set -o pipefail

initial_dir="$PWD"

function cleanup() {
  cd $initial_dir
  echo "Cleaning up minikube profile"
  minikube delete -p certtest

  echo "Cleaning up temporary files"
  if [ -d ./hack/test/tmp ]; then
    rm -rf ./hack/test/tmp
  fi
}

trap 'echo "ERROR at line ${LINENO}"; cleanup' ERR

function usage() {
  echo "Usage:"
  echo "    export AWS_ACCESS_KEY_ID=yourkey"
  echo "    export AWS_SECRET_ACCESS_KEY=yoursecretkey"
  echo "    $0 <your-letsencrypt-key-file.pem> <your-letsencrypt-URL>"
  echo
  echo "Example:"
  echo "    $0 ~/work/stg-private-key.pem https://acme-staging-v02.api.letsencrypt.org/acme/acct/123456"
}

if [ -z "${1:-}" ] || [ ! -f "${1:-}" ]; then
  echo "Please specify a valid file for the Let's Encrypt private key."
  usage
  exit 1
fi

if [ -z "${2:-}" ] ; then
  echo "Please specify your Let's Encrypt account URL."
  usage
  exit 1
fi

if [ -z "${AWS_ACCESS_KEY_ID:-}" ] ; then
  echo "Please export your AWS_ACCESS_KEY_ID."
  usage
  exit 1
fi

if [ -z "${AWS_SECRET_ACCESS_KEY:-}" ] ; then
  echo "Please export your AWS_SECRET_ACCESS_KEY."
  usage
  exit 1
fi

# default to podman, fail back to docker
which podman > /dev/null 2>&1 # returns 1 if podman isn't found
if [ $? == 0 ]; then
  ENGINE=podman
elif [ $? == 1 ]; then
  ENGINE=docker
fi

# Ensure that this script is run from the root of the operator's directory.
if [[ ! $(pwd) =~ .*certman-operator$ ]]; then
  echo "Please run this script from the root of the operator directory"
  exit
fi

if [ "${ENGINE}" == "docker" ]; then
  echo "Checking if docker service is active"
  systemctl is-active docker
fi

testdir="${initial_dir}/hack/test"
tmpdir="${initial_dir}/hack/test/tmp"
mkdir ${tmpdir}

# Start minikube with extra memory (needed for the go build)
MINIKUBE_OPTIONS="--memory=6g --bootstrapper=kubeadm --extra-config=kubelet.authentication-token-webhook=true --extra-config=kubelet.authorization-mode=Webhook --extra-config=scheduler.address=0.0.0.0 --extra-config=controller-manager.address=0.0.0.0 --extra-config=apiserver.service-node-port-range=1-65535"
# kubernetes v1.22 stopped supporting CustomResourceDefinition in apiextensions.k8s.io/v1beta1
# unfortunately, the router CRDs are still using that API
KUBE_VERSION="--kubernetes-version=v1.21.5"

minikube start -p certtest $MINIKUBE_OPTIONS $KUBE_VERSION
kubectl config use-context certtest

# Install openshift router
cd $tmpdir
git clone git@github.com:openshift/router.git
cd router
kubectl create -n openshift-ingress -f deploy/route_crd.yaml
kubectl create -n openshift-ingress -f deploy/router_rbac.yaml
kubectl create -n openshift-ingress -f deploy/router.yaml

# Create test namespaces
kubectl create -f ${testdir}/deploy/namespace.yaml

cd $tmpdir
git clone git@github.com:openshift/hive.git
cd hive
kubectl create -f config/crds

echo $2 > ${tmpdir}/accounturl.txt

echo "Creating secrets on minikube cluster"
kubectl -n certman-operator create secret generic lets-encrypt-account-staging \
    --from-file=private-key="${1}" \
    --from-file=account-url=${tmpdir}/accounturl.txt

kubectl -n certtest create secret generic aws --from-literal=aws_access_key_id=${AWS_ACCESS_KEY_ID} --from-literal=aws_secret_access_key=${AWS_SECRET_ACCESS_KEY}

echo "Creating configmap"
kubectl create -f ${testdir}/deploy/configmap.yaml

echo "Deleting temp dir to avoid build conflicts"
cd ${initial_dir}
rm -rf ./hack/test/tmp

echo "Building ${ENGINE} image from current working branch"
if [ "${ENGINE}" == "podman" ]; then
  eval $(minikube podman-env -p certtest)
  podman-remote build -f build/Dockerfile -t localhost/certman-operator:latest .
elif [ "${ENGINE}" == "docker" ]; then
  eval $(minikube docker-env -p certtest)
  docker build -f build/Dockerfile . -t localhost/certman-operator:latest
fi
kubectl create -f deploy/service_account.yaml
kubectl create -f deploy/role.yaml
kubectl create -f deploy/role_binding.yaml
kubectl create -f deploy/crds/certman.managed.openshift.io_certificaterequests.yaml
kubectl create -f ${testdir}/deploy/deploy.yaml -n certman-operator
kubectl create -f ${testdir}/deploy/service.yaml -n certman-operator
# install monitoring stack for the ServiceMonitor CRD and so we can verify monitoring works
helm install kube-monitoring prometheus-community/prometheus-operator -n openshift-monitoring
kubectl create -f ${testdir}/deploy/service_monitor.yaml -n certman-operator

echo "Certman-operator is now deployed. To view the pod, run:"
echo "  kubectl get pods -n certman-operator"

echo "Simulate a cluster build with:"
echo "  kubectl create -f ./hack/test/deploy/clusterdeploy.yaml"

echo "Delete cluster when finished:"
echo "  minikube delete -p certtest"
