package util

import "strings"

// DomainPrefix returns the fqdn without tld
func DomainPrefix(fqdn string, tld string) string {
	return strings.TrimSuffix(strings.TrimSuffix(fqdn, tld), ".")
}
