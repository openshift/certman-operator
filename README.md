# Certman Operator

## About
The Certman Operator is used to automate the provisioning and management of TLS certificates from [Let's Encrypt](https://letsencrypt.org/) for [OpenShift Dedicated](https://www.openshift.com/products/dedicated/) clusters provisioned via https://cloud.redhat.com/.

At a high level, Certman Operator is responsible for:

* Provisioning Certificates after a clusters successfull installation. 
* Reissuing Certificates prior to their expiry.
* Revoking Certificates upon cluster decomissioning.

## Dependencies

**GO:** 1.11

**Operator-SDK:** 0.5.0

**Hive** v1
Certman Operator is currently dependent on [Hive](https://github.com/openshift/hive). Hive is an API driven OpenShift cluster providing OpenShift Dedicated provisioning and management.

Specifically, Hive provides a [namespace scoped](https://github.com/openshift/hive/blob/master/config/crds/hive_v1alpha1_clusterdeployment.yaml#L34) [CustomResourceDefinition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#customresourcedefinitions) called [ClusterDeployment](https://github.com/openshift/hive/blob/master/config/crds/hive_v1alpha1_clusterdeployment.yaml). Certman watches the `Installed` spec of CRD and will attempt to [provision certificates](https://github.com/openshift/certman-operator/blob/master/pkg/controller/clusterdeployment/clusterdeployment_controller.go#L134) for the cluster once this field returns `true`. Hive is also responsible for the deployment of the certificates to the cluster via [syncsets](https://github.com/openshift/hive/blob/master/docs/syncset.md).

Only Hive v1 will work with this release.

## How the Certman Operator works

1. A new OpenShift Dedicated cluster is requested by from https://cloud.redhat.com.
1. Certman's `Reconcile` function watches the `Installed` field of the ClusterDeployment CRD (as explained above). Once the `Installed` field becomes `true`, a [CertificateRequest](https://github.com/openshift/certman-operator/blob/master/deploy/crds/certman_v1alpha1_certificaterequest_crd.yaml) resource is created for that cluster.
1. Certman operator will then request new certificates from Let’s Encrypt based on the populated spec fields of the CertificateRequest CRD.
1. To prove ownership of the domain, Certman will attempt to answer the Let’s Encrypt [DNS-01 challenge](https://letsencrypt.org/docs/challenge-types/) by publishing the `_acme-challenge` subdomain in the cluster’s DNS zone with a TTL of 1 min.
1. Wait for propagation of the record and then verify the existance of the challenge subdomain by using DNS over HTTPS service from Cloudflare. Certman will retry verification up to 5 times before erroring.
1. Once the challenge subdomain record has been verified, Let’s Encrypt can verify that you are in control of the domain’s DNS.
1. Let’s Encrypt will issue certificates once challenge has been successfully completed. Certman will then delete the challenge subdomain as it is no longer required. 
1. Certificates are then stored in a secret on the management cluster. Hive watchs for this secret.
1. Once the secret contains a valid certificates for the cluster, Hive will sync the secrets over to the OpenShift Dedicated cluster using a [SyncSet](https://github.com/openshift/hive/blob/master/docs/syncset.md).
1. Certman operator will reconcile all CertificateRequest every 10 minutes by default. During this reconciliation loop, certman will check for the validity of the existing certificates. As the certificates expiry nears 45 days, they will be reissued and the secret will be updated. Reissuing certificates this early avoids getting email notifications about certificate expiry from Let’s Encrypt.
1. Updates to secrets on certificate reissuance will trigger Hive controller’s reconciliation loop which will force a syncset of the new secret to the OpenShift Dedicated cluster. OpenShift will detect that secret has changed and will apply the new certificates to the cluster.
1. When a OpenShift Dedicated cluster is decommissioned, all valid certificates are first revoked and then the secret is deleted on the management cluster. Hive will then continue deleting other cluster resources.

## Limitations

* As described above in dependencies, Certman Operator requires [Hive](https://github.com/openshift/hive) for custom resources and actual deploymen of certificates. It is therefore **not** a suitable "out-of-the-box" solution for Let's Encrypt certificate management. For this, we recommend using either [openshift-acme](https://github.com/tnozicka/openshift-acme) or [cert-manager](https://github.com/jetstack/cert-manager). Certman Operator is ideal for use cases when large number of OpenShift clusters have to be managed centrally.
* Certman Operator currently only supports [DNS Challenges](https://tools.ietf.org/html/rfc8555#section-8.4) through AWS Route53. There are plans for GCP support. [HTTP Challenges](https://tools.ietf.org/html/rfc8555#section-8.3) is not supported.
* Certman Operator does not support creation of Let's Encrypt account at this time. You must already have Let's Encrypt account and keys that you can provide to the Certman Operator.
* Certman Operator does NOT configure the TLS certificates in an OpenShift cluster. This is managed by [Hive](https://github.com/openshift/hive) using [SyncSet](https://github.com/openshift/hive/blob/master/docs/syncset.md).

## CustomResourceDefinitions

The Certman Operator relies on the following custom resource definitions (CRDs):

* **`CertificateRequest`**, which provides the details needed to request a certificate from Let's Encrypt.

* **`ClusterDeployment`**, which defines a targeted OpenShift managed cluster. The Operator ensures at all times that the OpenShift managed cluster has valid certificates for control plane and pre-defined external routes.

## Setup Certman Operator

For local development, you can use either [minishift](https://github.com/minishift/minishift) or [minikube](https://kubernetes.io/docs/setup/minikube/) to develop and run the operator. You will also need to install the [operator-sdk](https://github.com/operator-framework/operator-sdk).

### Local development testing

The script `hack/test/local_test.sh` can be used to automate local testing by creating a minikube cluster and deploying certman-operator and its dependencies.

### Certman Operator Configuration

A [ConfigMap](https://docs.openshift.com/container-platform/3.11/dev_guide/configmaps.html) is used to store certman operator configuration. At the moment, there are 2 items that can be configured using ConfigMap.

1. `default_notification_email_address` - Email address to which Let's Encrypt certificate expiry notifications should be sent.

```
oc create configmap certman-operator \
    --from-literal=default_notification_email_address=foo@bar.com
```

### Certman Operator Secrets

[Secret](https://kubernetes.io/docs/concepts/configuration/secret/) is used to store Let's Encrypt account url and keys. We will use Let's Encrypt staging environment if it's an staging account, and use production environment if it's an production account.

```
 oc create secret generic lets-encrypt-account \
    --from-file=private-key=private-key.pem \
    --from-file=account-url=account.txt
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

## Additional record for control plane certificate

Certman Operator always creates a certificate for the control plane for the clusters Hive builds. By passing a string into the pod as an environment variable named `EXTRA_RECORD` Certman Operator can add an additional record to the SAN of the certificate for the API servers. This string should be the short hostname without the domain. The record will use the same domain as the rest of the cluster for this new record.
Example
```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: certman-operator
spec:
  template:
    spec:
    ...
      env:
      - name: EXTRA_RECORD
        value: "myapi"
```
The example will add `myapi.<clustername>.<clusterdomain>` to the certificate of the control plane.
## License

Certman Operator is licensed under Apache 2.0 license. See the [LICENSE](LICENSE) file for details.
