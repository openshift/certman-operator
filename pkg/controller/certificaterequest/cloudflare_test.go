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
)

func TestVerifyDnsResourceRecordUpdate(t *testing.T) {
	type args struct {
		reqLogger logr.Logger
		fqdn      string
		txtValue  string
	}
	nullLogger := logrTesting.NullLogger{}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "Test Max Negative TTL",
			args: args{
				reqLogger: nullLogger,
				fqdn:      "apps.foobar.com",
				txtValue:  "foobar",
			},
			want: maxNegativeCacheTTL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyDnsResourceRecordUpdate(tt.args.reqLogger, tt.args.fqdn, tt.args.txtValue); got != tt.want {
				t.Errorf("VerifyDnsResourceRecordUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchResourceRecordUsingCloudflareDNS(t *testing.T) {
	type args struct {
		reqLogger logr.Logger
		name      string
	}
	nullLogger := logrTesting.NullLogger{}
	tests := []struct {
		name    string
		args    args
		want    *CloudflareResponse
		wantErr bool
	}{
		{
			name: "Test Response",
			args: args{
				reqLogger: nullLogger,
				name:      "apps.foobar.com",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FetchResourceRecordUsingCloudflareDNS(tt.args.reqLogger, tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchResourceRecordUsingCloudflareDNS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
