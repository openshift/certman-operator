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

package clusterdeployment

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// handleDelete accepts a ClusterDeployment arg from which is lists out all related CertificateRequests.
// These are then iterated through for deletion. If an error occurs, it is returned.
func (r *ClusterDeploymentReconciler) handleDelete(cd *hivev1.ClusterDeployment, logger logr.Logger) error {

	// get a list of current CertificateRequests
	currentCRs, err := r.getCurrentCertificateRequests(cd, logger)
	if err != nil {
		logger.Error(err, err.Error())
		return err
	}

	// delete the certificaterequests
	for _, deleteCR := range currentCRs {
		deleteCR := deleteCR
		logger.Info(fmt.Sprintf("deleting CertificateRequest resource config %v", deleteCR.Name))
		if err := r.Client.Delete(context.TODO(), &deleteCR); err != nil {
			logger.Error(err, "error deleting CertificateRequest", "certrequest", deleteCR.Name)
			return err
		}
	}

	return nil
}
