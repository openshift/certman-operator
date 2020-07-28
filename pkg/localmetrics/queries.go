package localmetrics

const (
	CRT_SH_PG_DB_HOSTNAME string = "crt.sh"
	CRT_SH_PG_DB_PORT     int    = 5432
	CRT_SH_PG_DB_USERNAME string = "guest"
	CRT_SH_PG_DB_NAME     string = "certwatch"

	GET_COUNT_CERTS_ISSUED_BY_LE_SQL string = `
        SELECT COUNT(*) AS CERTS_ISSUED
        FROM (
            WITH ci AS (
                SELECT min(sub.CERTIFICATE_ID) ID,
                       min(sub.ISSUER_CA_ID) ISSUER_CA_ID,
                       array_agg(DISTINCT sub.NAME_VALUE) NAME_VALUES,
                       x509_subjectName(sub.CERTIFICATE) SUBJECT_NAME,
                       x509_notBefore(sub.CERTIFICATE) NOT_BEFORE,
                       x509_notAfter(sub.CERTIFICATE) NOT_AFTER
                    FROM (SELECT *
                              FROM certificate_and_identities cai
                              WHERE plainto_tsquery('certwatch', $1) @@ identities(cai.CERTIFICATE)
                                  AND cai.NAME_VALUE ILIKE ('%' || $1 || '%')
                                  AND coalesce(x509_notAfter(cai.CERTIFICATE), 'infinity'::timestamp) >= date_trunc('year', now() AT TIME ZONE 'UTC')
                                  AND x509_notAfter(cai.CERTIFICATE) >= now() AT TIME ZONE 'UTC'
                                  AND NOT EXISTS (
                                      SELECT 1
                                          FROM certificate c2
                                          WHERE x509_serialNumber(c2.CERTIFICATE) = x509_serialNumber(cai.CERTIFICATE)
                                              AND c2.ISSUER_CA_ID = cai.ISSUER_CA_ID
                                              AND c2.ID < cai.CERTIFICATE_ID
                                              AND x509_tbscert_strip_ct_ext(c2.CERTIFICATE) = x509_tbscert_strip_ct_ext(cai.CERTIFICATE)
                                          LIMIT 1
                                  )
                         ) sub
                    GROUP BY sub.CERTIFICATE
            )
            SELECT ci.ISSUER_CA_ID,
                   ca.NAME ISSUER_NAME,
                   array_to_string(ci.NAME_VALUES, chr(10)) NAME_VALUE,
                   ci.ID ID,
                   le.ENTRY_TIMESTAMP
                FROM ci
                        LEFT JOIN LATERAL (
                            SELECT min(ctle.ENTRY_TIMESTAMP) ENTRY_TIMESTAMP
                                FROM ct_log_entry ctle
                                WHERE ctle.CERTIFICATE_ID = ci.ID
                        ) le ON TRUE,
                     ca
                WHERE ca.ID in (SELECT id FROM ca WHERE lower(ca.NAME) LIKE lower('%Let''s Encrypt%')) AND ci.ISSUER_CA_ID = ca.ID AND le.ENTRY_TIMESTAMP >= $2
                GROUP BY ci.ID, ci.ISSUER_CA_ID, ca.NAME, le.ENTRY_TIMESTAMP, ci.NAME_VALUES
        ) AS Z;
	`
)
