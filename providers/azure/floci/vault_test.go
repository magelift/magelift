//go:build floci_azure

package floci_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/magelift/magelift/providers/azure/secrets"
)

type fakeCredential struct{}

func (fakeCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "floci-test-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

type localHTTPTransport struct{}

func (localHTTPTransport) Do(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = "http"
	return http.DefaultClient.Do(clone)
}

func TestKeyVaultSecretLifecycle(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI_AZURE") != "1" {
		t.Skip("set MAGELIFT_FLOCI_AZURE=1 to run against the Azure emulator")
	}
	endpoint := os.Getenv("MAGELIFT_AZURE_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:4577"
	}
	url := "https://" + strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(endpoint, "/"), "http://"), "https://") + "/devstoreaccount1-keyvault"
	client, err := azsecrets.NewClient(url, fakeCredential{}, &azsecrets.ClientOptions{
		ClientOptions:                        azcore.ClientOptions{Transport: localHTTPTransport{}},
		DisableChallengeResourceVerification: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := secrets.NewStoreWithClient(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	name := fmt.Sprintf("magelift-secret-%d", time.Now().UnixNano())
	if err := store.Set(ctx, name, []byte("encryption-key")); err != nil {
		t.Fatal(err)
	}
	data, err := store.Get(ctx, name)
	if err != nil || string(data) != "encryption-key" {
		t.Fatalf("secret data %q: %v", data, err)
	}
	if err := store.Delete(ctx, name); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, name); err == nil {
		t.Fatal("deleted secret remains readable")
	}
}
