package main

import "testing"

func TestValidatePart(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "value", value: "eu-north-1", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "newline", value: "marker\nleak", valid: false},
		{name: "nul", value: "marker\x00leak", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validatePart(test.value, "value")
			if (err == nil) != test.valid {
				t.Fatalf("validatePart(%q) error=%v, valid=%t", test.value, err, test.valid)
			}
		})
	}
}
