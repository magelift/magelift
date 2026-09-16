package database

import "testing"

func TestDatabaseVersionForMagento(t *testing.T) {
	for _, test := range []struct {
		version string
		want    string
	}{
		{version: "2.4.6-p15", want: DatabaseVersionMySQL80},
		{version: "2.4.7-p10", want: DatabaseVersionMySQL84},
		{version: "2.4.8-p5", want: DatabaseVersionMySQL84},
		{version: "2.4.9", want: DatabaseVersionMySQL84},
	} {
		t.Run(test.version, func(t *testing.T) {
			got, err := DatabaseVersionForMagento(test.version)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("database version = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDatabaseVersionForMagentoRejectsUnknownRelease(t *testing.T) {
	if _, err := DatabaseVersionForMagento("2.4.5-p17"); err == nil {
		t.Fatal("unknown release was accepted")
	}
}
