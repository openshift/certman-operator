//go:build fips_enabled
// +build fips_enabled

//go:debug fips140=on

// BOILERPLATE GENERATED -- DO NOT EDIT
// Run 'make ensure-fips' to regenerate

package main

import "crypto/fips140"

func init() {
	// Assert that FIPS mode is enabled via GODEBUG=fips140=on (or =only)
	if !fips140.Enabled() {
		panic("CRITICAL: Application startup aborted. FIPS 140 mode is NOT enabled!")
	}
}
