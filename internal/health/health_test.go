package health

import "testing"

func TestSummarizeUsesWorstStatus(t *testing.T) {
	tests := []struct {
		checks []Check
		want   Status
	}{
		{checks: []Check{{Status: StatusHealthy}}, want: StatusHealthy},
		{checks: []Check{{Status: StatusHealthy}, {Status: StatusUnavailable}}, want: StatusUnavailable},
		{checks: []Check{{Status: StatusUnavailable}, {Status: StatusUnhealthy}}, want: StatusUnhealthy},
	}
	for _, test := range tests {
		if got := Summarize(test.checks); got != test.want {
			t.Fatalf("Summarize() = %q, want %q", got, test.want)
		}
	}
}
