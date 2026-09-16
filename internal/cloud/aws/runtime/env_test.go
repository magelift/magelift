package runtime

import "testing"

func TestAppendSmtpEnvironmentWiresMagentoSMTP(t *testing.T) {
	got := appendSmtpEnvironment(nil, "email-smtp.eu-west-3.amazonaws.com", "AKIAIOSFODNN7EXAMPLE", "shop@example.invalid")
	want := map[string]string{
		"CONFIG__DEFAULT__SYSTEM__SMTP__TRANSPORT":           "smtp",
		"CONFIG__DEFAULT__SYSTEM__SMTP__HOST":                "email-smtp.eu-west-3.amazonaws.com",
		"CONFIG__DEFAULT__SYSTEM__SMTP__PORT":                "587",
		"CONFIG__DEFAULT__SYSTEM__SMTP__USERNAME":            "AKIAIOSFODNN7EXAMPLE",
		"CONFIG__DEFAULT__SYSTEM__SMTP__AUTH":                "LOGIN",
		"CONFIG__DEFAULT__SYSTEM__SMTP__SSL":                 "tls",
		"CONFIG__DEFAULT__SYSTEM__SMTP__DISABLE":             "0",
		"CONFIG__DEFAULT__TRANS_EMAIL__IDENT_GENERAL__EMAIL": "shop@example.invalid",
	}
	if len(got) != len(want) {
		t.Fatalf("bindings = %d, want %d", len(got), len(want))
	}
	for _, binding := range got {
		if want[binding.Name] != binding.Value {
			t.Errorf("%s = %q", binding.Name, binding.Value)
		}
	}
}

func TestAppendSmtpEnvironmentSkipsUnmanaged(t *testing.T) {
	base := []containerEnvironment{{Name: "MAGELIFT_QUEUE_MODE", Value: "rabbitmq"}}
	if got := appendSmtpEnvironment(base, "", "", ""); len(got) != len(base) {
		t.Fatalf("unmanaged SMTP appended %d bindings", len(got)-len(base))
	}
}

func TestAppendSmtpSecretReferencesPasswordJSONKey(t *testing.T) {
	base := []SecretReference{{Name: "MAGENTO_DC_CRYPT__KEY", ARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:crypt"}}
	got := appendSmtpSecret(base, "arn:aws:ssm:eu-west-3:123456789012:parameter/magelift/shop-staging/ses-smtp")
	if len(got) != len(base)+1 {
		t.Fatalf("secrets = %d, want %d", len(got), len(base)+1)
	}
	last := got[len(got)-1]
	if last.Name != "CONFIG__DEFAULT__SYSTEM__SMTP__PASSWORD" || last.JSONKey != "password" {
		t.Fatalf("smtp secret = %#v", last)
	}
	if got := appendSmtpSecret(base, ""); len(got) != len(base) {
		t.Fatalf("empty SMTP ARN appended %d secrets", len(got)-len(base))
	}
}
