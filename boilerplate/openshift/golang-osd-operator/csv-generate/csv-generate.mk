.PHONY: staging-csv-build
staging-csv-build:
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c staging -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -s $(SUPPLEMENTARY_IMAGE) -e $(SKIP_RANGE_ENABLED)

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
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c production -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -s $(SUPPLEMENTARY_IMAGE) -e $(SKIP_RANGE_ENABLED) -g hack

.PHONY: production-csv-build
production-csv-build:
	@${CONVENTION_DIR}/csv-generate/csv-generate.sh -o $(OPERATOR_NAME) -i $(OPERATOR_IMAGE) -V $(OPERATOR_VERSION) -c production -H $(CURRENT_COMMIT) -n $(COMMIT_NUMBER) -s $(SUPPLEMENTARY_IMAGE) -e $(SKIP_RANGE_ENABLED)

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
