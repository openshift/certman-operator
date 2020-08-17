#!/bin/bash
# This script will test certman-operator in minikube by doing the following:
# 1. Spoof a ClusterDeployment to replicate running in Hive,
#    which will trigger certificates to be generated.
set -o errexit
set -o pipefail

BASE_DOMAIN=$1
TOTAL_COUNT=$2

NAMES_ARR=()
START_TIMES_ARR=()
DURATION_TIMES_ARR=()

function usage() {
  echo "Usage:"
  echo "    $0 <BASE_DOMAIN> <COUNT>"
  echo
  echo "Example:"
  echo "    $0 acme.io 15"
}

function cleanup() {
    for i in $(seq 1 $TOTAL_COUNT)
    do
        NAME=${NAMES_ARR[$i]}
        kubectl delete clusterdeployment ${NAME}
    done
}

if [ -z "${BASE_DOMAIN}" ]; then
    usage
    exit 1
fi

if [ -z "${TOTAL_COUNT}" ]; then
    usage
    exit 1
fi

trap cleanup INT TERM EXIT

for i in $(seq 1 $TOTAL_COUNT)
do
    CD_START_TIME=`date +%s`
    RAND0=$(head /dev/urandom | tr -dc a-z | head -c 4 ; echo '')
    RAND1=$(head /dev/urandom | tr -dc A-Za-z0-9 | head -c 8 ; echo '')

    NAMES_ARR[$i]=${RAND0}
    START_TIMES_ARR[$i]=${CD_START_TIME}

    cat <<EOF | kubectl apply -f -
apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata:
  creationTimestamp: null
  labels:
    api.openshift.com/ccs: "false"
    api.openshift.com/environment: staging
    api.openshift.com/managed: "true"
    api.openshift.com/name: ${RAND0}
    api.openshift.com/service-lb-quota: "0"
    api.openshift.com/storage-quota-gib: "100"
    hive.openshift.io/cluster-platform: aws
    hive.openshift.io/cluster-region: us-west-2
    hive.openshift.io/cluster-type: managed
  name: ${RAND0}
spec:
  baseDomain: ${BASE_DOMAIN}
  certificateBundles:
  - certificateSecretRef:
      name: ${RAND0}-primary-cert-bundle-secret
    generate: true
    name: ${RAND0}-primary-cert-bundle
  clusterMetadata:
    adminKubeconfigSecretRef:
      name: ${RAND0}-admin-kubeconfig
    adminPasswordSecretRef:
      name: ${RAND0}-admin-password
    clusterID: ${RAND1}
    infraID: ${RAND1}
  clusterName: ${RAND0}
  ingress:
  - domain: ${RAND0}.${BASE_DOMAIN}
    name: default
    servingCertificate: ${RAND0}-primary-cert-bundle
  installed: true
  manageDNS: true
  platform:
    aws:
      credentialsSecretRef:
        name: aws
      region: us-west-2
  provisioning:
    imageSetRef:
      name: openshift-v4.3.25
    installConfigSecretRef:
      name: install-config
    sshPrivateKeySecretRef:
      name: ssh
  pullSecretRef:
    name: pull
EOF
done

ISSUED_COUNT=0

echo "Waiting for $TOTAL_COUNT certificates to be issued..."

while [ $ISSUED_COUNT -lt $TOTAL_COUNT ]
do
    for i in $(seq 1 $TOTAL_COUNT)
    do
        NAME=${NAMES_ARR[$i]}

        if [ ! ${DURATION_TIMES_ARR[$i]+abc} ]; then
            #echo "Checking if ${NAME}.${BASE_DOMAIN} certificate was issued"
            ISSUED=$(kubectl get certificaterequests ${NAME}-${NAME}-primary-cert-bundle -o json | jq .status.issued)

            if [ "${ISSUED}" == "true" ]; then
                CERT_ISSUED_END_TIME=`date +%s`
                CERT_ISSUE_DURATION=$((CERT_ISSUED_END_TIME-CD_START_TIME))
                DURATION_TIMES_ARR[$i]=${CERT_ISSUE_DURATION}
                echo "${NAME} ISSUE DURATION: ${CERT_ISSUE_DURATION} seconds"
                ((ISSUED_COUNT++))
            fi
        fi
    done
    sleep 1
done

cleanup
