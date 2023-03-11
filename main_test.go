package main

import (
	"flag"
	"io/ioutil"
	"testing"
)

func TestParseConfig(t *testing.T) {
	// don't Output help message during the test
	flag.CommandLine.SetOutput(ioutil.Discard)

	// The flag.CommandLine.Parse() call fails if there are flags re-defined
	// with the same name. If there are duplicates, parseConfig() will return
	// an error.
	_, err := parseConfig([]string{"test"})
	if err != nil {
		t.Fatal(err)
	}
}
