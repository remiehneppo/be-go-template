package main

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func TestReadPreferenceParsesKnownValues(t *testing.T) {
	tests := map[string]*readpref.ReadPref{
		"primary":            readpref.Primary(),
		"primaryPreferred":   readpref.PrimaryPreferred(),
		"secondary":          readpref.Secondary(),
		"secondaryPreferred": readpref.SecondaryPreferred(),
		"nearest":            readpref.Nearest(),
		"unknown":            readpref.Primary(),
	}
	for input, want := range tests {
		got := readPreference(input)
		if got.Mode() != want.Mode() {
			t.Fatalf("readPreference(%q) = %s, want %s", input, got.Mode(), want.Mode())
		}
	}
}
