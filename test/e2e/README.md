## Locally running e2e test suite
When updating your operator it's beneficial to add e2e tests for new functionality AND ensure existing functionality is not breaking using e2e tests. 
To do this, following steps are recommended

1. Run "make e2e-binary-build"  to make sure e2e tests build 
2. Deploy your new version of operator in a test cluster
3. Run "go install github.com/onsi/ginkgo/ginkgo@latest"
4. Get kubeadmin credentials from your cluster using 

ocm get /api/clusters_mgmt/v1/clusters/(cluster-id)/credentials | jq -r .kubeconfig > /(path-to)/kubeconfig

5. Run test suite using 
 
DISABLE_JUNIT_REPORT=true KUBECONFIG=/(path-to)/kubeconfig  ./(path-to)/bin/ginkgo  --tags=osde2e -v test/e2e
