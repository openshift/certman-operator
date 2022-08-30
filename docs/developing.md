## Setting up Certman Operator for local dev
### Requirements
To develop Certman Operator you will need the following:

* Access to the hive repo, https://github.com/openshift/hive
* Let’s Encrypt account id and private key.
* AWS keys and secret keys for the account you’re using.
* An OpenShift test cluster

### Setup Hive Dependencies
We don't need a running Hive to test and develop with. But we will need some CRDs from the project. Most importantly Certman Operator watches for ClusterDeployments and responds to that CR when the status is 'true'.

```bash
git clone git@github.com:openshift/hive.git
oc create -f hive/config/crds
```
### Install CertificateRequest CRD
This is the object Certman Operator creates and uses to track certs it creates.

```bash
git clone git@github.com:openshift/certman-operator.git
oc create -f certman-operator/deploy/crds/certman_v1alpha1_certificaterequest_crd.yaml
```
### Make your Namespace
This where all of the Certman Operator objects will live

.If using OpenShift
```bash
oc new-project certman-operator
oc project certman-operator
```

### Setup your ConfigMap
Certman Operator uses a ConfigMap to store options. At the moment, there are 2 items that can be configured using ConfigMap, Let's Encrypt environment, and the default notifcation email.

Example:
```bash
oc create configmap certman-operator \
    --from-literal=default_notification_email_address=foo@bar.com
```
1. default_notification_email_address - Email address to which Let's Encrypt certificate expiry notifications should be sent.

### Certman Operator Secrets
Two Secrets are required. One is the AWS credentials that we'll need for working with Route53.

```bash
oc create secret generic aws-iam-secret --from-literal=aws_access_key_id=XXX --from-literal=aws_secret_access_key=YYYY
```

Another Secret is used to store Let's Encrypt account url and keys. we will use Let's Encrypt staging api if it's an staging account, and use production api if it's an production account.

```bash
 oc create secret generic lets-encrypt-account \
    --from-file=private-key=private-key.pem \
    --from-file=account-url=account.txt
```

### Service Account and RBAC

```bash
oc create -f deploy/service_account.yaml
oc create -f deploy/role.yaml
oc create -f deploy/role_binding.yaml
oc create -f deploy/operator.yaml
```

### Optional: Run Certman Operator
For local developement the easiest way is the use the operator-sdk cli. This will run from the local directory and use local KUBECONFIG environment variable, default `~/.kube/config`

```bash
operator-sdk up local
```
### Optional: Run in cluster
Or build and upload to a cluster
```bash
operator-sdk build <your tag>
docker push <your tag>
```
Edit the operator.yaml with your tag. Then deploy to cluster.
```bash
oc create -f deploy/operator.yaml
```
Happy Developing!