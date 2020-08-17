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

package certificaterequest

import (
	"testing"

	"github.com/go-logr/logr"
	logrTesting "github.com/go-logr/logr/testing"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	cClient "github.com/openshift/certman-operator/pkg/clients"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcileCertificateRequest_IncrementDNSVerifyAttempts(t *testing.T) {
	type fields struct {
		client        client.Client
		scheme        *runtime.Scheme
		clientBuilder func(kubeClient client.Client, platfromSecret certmanv1alpha1.Platform, namespace string) (cClient.Client, error)
	}
	type args struct {
		reqLogger logr.Logger
		cr        *certmanv1alpha1.CertificateRequest
	}
	nullLogger := logrTesting.NullLogger{}
	testClient := setUpEmptyTestClient(t)
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "Has annotation",
			fields: fields{
				client: testClient,
			},
			args: args{
				reqLogger: nullLogger,
				cr: &certmanv1alpha1.CertificateRequest{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							dnsCheckAttemptsAnnotation: "3",
						},
					},
				},
			},
			want:    4,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileCertificateRequest{
				client:        tt.fields.client,
				scheme:        tt.fields.scheme,
				clientBuilder: tt.fields.clientBuilder,
			}
			if err := r.IncrementDNSVerifyAttempts(tt.args.reqLogger, tt.args.cr); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileCertificateRequest.IncrementDNSVerifyAttempts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := GetDNSVerifyAttempts(tt.args.cr); got != tt.want {
				t.Errorf("ReconcileCertificateRequest.IncrementDNSVerifyAttempts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDNSVerifyAttempts(t *testing.T) {
	type args struct {
		cr *certmanv1alpha1.CertificateRequest
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "Missing Annotation",
			args: args{
				cr: &certmanv1alpha1.CertificateRequest{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			want: 0,
		},
		{
			name: "Correct Annotation",
			args: args{
				cr: &certmanv1alpha1.CertificateRequest{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							dnsCheckAttemptsAnnotation: "7",
						},
					},
				},
			},
			want: 7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetDNSVerifyAttempts(tt.args.cr); got != tt.want {
				t.Errorf("GetDNSVerifyAttempts() = %v, want %v", got, tt.want)
			}
		})
	}
}
