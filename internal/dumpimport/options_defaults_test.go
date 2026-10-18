package dumpimport

import "testing"

func TestOptionsHostDefaultsWhenKubeUnset(t *testing.T) {
	opts := Options{}.withDefaults()
	if opts.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want 127.0.0.1", opts.Host)
	}
	if opts.Port != 3306 {
		t.Fatalf("Port = %d, want 3306", opts.Port)
	}
	if opts.Runner != "" {
		t.Fatalf("Runner = %q, want empty", opts.Runner)
	}
}

func TestOptionsKubeDoesNotDefaultLoopbackHost(t *testing.T) {
	opts := Options{Runner: RunnerKube}.withDefaults()
	if opts.Host != "" {
		t.Fatalf("kube Host = %q, want empty (private IP required explicitly)", opts.Host)
	}
	if opts.Port != 3306 {
		t.Fatalf("Port = %d, want 3306", opts.Port)
	}
}
