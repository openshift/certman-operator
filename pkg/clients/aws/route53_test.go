/*
Copyright 2019 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aws

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("controller_certificaterequest")

func TestNewClient(t *testing.T) {
	t.Run("returns an error if the credentials aren't set", func(t *testing.T) {
		testClient := setUpEmptyTestClient(t)
		reqLogger := log.WithValues("Request.Namespace", testHiveNamespace, "Request.Name", testHiveCertificateRequestName)

		_, actual := NewClient(reqLogger, testClient, testHiveAWSSecretName, testHiveNamespace, testHiveAWSRegion, testHiveClusterDeploymentName)

		if actual == nil {
			t.Error("expected an error when attempting to get missing account secret")
		}
	})

	t.Run("returns a client if the credential is set", func(t *testing.T) {
		testClient := setUpTestClient(t)
		reqLogger := log.WithValues("Request.Namespace", testHiveNamespace, "Request.Name", testHiveCertificateRequestName)

		_, err := NewClient(reqLogger, testClient, testHiveAWSSecretName, testHiveNamespace, testHiveAWSRegion, testHiveClusterDeploymentName)

		if err != nil {
			t.Errorf("unexpected error when creating the client: %q", err)
		}
	})
}

func TestListAllHostedZones(t *testing.T) {
	r53 := &mockRoute53Client{
		zoneCount: 550,
	}

	hostedZones, err := listAllHostedZones(r53, &route53.ListHostedZonesInput{})
	if err != nil {
		t.Fatalf("TestListAllHostedZones(): unexpected error: %s\n", err)
	}

	replyZoneCount := len(hostedZones)
	if replyZoneCount != r53.zoneCount {
		t.Errorf("TestListAllHostedZones(): got %d zones, expected %d\n", replyZoneCount, r53.zoneCount)
	}
}

// helpers
var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveCertificateRequestName = "clustername-1313-0-primary-cert-bundle"
var testHiveCertSecretName = "primary-cert-bundle-secret"
var testHiveACMEDomain = "not.a.valid.tld"
var testHiveAWSSecretName = "aws"
var testHiveAWSRegion = "not-relevant-1"
var testHiveClusterDeploymentName = "test-cluster"

var certRequest = &certmanv1alpha1.CertificateRequest{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveCertificateRequestName,
	},
	Spec: certmanv1alpha1.CertificateRequestSpec{
		ACMEDNSDomain: testHiveACMEDomain,
		CertificateSecret: v1.ObjectReference{
			Kind:      "Secret",
			Namespace: testHiveNamespace,
			Name:      testHiveCertSecretName,
		},
		Platform: certRequestPlatform,
		DnsNames: []string{
			"api.gibberish.goes.here",
		},
		Email:             "devnull@donot.route",
		ReissueBeforeDays: 10000,
	},
}

var certRequestPlatform = certmanv1alpha1.Platform{
	AWS: &certmanv1alpha1.AWSPlatformSecrets{
		Credentials: v1.LocalObjectReference{
			Name: testHiveAWSSecretName,
		},
		Region: testHiveAWSRegion,
	},
}

var awsSecret = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveAWSSecretName,
	},
	Data: map[string][]byte{
		"aws_access_key_id":     {},
		"aws_secret_access_key": {},
	},
}

var testClusterDeployment = &hivev1.ClusterDeployment{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveClusterDeploymentName,
	},
}

func setUpEmptyTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

	// aws is not an existing secret
	objects := []runtime.Object{certRequest}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}

func setUpTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)
	s.AddKnownTypes(hivev1.SchemeGroupVersion, testClusterDeployment)

	// aws is not an existing secret
	objects := []runtime.Object{certRequest, awsSecret, testClusterDeployment}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}

// set up a mock route53 client for testing
type mockRoute53Client struct {
	route53iface.Route53API
	zoneCount int
}

func (m *mockRoute53Client) ListHostedZones(lhzi *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	hostedZones := []*route53.HostedZone{}

	// figure out the start zone for the request
	var startI int
	if lhzi.Marker == nil {
		startI = 0
	} else {
		startI, _ = strconv.Atoi(strings.TrimLeft(*lhzi.Marker, "id"))
	}

	var maxItems int
	if lhzi.MaxItems == nil {
		maxItems = 100
	} else {
		maxItems, _ = strconv.Atoi(*lhzi.MaxItems)
	}

	// figure out the end zone for the request
	var endI int
	if startI+maxItems < m.zoneCount {
		endI = startI + maxItems
	} else {
		endI = m.zoneCount
	}

	// generate fake zones between the start marker and either the maxitems or the end of the zonecount
	var nextMarker string
	for i := startI; i < endI; i++ {
		callerRef := fmt.Sprintf("zone%d", i)
		id := fmt.Sprintf("id%d", i)
		name := fmt.Sprintf("name%d", i)

		hz := route53.HostedZone{
			CallerReference: &callerRef,
			Id:              &id,
			Name:            &name,
		}

		hostedZones = append(hostedZones, &hz)

		nextMarker = fmt.Sprintf("id%d", i+1)
	}

	isTruncated := endI < m.zoneCount

	output := &route53.ListHostedZonesOutput{
		HostedZones: hostedZones,
		IsTruncated: &isTruncated,
		Marker:      lhzi.Marker,
		MaxItems:    lhzi.MaxItems,
	}

	if isTruncated {
		output.NextMarker = &nextMarker
	}

	return output, nil
}
