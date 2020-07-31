package localmetrics

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/common/log"
)

// GetCountOfCertsIssued returns the number of certs issued for a given domain in the last durationDays number of days
func GetCountOfCertsIssued(domain string, durationDays int) int {

	db, err := sql.Open("postgres", getPsqlInfo())

	if err != nil {
		log.Error(err, "Failed to establish connection with crt.sh database")
	}

	defer db.Close()

	t := time.Now().UTC().AddDate(0, 0, durationDays*-1)

	certDuration := fmt.Sprintf("%d-%02d-%02d 00:00:00.000000", t.Year(), t.Month(), t.Day())

	row := db.QueryRow(GET_COUNT_CERTS_ISSUED_BY_LE_SQL, "%."+domain, certDuration)

	numCertsIssued := 0

	err = row.Scan(&numCertsIssued)

	if err != nil {
		log.Error(err, "Error while parsing crt.sh data")
	}

	return numCertsIssued
}

func getPsqlInfo() string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable binary_parameters=yes", CRT_SH_PG_DB_HOSTNAME, CRT_SH_PG_DB_PORT, CRT_SH_PG_DB_USERNAME, CRT_SH_PG_DB_NAME)
}
