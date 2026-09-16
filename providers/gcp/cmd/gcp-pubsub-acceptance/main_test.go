package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestRunValidatesArgumentsBeforeCreatingClients(t *testing.T) {
	err := run(context.Background(), nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "project is required") {
		t.Fatalf("missing project error = %v", err)
	}
}

func TestValidatePart(t *testing.T) {
	for _, test := range []struct {
		name        string
		value       string
		rejectSlash bool
		wantError   bool
	}{
		{name: "valid", value: "magelift-live-abc"},
		{name: "query", value: "magelift-live-abc?x=1", wantError: true},
		{name: "newline", value: "magelift-live-abc\n", wantError: true},
		{name: "subscription slash", value: "projects/demo/subscriptions/orders", rejectSlash: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validatePart(test.value, "value", test.rejectSlash)
			if (err != nil) != test.wantError {
				t.Fatalf("validatePart(%q) error = %v, wantError=%v", test.value, err, test.wantError)
			}
		})
	}
}

func TestParsePubSubRecoveryDestination(t *testing.T) {
	destination, err := parsePubSubRecoveryDestination("isolated")
	if err != nil || destination != "same-region-isolated" {
		t.Fatalf("isolated destination = %q err=%v", destination, err)
	}
	destination, err = parsePubSubRecoveryDestination("")
	if err != nil || destination != "same-region" {
		t.Fatalf("default destination = %q err=%v", destination, err)
	}
	if _, err := parsePubSubRecoveryDestination("alternate-region"); err == nil {
		t.Fatal("alternate-region destination was accepted")
	}
}
