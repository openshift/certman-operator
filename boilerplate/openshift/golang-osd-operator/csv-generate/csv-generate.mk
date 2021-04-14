
	
.PHONY: staging-hack-csv-build
staging-hack-csv-build: 
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c staging -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -g hack
	
.PHONY: staging-common-csv-build
staging-common-csv-build: 
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c staging -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -g common
	
.PHONY: staging-csv-build
staging-csv-build: staging-hack-csv-build

.PHONY: staging-common-csv-build-and-diff
staging-common-csv-build-and-diff: 
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c staging -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -g common -d

.PHONY: staging-catalog-build
staging-catalog-build: 
	@${CONVENTION_DIR}/csv-generate/catalog-build.sh -o $(OPERATOR_NAME) -c staging -r ${REGISTRY_IMAGE}
	
.PHONY: staging-saas-bundle-push
staging-saas-bundle-push: 
	@${CONVENTION_DIR}/csv-generate/catalog-publish.sh -o $(OPERATOR_NAME) -c staging -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -r ${REGISTRY_IMAGE}
	
.PHONY: staging-catalog-publish
staging-catalog-publish: 
	@${CONVENTION_DIR}/csv-generate/catalog-publish.sh -o $(OPERATOR_NAME) -c staging -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -p -r ${REGISTRY_IMAGE}
	
.PHONY: staging-catalog-build-and-publish
staging-catalog-build-and-publish: 
	@$(MAKE) -s staging-csv-build --no-print-directory
	@$(MAKE) -s staging-catalog-build --no-print-directory
	@$(MAKE) -s staging-catalog-publish --no-print-directory	
	
.PHONY: production-hack-csv-build
production-hack-csv-build: 
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c production -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -g hack
	
.PHONY: production-common-csv-build
production-common-csv-build: 
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c production -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -g common
	
.PHONY: production-csv-build
production-csv-build: production-hack-csv-build

.PHONY: production-common-csv-build-and-diff
production-common-csv-build-and-diff: 
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c production -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -g common -d

.PHONY: production-catalog-build
production-catalog-build: 
	@${CONVENTION_DIR}/csv-generate/catalog-build.sh -o $(OPERATOR_NAME) -c production -r ${REGISTRY_IMAGE}
	
.PHONY: production-saas-bundle-push
production-saas-bundle-push: 
	@${CONVENTION_DIR}/csv-generate/catalog-publish.sh -o $(OPERATOR_NAME) -c production -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -r ${REGISTRY_IMAGE}
	
.PHONY: production-catalog-publish
production-catalog-publish: 
	@${CONVENTION_DIR}/csv-generate/catalog-publish.sh -o $(OPERATOR_NAME) -c production -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -p -r ${REGISTRY_IMAGE}
	
.PHONY: production-catalog-build-and-publish
production-catalog-build-and-publish: 
	@$(MAKE) -s production-csv-build --no-print-directory
	@$(MAKE) -s production-catalog-build --no-print-directory
	@$(MAKE) -s production-catalog-publish --no-print-directory	
