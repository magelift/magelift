package cli

import "testing"

func TestParseBenchmarkMix(t *testing.T) {
	mix, err := parseBenchmarkMix("/=70,/graphql=30")
	if err != nil || len(mix) != 2 || mix[1].Path != "/graphql" || mix[1].Weight != 30 {
		t.Fatalf("mix = %#v, error = %v", mix, err)
	}
	if _, err := parseBenchmarkMix("/bad"); err == nil {
		t.Fatal("invalid benchmark mix accepted")
	}
}

func TestBenchmarkCommandExposesCatalogServiceMetadataFlags(t *testing.T) {
	command := benchmarkCommand(&options{})
	run := command.Commands()[0]
	for _, name := range []string{"database", "search", "queue", "cache"} {
		if run.Flag(name) == nil {
			t.Fatalf("benchmark run is missing --%s", name)
		}
	}
}
