package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

var ErrLocked = errors.New("deployment lock is held")
var ErrNotLocked = errors.New("deployment lock is not held")
var stableName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
var bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var kmsKeyARN = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)

type S3API interface {
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type Manager struct {
	client      S3API
	bucket      string
	key         string
	project     string
	environment string
	encryption  ObjectEncryption
	now         func() time.Time
}

type Info struct {
	Project     string    `json:"project" yaml:"project"`
	Environment string    `json:"environment" yaml:"environment"`
	Owner       string    `json:"owner" yaml:"owner"`
	AcquiredAt  time.Time `json:"acquiredAt" yaml:"acquiredAt"`
	ETag        string    `json:"etag" yaml:"etag"`
}

type Handle struct {
	manager *Manager
	info    Info
}

func NewManager(client S3API, bucket, project, environment string, encryption ObjectEncryption) (*Manager, error) {
	if client == nil || !bucketName.MatchString(bucket) || !stableName.MatchString(project) || !stableName.MatchString(environment) {
		return nil, errors.New("state lock requires an S3 client, stable names, and a state bucket")
	}
	if err := encryption.validate(); err != nil {
		return nil, err
	}
	return &Manager{
		client: client, bucket: bucket, key: "locks/" + project + "/" + environment + ".json",
		project: project, environment: environment, encryption: encryption, now: time.Now,
	}, nil
}

func (m *Manager) Acquire(ctx context.Context, project, environment, owner string) (*Handle, error) {
	if m == nil || m.client == nil || strings.TrimSpace(owner) == "" {
		return nil, errors.New("state lock manager and owner are required")
	}
	if !stableName.MatchString(project) || !stableName.MatchString(environment) {
		return nil, errors.New("state lock project and environment must be stable names")
	}
	if project != m.project || environment != m.environment {
		return nil, errors.New("state lock scope does not match the manager")
	}
	now := m.now()
	info := Info{Project: project, Environment: environment, Owner: owner, AcquiredAt: now}
	data, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("encode deployment lock: %w", err)
	}
	putInput := &s3.PutObjectInput{
		Bucket: awssdk.String(m.bucket), Key: awssdk.String(m.key), Body: bytes.NewReader(data), ContentType: awssdk.String("application/json"),
		IfNoneMatch: awssdk.String("*"),
	}
	m.encryption.applyPut(putInput)
	result, err := m.client.PutObject(ctx, putInput)
	if err != nil {
		if isPreconditionFailed(err) {
			current, inspectErr := m.inspect(ctx)
			if inspectErr == nil {
				return nil, fmt.Errorf("%w: owner %q acquired at %s", ErrLocked, current.Owner, current.AcquiredAt.UTC().Format(time.RFC3339))
			}
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquire deployment lock: %w", err)
	}
	if result != nil && result.ETag != nil {
		info.ETag = strings.Trim(awssdk.ToString(result.ETag), "\"")
	}
	return &Handle{manager: m, info: info}, nil
}

// Lock acquires the deployment lock and returns a release function for callers
// that should not depend on the handle representation.
func (m *Manager) Lock(ctx context.Context, project, environment, owner string) (func() error, error) {
	handle, err := m.Acquire(ctx, project, environment, owner)
	if err != nil {
		return nil, err
	}
	return func() error { return handle.Release(ctx) }, nil
}

func (m *Manager) Status(ctx context.Context) (Info, error) {
	if m == nil || m.client == nil {
		return Info{}, errors.New("state lock manager is required")
	}
	return m.inspect(ctx)
}

// Unlock removes a lock only after reading its current metadata. The CLI gates
// this operation behind an explicit confirmation because it can interrupt a
// running deployment.
func (m *Manager) Unlock(ctx context.Context) (Info, error) {
	if m == nil || m.client == nil {
		return Info{}, errors.New("state lock manager is required")
	}
	current, err := m.inspect(ctx)
	if err != nil {
		return Info{}, err
	}
	input := &s3.DeleteObjectInput{Bucket: awssdk.String(m.bucket), Key: awssdk.String(m.key)}
	if current.ETag != "" {
		input.IfMatch = awssdk.String(current.ETag)
	}
	if _, err := m.client.DeleteObject(ctx, input); err != nil {
		return Info{}, fmt.Errorf("unlock deployment state: %w", err)
	}
	return current, nil
}

func (h *Handle) Info() Info {
	if h == nil {
		return Info{}
	}
	return h.info
}

func (h *Handle) Release(ctx context.Context) error {
	if h == nil || h.manager == nil {
		return errors.New("deployment lock handle is required")
	}
	current, err := h.manager.inspect(ctx)
	if err != nil {
		return err
	}
	if current.Owner != h.info.Owner || !current.AcquiredAt.Equal(h.info.AcquiredAt) {
		return errors.New("deployment lock ownership changed")
	}
	input := &s3.DeleteObjectInput{Bucket: awssdk.String(h.manager.bucket), Key: awssdk.String(h.manager.key)}
	if current.ETag != "" {
		input.IfMatch = awssdk.String(current.ETag)
	}
	if _, err := h.manager.client.DeleteObject(ctx, input); err != nil {
		return fmt.Errorf("release deployment lock: %w", err)
	}
	h.manager = nil
	return nil
}

func (m *Manager) inspect(ctx context.Context) (Info, error) {
	object, err := m.client.GetObject(ctx, &s3.GetObjectInput{Bucket: awssdk.String(m.bucket), Key: awssdk.String(m.key)})
	if err != nil {
		if isNotFound(err) {
			return Info{}, ErrNotLocked
		}
		return Info{}, fmt.Errorf("inspect deployment lock: %w", err)
	}
	if object == nil || object.Body == nil {
		return Info{}, errors.New("inspect deployment lock returned no body")
	}
	defer object.Body.Close()
	var info Info
	if err := json.NewDecoder(object.Body).Decode(&info); err != nil {
		return Info{}, errors.New("deployment lock contains invalid metadata")
	}
	if info.Project == "" || info.Environment == "" || info.Owner == "" || info.AcquiredAt.IsZero() {
		return Info{}, errors.New("deployment lock metadata is incomplete")
	}
	if object.ETag != nil {
		info.ETag = strings.Trim(awssdk.ToString(object.ETag), "\"")
	}
	return info, nil
}

func isPreconditionFailed(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && (apiError.ErrorCode() == "PreconditionFailed" || apiError.ErrorCode() == "ConditionalRequestConflict")
}

func isNotFound(err error) bool {
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return false
	}
	switch apiError.ErrorCode() {
	case "NoSuchKey", "NotFound", "NoSuchBucket", "NoSuchObject":
		return true
	default:
		return false
	}
}
