package localmetrics

const (
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
