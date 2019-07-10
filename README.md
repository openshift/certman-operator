# Let's Encrypt Certificate Management Operator

## Project Status

**Project status: Alpha**

Not all planned features are completed. As this project is pre-1.0, we do not currently offer strong guarantees around our API stability. The API, spec, status and other user facing objects may change in a non-backward compatible way.

## About

The Certman Operator is used to automate the management and issuance of TLS certificates from [Let's Encrypt](https://letsencrypt.org/) for [OpenShift Dedicated](https://www.openshift.com/products/dedicated/) clusters provisioned via https://cloud.redhat.com/.

It will ensure certificates are valid and up to date, and attempt to renew certificates before expiry.

The Certman Operator watches [ClusterDeployment](https://github.com/openshift/hive/blob/master/config/crds/hive_v1alpha1_clusterdeployment.yaml) resource which is managd by [Hive](https://github.com/openshift/hive). Hive is API driven OpenShift cluster provisioning and management. Once an OpenShift Dedicated cluster has been successfully installed by Hive, Certman Operator will create a `CertificateRequest` resource. Based on details in the `CertificateRequest` resource, Certman Operator will order a new certificate from Let's Encrypt.

When you delete a OpenShift Dedicated cluster (i.e. when a ClusterDeployment is deleted), Certman Operator will revoke all valid certificates issued for the cluster.

## How the Operator Operates

1. A new OpenShift Dedicated cluster is requested by user from https://cloud.redhat.com.
1. Certman operator monitors the cluster installation progress. Once the cluster status indicates that install was successful, a CertificateRequest resource is created for that cluster.
1. Certman operator will then request new certificates from Let’s Encrypt based on the domains configured for the cluster.
1. Answer Let’s Encrypt “challenge” by adding entries in the cluster’s DNS zone with a TTL of 1 min so that entries can be updated in future and hopefully changes are propagated quickly.
1. Wait for DNS changes to propagate. Verify DNS changes have propagated by using DNS over HTTPS service from Cloudflare. Retry a few times if changes haven’t propagated yet.
1. Once DNS change propagation has been verified, answer the challenge so Let’s Encrypt can verify that you are in control of the domain’s DNS.
1. Let’s Encrypt will issue certificates once challenge has been successfully completed.
1. Certificates are then stored in a Secret on the management cluster. Hive is watching for Secret.
1. Once the Secret has valid certificates for the cluster, Hive will copy the secrets over to the OpenShift Dedicated cluster using SyncSet.
1. Certman operator will reconcile all CertificateRequest every 10 minutes by default. During this reconciliation loop, operator will check for the validity of the existing certificates. As the certificates get closer to 45 days, certificates will be renewed and the Secret will be updated. Renewing certificates early helps us avoid getting email notifications about certificate expiry from Let’s Encrypt.
1. Updates to Secret on certificate renewal will trigger Hive controller’s reconciliation loop which will then copy the updated Secret to the OpenShift Dedicated cluster. OpenShift will detect that Secret has changed and will apply the new certificates to the cluster.
1. When a OpenShift Dedicated cluster is decommissioned, all valid certificates are first revoked and then the Secret is deleted on the management cluster. Hive will then continue with deleting other cluster resources.

## Limitations

* Certman Operator has dependency on [Hive](https://github.com/openshift/hive) custom resources. It is therefore not suitable for certificate management on a cluster. We recommend using either [openshift-acme](https://github.com/tnozicka/openshift-acme) or [cert-manager](https://github.com/jetstack/cert-manager). Certman Operator is ideal for use cases when large number of OpenShift clusters have to be managed centrally.
* Certman Operator currently only supports [DNS Challenge](https://tools.ietf.org/html/rfc8555#section-8.4) through AWS Route53 for. [HTTP Challenge](https://tools.ietf.org/html/rfc8555#section-8.3) is not supported.
* Certman Operator does not support creation of Let's Encrypt account at this time. You must already have Let's Encrypt account and keys that you can provide to the Certman Operator.
* Certman Operator does NOT configure the TLS certificates in an OpenShift cluster. This is managed by [Hive](https://github.com/openshift/hive) using [SyncSet](https://github.com/openshift/hive/blob/master/docs/syncset.md).

## CustomResourceDefinitions

The Certman Operator acts on the following custom resource definitions (CRDs):

* **`CertificateRequest`**, which provides the details needed to request a certificate from Let's Encrypt.

* **`ClusterDeployment`**, which defines a desired OpenShift managed cluster. The Operator ensures at all times that the OpenShift managed cluster has valid certificates for control plane and pre-defined external routes.

## Setup Certman Operator

For local development, you can use either [minishift](https://github.com/minishift/minishift) or [minikube](https://kubernetes.io/docs/setup/minikube/) to develop and run the operator. You will also need to install the [operator-sdk](https://github.com/operator-framework/operator-sdk).

### Certman Operator Configuration

A [ConfigMap](https://docs.openshift.com/container-platform/3.11/dev_guide/configmaps.html) is used to store certman operator configuration. At the moment, there are 2 items that can be configured using ConfigMap.

1. `lets_encrypt_environment` - If set to `staging`, the certman operator will use Let's Encrypt [staging](https://letsencrypt.org/docs/staging-environment/) environment. Set the value to `production` to point to Let's Encrypt production endpoint.
1. `default_notification_email_address` - Email address to which Let's Encrypt certificate expiry notifications should be sent.

```
oc create configmap certman-operator \
    --from-literal=lets_encrypt_environment=staging \
    --from-literal=default_notification_email_address=foo@bar.com
```

### Certman Operator Secrets

[Secret](https://kubernetes.io/docs/concepts/configuration/secret/) is used to store Let's Encrypt account url and keys. A Secret with name `lets-encrypt-account-staging` will be used for Let's Encrypt staging environment and Secret with name `lets-encrypt-account-production` will be used for Let's Encrypt production environment.

```
 oc create secret generic lets-encrypt-account-production \
    --from-file=private-key=prod-private-key.pem \
    --from-file=account-url=prod-account.txt

oc create secret generic lets-encrypt-account-staging \
    --from-file=private-key=stg-private-key.pem \
    --from-file=account-url=stg-account.txt
```

### Custom Resource Definitions (CRDs)

#### Create Hive CRDs

```
git clone git@github.com:openshift/hive.git
oc create -f hive/config/crds
```

#### Create Certman Operator CRDs

```
oc create -f https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/crds/certman_v1alpha1_certificaterequest_crd.yaml
```

### Run Operator From Source

```
operator-sdk up local
```

### Build Operator Image

```
docker login quay.io

operator-sdk build quay.io/tparikh/certman-operator

docker push quay.io/tparikh/certman-operator
```

### Setup & Deploy Operator On OpenShift/Kubernetes Cluster

#### Create & Use OpenShift Project

```
oc new-project certman-operator
oc project certman-operator
```

####  Setup Service Account

```
oc create -f deploy/service_account.yaml
```

#### Setup RBAC

```
oc create -f deploy/role.yaml
oc create -f deploy/role_binding.yaml
```

#### Deploy the Operator
```
oc create -f deploy/operator.yaml
```

## Metrics

`certman_operator_certs_in_last_day_openshift_com` reports how many certs have been issued for Openshift.com in the last 24 hours.

`certman_operator_certs_in_last_day_openshift_apps_com` reports how many certs have been issued for Openshiftapps.com in the last 24 hours.

`certman_operator_certs_in_last_week_openshift_com` reports how many certs have been issued for Openshift.com in the last 7 days.

`certman_operator_certs_in_last_week_openshift_apps_com` reports how many certs have been issued for Openshiftapps.com in the last 7 days.

`certman_operator_duplicate_certs_in_last_week` reports how many certs have had duplication issues.

## License

Certman Operator is licensed under Apache 2.0 license. See the [LICENSE](LICENSE) file for details.