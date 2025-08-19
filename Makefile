KONFLUX_BUILDS=true
FIPS_ENABLED=true
include boilerplate/generated-includes.mk

SHELL := /usr/bin/env bash

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

.PHONY: go-mac-build
# This script is only for MacOS Developer
# Prerequisite: x86_64-unknown-linux-gnu-gcc
# brew tap SergioBenitez/osxct
# brew install x86_64-unknown-linux-gnu
go-mac-build:
	CC=x86_64-unknown-linux-gnu-gcc CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make go-build
