package localmetrics

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/common/log"
)

// GetListOfCertsExpiringSoon returns a list of certs expiring within the specified number of days
func GetListOfCertsExpiringSoon(domain string, durationDays int) [][]string {

	data := [][]string{}
	db, err := sql.Open("postgres", getPsqlInfo())

	if err != nil {
		log.Error(err, "Failed to establish connection with crt.sh database")
	}

	defer db.Close()

	today := time.Now().UTC()

	futureDate := time.Now().UTC().AddDate(0, 0, durationDays)

	certDuration := fmt.Sprintf("%d-%02d-%02d 00:00:00.000000", today.Year(), today.Month(), today.Day())

	futureDuration := fmt.Sprintf("%d-%02d-%02d 00:00:00.000000", futureDate.Year(), futureDate.Month(), futureDate.Day())

	rows, err := db.Query(GET_LIST_CERTS_ISSUED_BY_LE_SQL_EXPIRING_SOON, "%."+domain, certDuration, futureDuration)

	if err != nil {
		log.Error(err, "Failed to recieve data from crt.sh database")
	}

	defer rows.Close()

	for rows.Next() {
		var name string
		var notBefore string
		var notAfter string
		var row []string

		err = rows.Scan(&name, &notBefore, &notAfter)

		if err != nil {
			log.Error(err, "Error reading table")
		}

		row = append(row, name)
		row = append(row, notBefore)
		row = append(row, notAfter)

		data = append(data, row)
	}

	err = rows.Err()

	if err != nil {
		panic(err)
	}

	return data
}

// GetListOfCertsIssued returns a list of certs issued for a given domain since durationDays number of days ago
func GetListOfCertsIssued(domain string, durationDays int) [][]string {

	data := [][]string{}

	db, err := sql.Open("postgres", getPsqlInfo())

	if err != nil {
		log.Error(err, "Failed to establish connection with crt.sh database")
	}

	defer db.Close()

	t := time.Now().UTC().AddDate(0, 0, durationDays*-1)

	certDuration := fmt.Sprintf("%d-%02d-%02d 00:00:00.000000", t.Year(), t.Month(), t.Day())

	rows, err := db.Query(GET_LIST_CERTS_ISSUED_BY_LE_SQL, "%."+domain, certDuration)

	if err != nil {
		log.Error(err, "Failed to recieve data from crt.sh database")
	}

	defer rows.Close()

	for rows.Next() {
		var name string
		var notBefore string
		var notAfter string
		var row []string

		err = rows.Scan(&name, &notBefore, &notAfter)

		if err != nil {
			log.Error(err, "Error reading table")
		}

		row = append(row, name)
		row = append(row, notBefore)
		row = append(row, notAfter)

		data = append(data, row)
	}

	err = rows.Err()

	if err != nil {
		panic(err)
	}

	return data
}

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

	var numCertsIssued int

	err = row.Scan(&numCertsIssued)

	if err != nil {
		panic(err)
	}

	return numCertsIssued
}

func getPsqlInfo() string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", CRT_SH_PG_DB_HOSTNAME, CRT_SH_PG_DB_PORT, CRT_SH_PG_DB_USERNAME, CRT_SH_PG_DB_NAME)
}
