package platform

import "testing"

func TestNormalizeTunnelTarget(t *testing.T) {
	for _, test := range []struct {
		input, want string
	}{
		{"web", TunnelTargetApp},
		{"database", TunnelTargetDatabase},
		{"rabbitmq-management", TunnelTargetQueueUI},
		{"opensearch", TunnelTargetSearch},
	} {
		got, err := NormalizeTunnelTarget(test.input)
		if err != nil {
			t.Fatalf("NormalizeTunnelTarget(%q): %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("NormalizeTunnelTarget(%q) = %q, want %q", test.input, got, test.want)
		}
	}
	if _, err := NormalizeTunnelTarget("unknown"); err == nil {
		t.Fatal("unknown tunnel target should fail")
	}
}

func TestValidateTunnelQueryRejectsInvalidPorts(t *testing.T) {
	for _, query := range []TunnelQuery{
		{Target: TunnelTargetApp, LocalPort: 0, RemotePort: 80},
		{Target: TunnelTargetApp, LocalPort: 8080, RemotePort: 65536},
		{Target: TunnelTargetApp, LocalPort: 8080, RemotePort: 0},
	} {
		if err := ValidateTunnelQuery(query); err == nil {
			t.Fatalf("ValidateTunnelQuery(%#v) should fail", query)
		}
	}
}

func TestDefaultTunnelPorts(t *testing.T) {
	if got := DefaultTunnelLocalPort(TunnelTargetApp); got != 8080 {
		t.Fatalf("app local port = %d, want 8080", got)
	}
	if got := DefaultTunnelRemotePort(TunnelTargetQueueUI); got != 15672 {
		t.Fatalf("queue UI remote port = %d, want 15672", got)
	}
}
