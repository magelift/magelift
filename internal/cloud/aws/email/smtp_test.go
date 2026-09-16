package email

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// Vectors computed from the AWS-documented Python reference
// (smtp_credentials_generate.py) with the documentation's example secret.
// The secret below is AWS's published example, not a real credential.
func TestSmtpPasswordMatchesAWSReferenceVectors(t *testing.T) {
	const exampleSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	cases := []struct {
		region string
		want   string
	}{
		{"eu-west-3", "BDDdpSTzbzUY3SJq/n4Oq7heDuCWbOtogMi5S3AR673+"},
		{"us-east-1", "BLBM/9hSUELfq8Gw+rU1YcBjkOxGbhT2XG763xVLGWL9"},
	}
	for _, tc := range cases {
		got := SmtpPasswordFromSecretAccessKey(exampleSecret, tc.region)
		if got != tc.want {
			t.Errorf("region %s: got %q want %q", tc.region, got, tc.want)
		}
		decoded, err := base64.StdEncoding.DecodeString(got)
		if err != nil {
			t.Fatalf("region %s: password is not base64: %v", tc.region, err)
		}
		if len(decoded) != 33 || decoded[0] != sesSMTPVersion {
			t.Errorf("region %s: decoded length %d version byte %#02x, want 33 bytes starting 0x04", tc.region, len(decoded), decoded[0])
		}
	}
}

func TestSmtpEndpoint(t *testing.T) {
	if got := SmtpEndpoint("eu-west-3"); got != "email-smtp.eu-west-3.amazonaws.com" {
		t.Fatalf("endpoint = %q", got)
	}
	if SmtpPort != 587 {
		t.Fatalf("port = %d", SmtpPort)
	}
}

func TestSmtpCredentialsJSONShape(t *testing.T) {
	encoded, err := SmtpCredentialsJSON("AKIAIOSFODNN7EXAMPLE", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(encoded), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["username"] != "AKIAIOSFODNN7EXAMPLE" || payload["password"] != "secret-value" || len(payload) != 2 {
		t.Fatalf("payload = %s", encoded)
	}
}
