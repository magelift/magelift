package resilience

import (
	"context"
	"testing"

	secret "github.com/scaleway/scaleway-sdk-go/api/secret/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

func TestScalewaySecretSDKTranslatesOfficialRequestsWithoutExposingValues(t *testing.T) {
	t.Parallel()

	client := &fakeOfficialSecretAPI{
		items:  []*secret.Secret{{ID: "id-1", Name: "shop/api", Status: secret.SecretStatusReady, Protected: false, Tags: []string{"one"}}},
		access: []byte("secret-value"),
	}
	api, err := NewScalewaySecretAPIFromClient(client, "project-1", "fr-par")
	if err != nil {
		t.Fatal(err)
	}

	listed, err := api.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].ID != "id-1" || listed[0].Name != "shop/api" || listed[0].Protected {
		t.Fatalf("List = %#v, err=%v", listed, err)
	}
	if client.listRequest == nil || client.listRequest.Region != scw.Region("fr-par") || client.listRequest.ProjectID == nil || *client.listRequest.ProjectID != "project-1" || client.listRequest.Page == nil || *client.listRequest.Page != 1 {
		t.Fatalf("List request = %#v", client.listRequest)
	}

	data := []byte("secret-value")
	if err := api.CreateVersion(context.Background(), "id-1", data); err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	if client.versionRequest == nil || client.versionRequest.Region != scw.Region("fr-par") || client.versionRequest.SecretID != "id-1" || string(client.versionRequest.Data) != "secret-value" {
		t.Fatal("CreateVersion did not copy the value inside the provider boundary")
	}

	created, err := api.Create(context.Background(), "shop/new", []string{"managed"}, false)
	if err != nil || created.ID != "created" {
		t.Fatalf("Create = %#v, err=%v", created, err)
	}
	if client.createRequest == nil || client.createRequest.ProjectID != "project-1" || client.createRequest.Region != scw.Region("fr-par") || client.createRequest.Name != "shop/new" || client.createRequest.Protected || len(client.createRequest.Tags) != 1 || client.createRequest.Tags[0] != "managed" {
		t.Fatalf("Create request = %#v", client.createRequest)
	}

	value, err := api.Access(context.Background(), "id-1", "latest_enabled")
	if err != nil {
		t.Fatalf("Access returned an error: %v", err)
	}
	if string(value) != "secret-value" {
		t.Fatal("Access returned an unexpected value")
	}
	if client.accessRequest == nil || client.accessRequest.Region != scw.Region("fr-par") || client.accessRequest.SecretID != "id-1" || client.accessRequest.Revision != "latest_enabled" {
		t.Fatalf("Access request = %#v", client.accessRequest)
	}

	metadata, err := api.Get(context.Background(), "id-1")
	if err != nil || metadata.ID != "id-1" {
		t.Fatalf("Get = %#v, err=%v", metadata, err)
	}
	if err := api.Delete(context.Background(), "id-1"); err != nil {
		t.Fatal(err)
	}
	if len(client.deleteRequests) != 1 || client.deleteRequests[0].SecretID != "id-1" || client.deleteRequests[0].Region != scw.Region("fr-par") {
		t.Fatalf("Delete requests = %#v", client.deleteRequests)
	}
	if _, err := api.Protect(context.Background(), "id-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Unprotect(context.Background(), "id-1"); err != nil {
		t.Fatal(err)
	}
}

func TestNewScalewaySecretAPIFromClientValidatesProviderScope(t *testing.T) {
	t.Parallel()

	client := &fakeOfficialSecretAPI{}
	for _, test := range []struct {
		name    string
		api     SecretSDKAPI
		project string
		region  string
	}{
		{name: "nil client", project: "project-1", region: "fr-par"},
		{name: "missing project", api: client, region: "fr-par"},
		{name: "missing region", api: client, project: "project-1"},
		{name: "multiline project", api: client, project: "project\n1", region: "fr-par"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewScalewaySecretAPIFromClient(test.api, test.project, test.region); err == nil {
				t.Fatal("invalid provider scope was accepted")
			}
		})
	}
}

