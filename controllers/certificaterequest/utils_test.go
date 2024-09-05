package certificaterequest

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecretExists(t *testing.T) {
	tests := []struct {
		name          string
		setupClient   func() client.Client
		expectedExist bool
		expectedErr   bool
	}{
		{
			name: "Secret exists",
			setupClient: func() client.Client {
				return fake.NewClientBuilder().
					WithObjects(&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-secret",
							Namespace: "test-namespace",
						},
					}).
					Build()
			},
			expectedExist: true,
			expectedErr:   false,
		},
		{
			name: "Secret does not exist",
			setupClient: func() client.Client {
				return fake.NewClientBuilder().Build()
			},
			expectedExist: false,
			expectedErr:   false,
		},
		{
			name: "Error occurred",
			setupClient: func() client.Client {
				// Create a fake client that will always return an error
				return &errorClient{fake.NewClientBuilder().Build()}
			},
			expectedExist: false,
			expectedErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			exists, err := SecretExists(client, "test-secret", "test-namespace")

			if exists != tt.expectedExist {
				t.Errorf("Expected exists to be %v, got %v", tt.expectedExist, exists)
			}

			if (err != nil) != tt.expectedErr {
				t.Errorf("Expected error to be %v, got %v", tt.expectedErr, err != nil)
			}
		})
	}
}

// errorClient is a wrapper around the fake client that always returns an error on Get
type errorClient struct {
	client.Client
}

func (e *errorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return runtime.NewMissingKindErr("Error for testing")
}
