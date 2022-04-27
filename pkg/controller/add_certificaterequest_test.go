package controller

import (
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openshift/certman-operator/pkg/controller/certificaterequest"
	"github.com/openshift/certman-operator/pkg/controller/clusterdeployment"
)

func TestInit(t *testing.T) {
	tests := []struct {
		Name         string
		ExpectedFunc func(mgr manager.Manager) error
	}{
		{
			Name:         "certificaterequest",
			ExpectedFunc: certificaterequest.Add,
		},
		{
			Name:         "clusterdeployment",
			ExpectedFunc: clusterdeployment.Add,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			added := false

			for _, f := range AddToManagerFuncs {

				if reflect.ValueOf(test.ExpectedFunc).Pointer() == reflect.ValueOf(f).Pointer() {
					added = true
				}
			}

			if !added {
				t.Errorf("init() %s: expected func to be in AddToManagerFuncs but it wasn't\n", test.Name)
			}
		})
	}
}