type fakeOfficialSecretAPI struct {
	items            []*secret.Secret
	access           []byte
	listRequest      *secret.ListSecretsRequest
	createRequest    *secret.CreateSecretRequest
	versionRequest   *secret.CreateSecretVersionRequest
	accessRequest    *secret.AccessSecretVersionRequest
	deleteRequests   []*secret.DeleteSecretRequest
	protectRequest   *secret.ProtectSecretRequest
	unprotectRequest *secret.UnprotectSecretRequest
}

func (f *fakeOfficialSecretAPI) GetSecret(req *secret.GetSecretRequest, _ ...scw.RequestOption) (*secret.Secret, error) {
	return &secret.Secret{ID: req.SecretID, Name: "shop/api", Status: secret.SecretStatusReady}, nil
}

func (f *fakeOfficialSecretAPI) ListSecrets(req *secret.ListSecretsRequest, _ ...scw.RequestOption) (*secret.ListSecretsResponse, error) {
	f.listRequest = &secret.ListSecretsRequest{Region: req.Region, Page: req.Page}
	if req.ProjectID != nil {
		project := *req.ProjectID
		f.listRequest.ProjectID = &project
	}
	return &secret.ListSecretsResponse{Secrets: f.items, TotalCount: uint64(len(f.items))}, nil
}

func (f *fakeOfficialSecretAPI) AccessSecretVersion(req *secret.AccessSecretVersionRequest, _ ...scw.RequestOption) (*secret.AccessSecretVersionResponse, error) {
	f.accessRequest = &secret.AccessSecretVersionRequest{Region: req.Region, SecretID: req.SecretID, Revision: req.Revision}
	return &secret.AccessSecretVersionResponse{SecretID: req.SecretID, Revision: 1, Data: append([]byte(nil), f.access...)}, nil
}

func (f *fakeOfficialSecretAPI) DeleteSecret(req *secret.DeleteSecretRequest, _ ...scw.RequestOption) error {
	f.deleteRequests = append(f.deleteRequests, &secret.DeleteSecretRequest{Region: req.Region, SecretID: req.SecretID})
	return nil
}

func (f *fakeOfficialSecretAPI) CreateSecret(req *secret.CreateSecretRequest, _ ...scw.RequestOption) (*secret.Secret, error) {
	f.createRequest = &secret.CreateSecretRequest{Region: req.Region, ProjectID: req.ProjectID, Name: req.Name, Tags: append([]string(nil), req.Tags...), Protected: req.Protected}
	return &secret.Secret{ID: "created", Name: req.Name, Status: secret.SecretStatusReady, Protected: req.Protected}, nil
}

func (f *fakeOfficialSecretAPI) CreateSecretVersion(req *secret.CreateSecretVersionRequest, _ ...scw.RequestOption) (*secret.SecretVersion, error) {
	f.versionRequest = &secret.CreateSecretVersionRequest{Region: req.Region, SecretID: req.SecretID, Data: append([]byte(nil), req.Data...)}
	return &secret.SecretVersion{SecretID: req.SecretID}, nil
}

func (f *fakeOfficialSecretAPI) ProtectSecret(req *secret.ProtectSecretRequest, _ ...scw.RequestOption) (*secret.Secret, error) {
	f.protectRequest = &secret.ProtectSecretRequest{Region: req.Region, SecretID: req.SecretID}
	return &secret.Secret{ID: req.SecretID, Name: "shop/api", Status: secret.SecretStatusReady, Protected: true}, nil
}

func (f *fakeOfficialSecretAPI) UnprotectSecret(req *secret.UnprotectSecretRequest, _ ...scw.RequestOption) (*secret.Secret, error) {
	f.unprotectRequest = &secret.UnprotectSecretRequest{Region: req.Region, SecretID: req.SecretID}
	return &secret.Secret{ID: req.SecretID, Name: "shop/api", Status: secret.SecretStatusReady, Protected: false}, nil
}

var _ SecretSDKAPI = (*fakeOfficialSecretAPI)(nil)
