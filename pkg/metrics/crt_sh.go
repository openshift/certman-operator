package metrics

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/olekukonko/tablewriter"
)

// GetListOfCertsExpiringSoon returns a list of certs expiring within the specified number of days
func GetListOfCertsExpiringSoon(domain string, durationDays int) [][]string {

	data := [][]string{}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "NOT BEFORE", "NOT AFTER"})
	table.SetRowLine(true) // Enable row line

	db, err := sql.Open("postgres", getPsqlInfo())

	if err != nil {
		log.Fatalf("%v\n", err)
	}

	defer db.Close()

	today := time.Now().UTC()

	futureDate := time.Now().UTC().AddDate(0, 0, durationDays)

	certDuration := fmt.Sprintf("%d-%02d-%02d 00:00:00.000000", today.Year(), today.Month(), today.Day())

	futureDuration := fmt.Sprintf("%d-%02d-%02d 00:00:00.000000", futureDate.Year(), futureDate.Month(), futureDate.Day())

	rows, err := db.Query(GET_LIST_CERTS_ISSUED_BY_LE_SQL_EXPIRING_SOON, "%."+domain, certDuration, futureDuration)

	if err != nil {
		log.Fatalf("%v\n", err)
	}

	defer rows.Close()

	for rows.Next() {
		var name string
		var notBefore string
		var notAfter string
		var row []string

		err = rows.Scan(&name, &notBefore, &notAfter)

		if err != nil {
			log.Fatalf("%v\n", err)
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

	for _, v := range data {
		table.Append(v)
	}

	log.Printf("\nFollowing certificates issued by Let's Encrypt for %s domain are expiring in %d days\n", domain, durationDays)

	table.Render()

	return data
}

// GetListOfCertsIssued returns a list of certs issued for a given domain since durationDays number of days ago
func GetListOfCertsIssued(domain string, durationDays int) [][]string {

	data := [][]string{}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "NOT BEFORE", "NOT AFTER"})
	table.SetRowLine(true) // Enable row line

	db, err := sql.Open("postgres", getPsqlInfo())

	if err != nil {
		log.Fatalf("%v\n", err)
	}

	defer db.Close()

	t := time.Now().UTC().AddDate(0, 0, durationDays*-1)

	certDuration := fmt.Sprintf("%d-%02d-%02d 00:00:00.000000", t.Year(), t.Month(), t.Day())

	rows, err := db.Query(GET_LIST_CERTS_ISSUED_BY_LE_SQL, "%."+domain, certDuration)

	if err != nil {
		log.Fatalf("%v\n", err)
	}

	defer rows.Close()

	for rows.Next() {
		var name string
		var notBefore string
		var notAfter string
		var row []string

		err = rows.Scan(&name, &notBefore, &notAfter)

		if err != nil {
			log.Fatalf("%v\n", err)
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

	for _, v := range data {
		table.Append(v)
	}

	log.Printf("\nFollowing certificates have been issued by Let's Encrypt for %s domain in last %d days\n", domain, durationDays)

	table.Render()

	return data
}

// GetCountOfCertsIssued returns the number of certs issued for a given domain in the last durationDays number of days
func GetCountOfCertsIssued(domain string, durationDays int) int {

	db, err := sql.Open("postgres", getPsqlInfo())

	if err != nil {
		log.Fatalf("%v\n", err)
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

	log.Printf("\n%d certificates have been issued by Let's Encrypt for %s domain in last %d days.\n", numCertsIssued, domain, durationDays)

	return numCertsIssued
}

func getPsqlInfo() string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", CRT_SH_PG_DB_HOSTNAME, CRT_SH_PG_DB_PORT, CRT_SH_PG_DB_USERNAME, CRT_SH_PG_DB_NAME)
}
