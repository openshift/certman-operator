# Let's Encrypt Certificate Management Operator

## Project Status

**Project status: Alpha**

Not all planned features are completed. As this project is pre-1.0, we do not currently offer strong guarantees around our API stability. The API, spec, status and other user facing objects may change in a non-backward compatible way.

## About

The Certman Operator is used to automate the management and issuance of TLS certificates from [Let's Encrypt](https://letsencrypt.org/) for [OpenShift Dedicated](https://www.openshift.com/products/dedicated/) clusters provisioned via https://cloud.redhat.com/.

It will ensure certificates are valid and up to date, and attempt to renew certificates before expiry.

The Certman Operator watches [ClusterDeployment](https://github.com/openshift/hive/blob/master/config/crds/hive_v1alpha1_clusterdeployment.yaml) resource which is managd by [Hive](https://github.com/openshift/hive). Hive is API driven OpenShift cluster provisioning and management. Once an OpenShift Dedicated cluster has been successfully installed by Hive, Certman Operator will create a `CertificateRequest` resource. Based on details in the `CertificateRequest` resource, Certman Operator will order a new certificate from Let's Encrypt.

When you delete a OpenShift Dedicated cluster (i.e. when a ClusterDeployment is deleted), Certman Operator will revoke all valid certificates issued for the cluster.

### Limitations

* Certman Operator has dependency on [Hive](https://github.com/openshift/hive) custom resources. It is therefore not suitable for certificate management on a cluster. We recommend using either [openshift-acme](https://github.com/tnozicka/openshift-acme) or [cert-manager](https://github.com/jetstack/cert-manager). Certman Operator is ideal for use cases when large number of OpenShift clusters have to be managed centrally.
* Certman Operator currently only supports [DNS Challenge](https://tools.ietf.org/html/rfc8555#section-8.4) through AWS Route53 for. [HTTP Challenge](https://tools.ietf.org/html/rfc8555#section-8.3) is not supported.
* Certman Operator does not support creation of Let's Encrypt account at this time. You must already have Let's Encrypt account and keys that you can provide to the Certman Operator.
* Certman Operator does NOT configure the TLS certificates in an OpenShift cluster. This is managed by [Hive](https://github.com/openshift/hive) using [SyncSet](https://github.com/openshift/hive/blob/master/docs/syncset.md).

## CustomResourceDefinitions

The Certman Operator acts on the following custom resource definitions (CRDs):

* **`CertificateRequest`**, which provides the details needed to request a certificate from Let's Encrypt.

* **`ClusterDeployment`**, which defines a desired OpenShift managed cluster. The Operator ensures at all times that the OpenShift managed cluster has valid certificates for control plane and pre-defined external routes.



