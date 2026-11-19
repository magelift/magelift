package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/s3state"
	"github.com/magelift/magelift/providers/aws/secrets"
	"github.com/magelift/magelift/sdk"
)

type heldLock struct {
	mu      sync.Mutex
	target  sdk.Envelope
	backend string
	release func(context.Context) error
}

func newToken() (string, error) {
	var id [24]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func (s *Server) BootstrapVerify(req *sdk.BootstrapVerifyCall, resp *sdk.BootstrapVerifyResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		err := platform.ModuleBootstrap(m).VerifyAccount(ctx, p)
		resp.Verified = err == nil
		return err
	})
}

func (s *Server) BootstrapEnsure(req *sdk.BootstrapEnsureCall, resp *sdk.BootstrapEnsureResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		opts, err := decode[platform.BootstrapRequest](req.RequestJSON)
		if err != nil {
			return err
		}
		result, err := platform.ModuleBootstrap(m).Ensure(ctx, p, opts)
		if err != nil {
			return err
		}
		return encode(&resp.ResultJSON, result)
	})
}

func (s *Server) StateStatus(req *sdk.StateStatusCall, resp *sdk.StateStatusResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		locked, info, backend, err := platform.ModuleState(m).Status(ctx, p)
		if err != nil {
			return err
		}
		resp.Locked, resp.BackendURL = locked, backend
		if info != nil {
			return encode(&resp.InfoJSON, info)
		}
		return nil
	})
}

func (s *Server) StateLock(req *sdk.StateLockCall, resp *sdk.StateLockResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		token, err := newToken()
		if err != nil {
			return err
		}
		release, err := platform.ModuleState(m).Lock(ctx, p, req.Owner)
		if s3state.IsNotFound(err) {
			return &sdk.OperationError{Code: sdk.ErrCodeNotFound, Message: fmt.Sprintf("state bucket is missing; run magelift bootstrap --env %s --access-log-bucket <existing-bucket>", p.Environment())}
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.locks == nil {
			s.locks = make(map[string]*heldLock)
		}
		s.locks[token] = &heldLock{target: req.Envelope, backend: req.Plan.StateBackendURL, release: release}
		resp.Locked, resp.ReleaseToken = true, token
		return nil
	})
}

func (s *Server) StateUnlock(req *sdk.StateUnlockCall, resp *sdk.StateUnlockResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		if req.ReleaseToken != "" {
			s.mu.Lock()
			lock, ok := s.locks[req.ReleaseToken]
			s.mu.Unlock()
			if !ok || lock.target != req.Envelope || lock.backend != req.Plan.StateBackendURL {
				return invalid(errors.New("AWS lock does not belong to this invocation and stack"))
			}
			lock.mu.Lock()
			defer lock.mu.Unlock()
			if err := lock.release(ctx); err != nil {
				return err
			}
			s.mu.Lock()
			delete(s.locks, req.ReleaseToken)
			s.mu.Unlock()
			return nil
		}
		info, err := platform.ModuleState(m).Unlock(ctx, p)
		if err != nil {
			return err
		}
		if info != nil {
			return encode(&resp.InfoJSON, info)
		}
		return nil
	})
}

func (s *Server) StateBackup(req *sdk.StateBackupCall, resp *sdk.StateBackupResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		result, err := platform.ModuleState(m).Backup(ctx, p)
		if err != nil {
			return err
		}
		return encode(&resp.ResultJSON, result)
	})
}

func (s *Server) StateRestore(req *sdk.StateRestoreCall, resp *sdk.StateRestoreResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		result, err := platform.ModuleState(m).Restore(ctx, p, req.Location)
		if err != nil {
			return err
		}
		return encode(&resp.ResultJSON, result)
	})
}

func (s *Server) SecretList(req *sdk.SecretListCall, resp *sdk.SecretListResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		values, err := platform.ModuleSecrets(m).List(ctx, p)
		if err != nil {
			return err
		}
		return encode(&resp.SecretsJSON, values)
	})
}

func (s *Server) SecretSet(req *sdk.SecretSetCall, resp *sdk.SecretSetResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		err := platform.ModuleSecrets(m).Set(ctx, p, req.Name, req.Value)
		resp.Written = err == nil
		return err
	})
}

func (s *Server) SecretRemove(req *sdk.SecretRemoveCall, resp *sdk.SecretRemoveResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		err := platform.ModuleSecrets(m).Remove(ctx, p, req.Name)
		resp.Removed = err == nil
		return err
	})
}

func (s *Server) SecretRead(req *sdk.SecretReadRequest, resp *sdk.SecretReadResult) error {
	return respond(req.ProtocolVersion, &resp.Error, time.Minute, func(ctx context.Context) error {
		store, err := secrets.New(ctx, req.Envelope.Region)
		if err != nil {
			return err
		}
		if req.Parameter {
			resp.Value, err = store.GetParameter(ctx, req.Name)
		} else {
			resp.Value, err = store.GetSecretValue(ctx, req.Name)
		}
		return err
	})
}
