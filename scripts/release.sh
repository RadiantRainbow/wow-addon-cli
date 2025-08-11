#!/usr/bin/env bash

make dist
if [[ $? != 0 ]]; then
	echo "ERROR: make dist failed"
	exit $?
fi

echo "INFO: Switching to release account for gh"
gh auth switch -u RadiantRainbow
if [[ $? != 0 ]]; then
	echo "ERROR: Switching to release account for gh"
	exit $?
fi

echo "INFO: Showing last 10 releases"
gh release list -L 10

VERSION="$1"

if [[ -z "$VERSION" ]]; then
	echo "ERROR: version empty"
	exit 1
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "ERROR: version not in correct format"
	exit 1
fi

echo "INFO: using version: $VERSION"

gh release create "$VERSION" --fail-on-no-commits ./dist/*
