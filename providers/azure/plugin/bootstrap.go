package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func (s *Server) BootstrapVerify(req *sdk.BootstrapVerifyCall, resp *sdk.BootstrapVerifyResult) error {
	if req == nil || resp == nil {
		return errors.New("bootstrap verification request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := statePlan(req.Envelope, req.Plan)
	if err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := s.blobClient(ctx, plan)
	if err == nil {
		_, err = client.ServiceClient().GetAccountInfo(ctx, nil)
		var responseError *azcore.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == 501 {
			// Some Azure emulators implement container listing but
			// do not implement the Azure account-info operation.
			_, err = client.NewListContainersPager(nil).NextPage(ctx)
		}
	}
	if err != nil {
		resp.Error = stateError(fmt.Errorf("verify Azure Blob state account: %w", err))
		return nil
	}
	resp.Verified = true
	return nil
}

func (s *Server) BootstrapEnsure(req *sdk.BootstrapEnsureCall, resp *sdk.BootstrapEnsureResult) error {
	if req == nil || resp == nil {
		return errors.New("bootstrap request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := statePlan(req.Envelope, req.Plan)
	if err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
		return nil
	}
	var request platform.BootstrapRequest
	if len(req.RequestJSON) > 0 {
		if err := json.Unmarshal(req.RequestJSON, &request); err != nil {
			resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: "invalid Azure bootstrap request"}
			return nil
		}
	}
	if request.AccessLogBucket != "" || request.GitHubOwner != "" || request.GitHubRepo != "" || request.ApplyCIPermissions {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeCompatibility, Message: "Azure bootstrap does not yet configure CI identities or access-log buckets"}
		return nil
	}
	if strings.TrimSpace(os.Getenv("PULUMI_CONFIG_PASSPHRASE")) == "" && strings.TrimSpace(os.Getenv("PULUMI_CONFIG_PASSPHRASE_FILE")) == "" {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeCredential, Message: "set PULUMI_CONFIG_PASSPHRASE or PULUMI_CONFIG_PASSPHRASE_FILE from a stable secret before Azure bootstrap"}
		return nil
	}
	backend, _ := url.Parse(plan.BackendURL)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := s.blobClient(ctx, plan)
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	container := client.ServiceClient().NewContainerClient(backend.Host)
	_, err = container.GetProperties(ctx, nil)
	if err != nil {
		var responseError *azcore.ResponseError
		if !errors.As(err, &responseError) || responseError.StatusCode != 404 {
			resp.Error = stateError(err)
			return nil
		}
		_, err = client.CreateContainer(ctx, backend.Host, nil)
		if err != nil {
			if !errors.As(err, &responseError) || responseError.StatusCode != 409 {
				resp.Error = stateError(err)
				return nil
			}
			if _, err = container.GetProperties(ctx, nil); err != nil {
				resp.Error = stateError(err)
				return nil
			}
		}
	}
	result := platform.BootstrapResult{BackendURL: plan.BackendURL, Details: map[string]any{
		"stateContainer":             backend.Host,
		"stateAccountSource":         "AZURE_STORAGE_ACCOUNT",
		"encryptionPassphraseSource": "PULUMI_CONFIG_PASSPHRASE or PULUMI_CONFIG_PASSPHRASE_FILE",
	}}
	resp.ResultJSON, err = json.Marshal(result)
	if err != nil {
		resp.Error = stateError(err)
	}
	return nil
}
