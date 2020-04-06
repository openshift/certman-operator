/*
Copyright 2020 Red Hat, Inc.

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

import "testing"

func TestReconcile(t *testing.T) {
	t.Run("errors if lets-encrypt account secret is unset", func(t *testing.T) {
		testClient := setUpEmptyTestClient(t)

		_, err := rcrReconcile(t, testClient)

		if err == nil {
			t.Error("expected an error when reconciling without a Let's Encrypt account secret")
		}
	})

	t.Run("errors if AWS account secret is unset", func(t *testing.T) {
		testClient := setUpTestClient(t, "lets-encrypt-account-staging", false)

		_, err := rcrReconcile(t, testClient)

		if err == nil {
			t.Error("expected an error when reconciling without an AWS account secret")
		}
	})
}
