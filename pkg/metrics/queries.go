package metrics

const (
	DynectCompanyName                      string = "redhat"
	OpenShiftDotCom                        string = "openshift.com"
	OpenShiftDotOrg                        string = "openshift.org"
	OpenShiftAppsDotCom                    string = "openshiftapps.com"
	OkdDotIo                               string = "okd.io"
	RedHatDotCom                           string = "redhat.com"
	RhcloudDotCom                          string = "rhcloud.com"
	AosSreOpsEmailAddr                     string = "aos-sre-ops@redhat.com"
	AosSreEmailAddr                        string = "aos-sre@redhat.com"
	LetsEncryptStagingEndpoint             string = "https://acme-staging-v02.api.letsencrypt.org/directory"
	LetsEncryptProductionEndpoint          string = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptAccountUrlFileName          string = "regr.json"
	LetsEncryptAccountPrivateKeyFileName   string = "private-key.pem"
	KeyFileExtension                       string = "key"
	CrtFileExtension                       string = "crt"
	LetsEncryptCrtFileName                 string = "lets_encrypt_ca.crt"
	LetsEncryptIntermediateCertFileName    string = "lets_encrypt.ca.crt"
	AcmeChallengeSubDomain                 string = "_acme-challenge"
	ACCOUNTS                               string = "accounts"
	PemBlockType                           string = "EC PRIVATE KEY"
	Wildcard                               string = "*"
	DOT                                    string = "."
	CloudflareDnsOverHttpsEndpoint         string = "https://cloudflare-dns.com/dns-query"
	CloudflareRequestContentType           string = "application/dns-json"
	LetsEncryptCertIssuingAuthority        string = "Let's Encrypt Authority X3"
	StagingLetsEncryptCertIssuingAuthority string = "Fake LE Intermediate X1"

	CRT_SH_PG_DB_HOSTNAME string = "crt.sh"
	CRT_SH_PG_DB_PORT     int    = 5432
	CRT_SH_PG_DB_USERNAME string = "guest"
	CRT_SH_PG_DB_NAME     string = "certwatch"

	GET_COUNT_CERTS_ISSUED_BY_LE_SQL string = `
		select count(*) as certs_issued from (SELECT ci.ISSUER_CA_ID,
                      ca.NAME ISSUER_NAME,
                      ci.NAME_VALUE NAME_VALUE,
                      min(c.ID) MIN_CERT_ID,
                      min(ctle.ENTRY_TIMESTAMP) MIN_ENTRY_TIMESTAMP,
                      x509_notBefore(c.CERTIFICATE) NOT_BEFORE,
                      x509_notAfter(c.CERTIFICATE) NOT_AFTER
               FROM ca,
                    ct_log_entry ctle,
                    certificate_identity ci,
                    certificate c
               WHERE ca.ID in (SELECT id FROM ca WHERE lower(ca.NAME) LIKE lower('%Let''s Encrypt%'))
                 AND ci.ISSUER_CA_ID = ca.ID
                 AND c.ID = ctle.CERTIFICATE_ID
                 AND reverse(lower(ci.NAME_VALUE)) LIKE reverse(lower($1))
                 AND ci.CERTIFICATE_ID = c.ID
               GROUP BY c.ID, ci.ISSUER_CA_ID, ISSUER_NAME, NAME_VALUE
              ) AS certs WHERE certs.MIN_ENTRY_TIMESTAMP >= $2
	`

	GET_LIST_CERTS_ISSUED_BY_LE_SQL string = `
		select name_value, not_before, not_after from (SELECT ci.ISSUER_CA_ID,
                      ca.NAME ISSUER_NAME,
                      ci.NAME_VALUE NAME_VALUE,
                      min(c.ID) MIN_CERT_ID,
                      min(ctle.ENTRY_TIMESTAMP) MIN_ENTRY_TIMESTAMP,
                      x509_notBefore(c.CERTIFICATE) NOT_BEFORE,
                      x509_notAfter(c.CERTIFICATE) NOT_AFTER
               FROM ca,
                    ct_log_entry ctle,
                    certificate_identity ci,
                    certificate c
               WHERE ca.ID in (SELECT id FROM ca WHERE lower(ca.NAME) LIKE lower('%Let''s Encrypt%'))
                 AND ci.ISSUER_CA_ID = ca.ID
                 AND c.ID = ctle.CERTIFICATE_ID
                 AND reverse(lower(ci.NAME_VALUE)) LIKE reverse(lower($1))
                 AND ci.CERTIFICATE_ID = c.ID
               GROUP BY c.ID, ci.ISSUER_CA_ID, ISSUER_NAME, NAME_VALUE
              ) AS certs WHERE certs.MIN_ENTRY_TIMESTAMP >= $2
	`

	GET_LIST_CERTS_ISSUED_BY_LE_SQL_EXPIRING_SOON string = `
		select name_value, not_before, not_after from (SELECT ci.ISSUER_CA_ID,
                      ca.NAME ISSUER_NAME,
                      ci.NAME_VALUE NAME_VALUE,
                      min(c.ID) MIN_CERT_ID,
                      min(ctle.ENTRY_TIMESTAMP) MIN_ENTRY_TIMESTAMP,
                      x509_notBefore(c.CERTIFICATE) NOT_BEFORE,
                      x509_notAfter(c.CERTIFICATE) NOT_AFTER
               FROM ca,
                    ct_log_entry ctle,
                    certificate_identity ci,
                    certificate c
               WHERE ca.ID in (SELECT id FROM ca WHERE lower(ca.NAME) LIKE lower('%Let''s Encrypt%'))
                 AND ci.ISSUER_CA_ID = ca.ID
                 AND c.ID = ctle.CERTIFICATE_ID
                 AND reverse(lower(ci.NAME_VALUE)) LIKE reverse(lower($1))
                 AND ci.CERTIFICATE_ID = c.ID
               GROUP BY c.ID, ci.ISSUER_CA_ID, ISSUER_NAME, NAME_VALUE
              ) AS certs WHERE certs.NOT_AFTER >= $2 AND certs.NOT_AFTER <= $3
	`
)
