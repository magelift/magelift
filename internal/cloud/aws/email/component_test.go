package email

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	mockSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	mockRegion = "eu-west-3"
	mockDomain = "example.invalid"
	mockZone   = "Z1234567890ABC"
)

type emailMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *emailMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	outputs := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:sesv2/emailIdentity:EmailIdentity":
		outputs["arn"] = resource.NewStringProperty("arn:aws:ses:" + mockRegion + ":123456789012:identity/" + mockDomain)
		outputs["dkimSigningAttributes"] = resource.NewObjectProperty(resource.PropertyMap{
			"tokens": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("token1"),
				resource.NewStringProperty("token2"),
				resource.NewStringProperty("token3"),
			}),
		})
	case "aws:iam/accessKey:AccessKey":
		outputs["id"] = resource.NewStringProperty("AKIAIOSFODNN7EXAMPLE")
		outputs["secret"] = resource.MakeSecret(resource.NewStringProperty(mockSecret))
	case "aws:ssm/parameter:Parameter":
		outputs["arn"] = resource.NewStringProperty("arn:aws:ssm:" + mockRegion + ":123456789012:parameter/magelift/shop-staging/ses-smtp")
	}
	return args.Name + "-id", outputs, nil
}

func (m *emailMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return nil, nil
}

func runComponent(t *testing.T, args Args) *emailMocks {
	t.Helper()
	mocks := &emailMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String(mockRegion)})
		if err != nil {
			return err
		}
		args.RegionalProvider = provider
		_, err = New(ctx, "shop-staging", args)
		return err
	}, pulumi.WithMocks("magelift", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}
	return mocks
}

func baseArgs() Args {
	return Args{Project: "shop", Environment: "staging", Domain: mockDomain, HostedZoneID: mockZone, Region: mockRegion, Tags: map[string]string{"test": "email"}}
}

func resourcesOfType(mocks *emailMocks, token string) []pulumi.MockResourceArgs {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	var result []pulumi.MockResourceArgs
	for _, registered := range mocks.resources {
		if registered.TypeToken == token {
			result = append(result, registered)
		}
	}
	return result
}

func TestCreatesIdentityDKIMRecordsUserKeyPolicyAndSecret(t *testing.T) {
	mocks := runComponent(t, baseArgs())
	for token, want := range map[string]int{
		"aws:sesv2/emailIdentity:EmailIdentity": 1,
		"aws:route53/record:Record":             3,
		"aws:iam/user:User":                     1,
		"aws:iam/accessKey:AccessKey":           1,
		"aws:iam/userPolicy:UserPolicy":         1,
		"aws:ssm/parameter:Parameter":           1,
	} {
		if got := len(resourcesOfType(mocks, token)); got != want {
			t.Errorf("%s count = %d, want %d", token, got, want)
		}
	}
}

func TestDKIMRecordsTargetSuppliedZone(t *testing.T) {
	mocks := runComponent(t, baseArgs())
	for _, record := range resourcesOfType(mocks, "aws:route53/record:Record") {
		inputs := record.Inputs.Mappable()
		if inputs["zoneId"] != mockZone {
			t.Errorf("DKIM record zone = %v, want %s", inputs["zoneId"], mockZone)
		}
		if inputs["type"] != "CNAME" {
			t.Errorf("DKIM record type = %v, want CNAME", inputs["type"])
		}
	}
}

func TestSMTPCredentialUsesConvertedPasswordMarkedSecret(t *testing.T) {
	mocks := runComponent(t, baseArgs())
	params := resourcesOfType(mocks, "aws:ssm/parameter:Parameter")
	if len(params) != 1 {
		t.Fatalf("ssm parameters = %d, want 1", len(params))
	}
	value, ok := params[0].Inputs["value"]
	if !ok {
		t.Fatal("ssm parameter has no value input")
	}
	if !value.IsSecret() {
		t.Error("ssm credential value is not marked secret")
	}
	// Mock inputs capture pre-resolution values, so resolved JSON content is
	// covered by TestSmtpCredentialsJSONShape plus the live send cell instead.
	name, _ := params[0].Inputs.Mappable()["name"].(string)
	if name != "/magelift/shop-staging/ses-smtp" {
		t.Errorf("ssm parameter name = %q", name)
	}
}

func TestRawSecretNeverAppearsOutsideSecretValues(t *testing.T) {
	mocks := runComponent(t, baseArgs())
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	for _, registered := range mocks.resources {
		encoded, err := json.Marshal(registered.Inputs.Mappable())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), mockSecret) {
			t.Errorf("%s inputs leak the raw IAM secret", registered.TypeToken)
		}
	}
}

func TestSendingPolicyGrantsOnlySendRawEmail(t *testing.T) {
	mocks := runComponent(t, baseArgs())
	policies := resourcesOfType(mocks, "aws:iam/userPolicy:UserPolicy")
	if len(policies) != 1 {
		t.Fatalf("user policies = %d, want 1", len(policies))
	}
	document, _ := policies[0].Inputs.Mappable()["policy"].(string)
	var parsed struct {
		Statement []struct {
			Effect   string
			Action   []string
			Resource string
		}
	}
	if err := json.Unmarshal([]byte(document), &parsed); err != nil {
		t.Fatalf("policy JSON invalid: %v", err)
	}
	if len(parsed.Statement) != 1 || len(parsed.Statement[0].Action) != 1 ||
		parsed.Statement[0].Action[0] != "ses:SendRawEmail" || parsed.Statement[0].Resource != "*" {
		t.Errorf("sending policy is not least-privilege SendRawEmail: %s", document)
	}
}

func TestValidationRejectsBadInputs(t *testing.T) {
	cases := []struct {
		name string
		args Args
		want string
	}{
		{"empty domain", Args{Project: "shop", Environment: "staging", Domain: "", HostedZoneID: mockZone, Region: mockRegion}, "not a valid DNS domain"},
		{"bad zone", Args{Project: "shop", Environment: "staging", Domain: mockDomain, HostedZoneID: "nope", Region: mockRegion}, "not a valid Route 53 zone ID"},
		{"empty region", Args{Project: "shop", Environment: "staging", Domain: mockDomain, HostedZoneID: mockZone}, "region is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mocks := &emailMocks{}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				_, err := New(ctx, "shop-staging", tc.args)
				return err
			}, pulumi.WithMocks("magelift", "test", mocks))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if got := len(mocks.resources); got != 0 {
				t.Fatalf("created %d resources despite invalid args", got)
			}
		})
	}
}
