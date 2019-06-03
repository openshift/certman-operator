package clusterdeployment

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

func (r *ReconcileClusterDeployment) handleDelete(cd *hivev1alpha1.ClusterDeployment, logger logr.Logger) error {

	// get a list of current CertificateRequests
	currentCRs, err := r.getCurrentCertificateRequests(cd, logger)
	if err != nil {
		logger.Error(err, err.Error())
		return err
	}

	// delete the  certificaterequests
	for _, deleteCR := range currentCRs {
		logger.Info(fmt.Sprintf("deleting CertificateRequest resource config %v", deleteCR.Name))
		if err := r.client.Delete(context.TODO(), &deleteCR); err != nil {
			logger.Error(err, "error deleting CertificateRequest", "certrequest", deleteCR.Name)
			return err
		}
	}

	return nil
}
