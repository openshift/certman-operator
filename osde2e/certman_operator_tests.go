// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Certman Operator", Ordered, func() {
	BeforeAll(func(ctx context.Context) {
	})

	It("certificate secret exists under openshift-config namespace", func(ctx context.Context) {
	})

	It("certificate secret should be applied to apiserver object", func(ctx context.Context) {
	})
})
