package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/secretsafe"
	azurestate "github.com/magelift/magelift/providers/azure/state"
	"github.com/magelift/magelift/sdk"
)

var storageAccountPattern = regexp.MustCompile(`^[a-z0-9]{3,24}$`)

func (s *Server) stateManager(ctx context.Context, plan storedPlan) (*azurestate.Manager, error) {
	if s.stateFactory != nil {
		return s.stateFactory(ctx, plan)
	}
	backend, err := url.Parse(plan.BackendURL)
	if err != nil || backend.Scheme != "azblob" || backend.Host == "" {
		return nil, errors.New("Azure Blob state container is invalid")
	}
	client, err := s.blobClient(ctx, plan)
	if err != nil {
		return nil, err
	}
	return azurestate.NewManager(client, backend.Host, plan.Spec.Project, plan.Spec.Environment)
}

func (s *Server) blobClient(ctx context.Context, plan storedPlan) (*azblob.Client, error) {
	if s.blobFactory != nil {
		return s.blobFactory(ctx, plan)
	}
	account := strings.TrimSpace(os.Getenv("AZURE_STORAGE_ACCOUNT"))
	if !storageAccountPattern.MatchString(account) {
		return nil, errors.New("set AZURE_STORAGE_ACCOUNT to the Azure Blob state account name")
	}
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("create Azure credential: %w", err)
	}
	client, err := azblob.NewClient("https://"+account+".blob.core.windows.net", credential, nil)
	if err != nil {
		return nil, fmt.Errorf("create Azure state client: %w", err)
	}
	return client, nil
}

func statePlan(envelope sdk.Envelope, plan sdk.StoredPlan) (storedPlan, error) {
	return checkedStoredPlan(&sdk.StackCall{Envelope: envelope, Plan: plan})
}

func stateError(err error) *sdk.OperationError {
	code := sdk.ErrCodeUpstream
	if errors.Is(err, azurestate.ErrLocked) || errors.Is(err, azurestate.ErrLockReplaced) {
		code = sdk.ErrCodeConflict
	}
	message, _ := secretsafe.RedactSensitiveText(err.Error())
	return &sdk.OperationError{Code: code, Message: message}
}

func (s *Server) StateStatus(req *sdk.StateStatusCall, resp *sdk.StateStatusResult) error {
	if req == nil || resp == nil {
		return errors.New("state status request and response are required")
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
	manager, err := s.stateManager(ctx, plan)
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	info, err := manager.Status(ctx)
	if errors.Is(err, azurestate.ErrNotLocked) {
		resp.BackendURL = plan.BackendURL
		return nil
	}
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	resp.Locked, resp.InfoJSON, resp.BackendURL = true, encoded, plan.BackendURL
	return nil
}

func (s *Server) StateLock(req *sdk.StateLockCall, resp *sdk.StateLockResult) error {
	if req == nil || resp == nil {
		return errors.New("state lock request and response are required")
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
	manager, err := s.stateManager(ctx, plan)
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	key := plan.BackendURL + "\x00" + plan.Envelope.StackName
	if s.locks[key] != nil {
		resp.Error = stateError(azurestate.ErrLocked)
		return nil
	}
	handle, err := manager.Acquire(ctx, req.Owner)
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	if s.locks == nil {
		s.locks = make(map[string]*azurestate.Handle)
	}
	s.locks[key] = handle
	resp.Locked = true
	return nil
}

func (s *Server) StateUnlock(req *sdk.StateUnlockCall, resp *sdk.StateUnlockResult) error {
	if req == nil || resp == nil {
		return errors.New("state unlock request and response are required")
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
	manager, err := s.stateManager(ctx, plan)
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	key := plan.BackendURL + "\x00" + plan.Envelope.StackName
	var info platform.LockInfo
	if handle := s.locks[key]; handle != nil {
		info, err = manager.Status(ctx)
		if err == nil {
			err = handle.Release(ctx)
		}
		if err == nil || errors.Is(err, azurestate.ErrNotLocked) || errors.Is(err, azurestate.ErrLockReplaced) {
			delete(s.locks, key)
		}
	} else {
		info, err = manager.Unlock(ctx)
	}
	if errors.Is(err, azurestate.ErrNotLocked) {
		return nil
	}
	if err != nil {
		resp.Error = stateError(err)
		return nil
	}
	resp.InfoJSON, err = json.Marshal(info)
	if err != nil {
		resp.Error = stateError(err)
	}
	return nil
}
