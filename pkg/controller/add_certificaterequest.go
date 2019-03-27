package controller

import (
	"github.com/openshift/certman-operator/pkg/controller/certificaterequest"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, certificaterequest.Add)
}
