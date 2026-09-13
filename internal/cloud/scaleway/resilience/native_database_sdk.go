package resilience

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	rdb "github.com/scaleway/scaleway-sdk-go/api/rdb/v1"
	secret "github.com/scaleway/scaleway-sdk-go/api/secret/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// NewScalewayNativeAPIWithDatabase constructs the Object Storage and Managed
// Database translators from official Scaleway SDK clients. Authentication is
// deliberately supplied through SDK options (for example scw.WithEnv or a
// community credential resolver), so no secret value enters operation state.
func NewScalewayNativeAPIWithDatabase(ctx context.Context, config NativeAPIConfig, options ...scw.ClientOption) (*NativeAPI, error) {
	return newScalewayNativeAPIWithOfficialServices(ctx, config, true, false, options...)
}

// NewScalewayDatabaseNativeAPI constructs only the Managed Database
// translator. Database-only recovery cells must not require an Object Storage
// archive bucket or perform an unrelated object inventory request.
func NewScalewayDatabaseNativeAPI(ctx context.Context, config NativeAPIConfig, options ...scw.ClientOption) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway recovery context is required")
	}
	if strings.TrimSpace(firstNonEmptyScaleway(config.DatabaseRegion, config.Region)) == "" {
		return nil, errors.New("Scaleway Managed Database region is required")
	}
	if err := config.validatePolicy(); err != nil {
		return nil, err
	}
	client, region, project, err := newScalewayServiceClient(config, options...)
	if err != nil {
		return nil, err
	}
	native := &NativeAPI{
		database: &scalewayDatabaseSDK{api: rdb.NewAPI(client), region: region, project: project},
		config:   normalizeScalewayConfig(config),
	}
	return native, nil
}

// NewScalewayNativeAPIWithDatabaseAndSecrets constructs the Object Storage,
// Managed Database, and Secret Manager translators from one official SDK
// client. The service booleans are internal so the public constructors stay
// explicit about which native boundaries they enable.
func NewScalewayNativeAPIWithDatabaseAndSecrets(ctx context.Context, config NativeAPIConfig, options ...scw.ClientOption) (*NativeAPI, error) {
	return newScalewayNativeAPIWithOfficialServices(ctx, config, true, true, options...)
}

// NewScalewayNativeAPIWithSecrets constructs the official Object Storage and
// Secret Manager translators without enabling Managed Database calls. This
// keeps focused recovery cells from requiring database permissions or making
// unrelated inventory requests.
func NewScalewayNativeAPIWithSecrets(ctx context.Context, config NativeAPIConfig, options ...scw.ClientOption) (*NativeAPI, error) {
	return newScalewayNativeAPIWithOfficialServices(ctx, config, false, true, options...)
}

func newScalewayNativeAPIWithOfficialServices(ctx context.Context, config NativeAPIConfig, includeDatabase, includeSecrets bool, options ...scw.ClientOption) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway recovery context is required")
	}
	native, err := NewScalewayNativeAPI(ctx, config)
	if err != nil {
		return nil, err
	}
	client, region, project, err := newScalewayServiceClient(config, options...)
	if err != nil {
		return nil, err
	}
	if includeDatabase {
		native.database = &scalewayDatabaseSDK{api: rdb.NewAPI(client), region: region, project: project}
	}
	if includeSecrets {
		native.secrets = &scalewaySecretSDK{api: secret.NewAPI(client), region: region, project: project}
	}
	return native, nil
}

func newScalewayServiceClient(config NativeAPIConfig, options ...scw.ClientOption) (*scw.Client, scw.Region, string, error) {
	region := firstNonEmptyScaleway(config.DatabaseRegion, config.Region)
	clientOptions := []scw.ClientOption{scw.WithEnv()}
	if strings.TrimSpace(config.DatabaseProjectID) != "" {
		clientOptions = append(clientOptions, scw.WithDefaultProjectID(config.DatabaseProjectID))
	}
	if region != "" {
		clientOptions = append(clientOptions, scw.WithDefaultRegion(scw.Region(region)))
	}
	clientOptions = append(clientOptions, options...)
	// Scaleway's CreateSnapshot/CreateInstanceFromSnapshot POSTs can stay open
	// after the resource exists. Request context is not enough; bound every
	// HTTP round-trip so reconcile-by-name can run.
	clientOptions = append(clientOptions, scw.WithHTTPClient(&http.Client{Timeout: databaseMutatingPostTime}))
	client, err := scw.NewClient(clientOptions...)
	if err != nil {
		return nil, scw.Region(region), config.DatabaseProjectID, fmt.Errorf("construct Scaleway SDK client: %w", err)
	}
	project := strings.TrimSpace(config.DatabaseProjectID)
	if project == "" {
		if defaultProject, ok := client.GetDefaultProjectID(); ok {
			project = defaultProject
		}
	}
	return client, scw.Region(region), project, nil
}

type scalewayDatabaseSDK struct {
	api     *rdb.API
	region  scw.Region
	project string
}

func (client *scalewayDatabaseSDK) GetInstance(ctx context.Context, id string) (DatabaseInstance, error) {
	if client == nil || client.api == nil {
		return DatabaseInstance{}, errors.New("Scaleway Managed Database SDK is not configured")
	}
	return awaitScalewayRDBCall(ctx, func() (DatabaseInstance, error) {
		instance, err := client.api.GetInstance(&rdb.GetInstanceRequest{Region: client.region, InstanceID: id}, scw.WithContext(ctx))
		if err != nil {
			return DatabaseInstance{}, err
		}
		return scalewayDatabaseInstance(instance)
	})
}

func (client *scalewayDatabaseSDK) ListInstances(ctx context.Context) ([]DatabaseInstance, error) {
	if client == nil || client.api == nil {
		return nil, errors.New("Scaleway Managed Database SDK is not configured")
	}
	instances := make([]DatabaseInstance, 0)
	for page := int32(1); ; page++ {
		request := &rdb.ListInstancesRequest{Region: client.region, Page: &page}
		if client.project != "" {
			request.ProjectID = &client.project
		}
		response, err := client.api.ListInstances(request, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errors.New("Scaleway Managed Database instance listing returned an empty response")
		}
		for _, instance := range response.Instances {
			mapped, err := scalewayDatabaseInstance(instance)
			if err != nil {
				return nil, err
			}
			instances = append(instances, mapped)
		}
		if len(response.Instances) == 0 || uint32(len(instances)) >= response.TotalCount {
			return instances, nil
		}
	}
}

func (client *scalewayDatabaseSDK) DeleteInstance(ctx context.Context, id string) error {
	if client == nil || client.api == nil {
		return errors.New("Scaleway Managed Database SDK is not configured")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("Scaleway Managed Database instance ID is required")
	}
	_, err := client.api.DeleteInstance(&rdb.DeleteInstanceRequest{Region: client.region, InstanceID: id}, scw.WithContext(ctx))
	return err
}

func (client *scalewayDatabaseSDK) CreateSnapshot(ctx context.Context, instanceID, name string, expiresAt time.Time) (DatabaseSnapshot, error) {
	if client == nil || client.api == nil {
		return DatabaseSnapshot{}, errors.New("Scaleway Managed Database SDK is not configured")
	}
	return awaitScalewayRDBCall(ctx, func() (DatabaseSnapshot, error) {
		snapshot, err := client.api.CreateSnapshot(&rdb.CreateSnapshotRequest{Region: client.region, InstanceID: instanceID, Name: name, ExpiresAt: &expiresAt}, scw.WithContext(ctx))
		if err != nil {
			return DatabaseSnapshot{}, err
		}
		return scalewayDatabaseSnapshot(snapshot)
	})
}

func (client *scalewayDatabaseSDK) GetSnapshot(ctx context.Context, id string) (DatabaseSnapshot, error) {
	if client == nil || client.api == nil {
		return DatabaseSnapshot{}, errors.New("Scaleway Managed Database SDK is not configured")
	}
	return awaitScalewayRDBCall(ctx, func() (DatabaseSnapshot, error) {
		snapshot, err := client.api.GetSnapshot(&rdb.GetSnapshotRequest{Region: client.region, SnapshotID: id}, scw.WithContext(ctx))
		if err != nil {
			return DatabaseSnapshot{}, err
		}
		return scalewayDatabaseSnapshot(snapshot)
	})
}

func (client *scalewayDatabaseSDK) ListSnapshots(ctx context.Context) ([]DatabaseSnapshot, error) {
	if client == nil || client.api == nil {
		return nil, errors.New("Scaleway Managed Database SDK is not configured")
	}
	snapshots := make([]DatabaseSnapshot, 0)
	for page := int32(1); ; page++ {
		request := &rdb.ListSnapshotsRequest{Region: client.region, Page: &page}
		if client.project != "" {
			request.ProjectID = &client.project
		}
		response, err := client.api.ListSnapshots(request, scw.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errors.New("Scaleway Managed Database snapshot listing returned an empty response")
		}
		for _, snapshot := range response.Snapshots {
			mapped, err := scalewayDatabaseSnapshot(snapshot)
			if err != nil {
				return nil, err
			}
			snapshots = append(snapshots, mapped)
		}
		if len(response.Snapshots) == 0 || uint32(len(snapshots)) >= response.TotalCount {
			return snapshots, nil
		}
	}
}

func (client *scalewayDatabaseSDK) DeleteSnapshot(ctx context.Context, id string) error {
	if client == nil || client.api == nil {
		return errors.New("Scaleway Managed Database SDK is not configured")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("Scaleway Managed Database snapshot ID is required")
	}
	_, err := client.api.DeleteSnapshot(&rdb.DeleteSnapshotRequest{Region: client.region, SnapshotID: id}, scw.WithContext(ctx))
	return err
}

func (client *scalewayDatabaseSDK) CreateInstanceFromSnapshot(ctx context.Context, snapshotID, name, nodeType string, isHA bool) (DatabaseInstance, error) {
	if client == nil || client.api == nil {
		return DatabaseInstance{}, errors.New("Scaleway Managed Database SDK is not configured")
	}
	request := &rdb.CreateInstanceFromSnapshotRequest{Region: client.region, SnapshotID: snapshotID, InstanceName: name, IsHaCluster: &isHA}
	if nodeType != "" {
		request.NodeType = &nodeType
	}
	return awaitScalewayRDBCall(ctx, func() (DatabaseInstance, error) {
		instance, err := client.api.CreateInstanceFromSnapshot(request, scw.WithContext(ctx))
		if err != nil {
			return DatabaseInstance{}, err
		}
		return scalewayDatabaseInstance(instance)
	})
}

func (client *scalewayDatabaseSDK) UpdateInstanceTags(ctx context.Context, id string, tags []string) (DatabaseInstance, error) {
	if client == nil || client.api == nil {
		return DatabaseInstance{}, errors.New("Scaleway Managed Database SDK is not configured")
	}
	copyTags := append([]string(nil), tags...)
	return awaitScalewayRDBCall(ctx, func() (DatabaseInstance, error) {
		instance, err := client.api.UpdateInstance(&rdb.UpdateInstanceRequest{Region: client.region, InstanceID: id, Tags: &copyTags}, scw.WithContext(ctx))
		if err != nil {
			return DatabaseInstance{}, err
		}
		return scalewayDatabaseInstance(instance)
	})
}

func scalewayDatabaseInstance(instance *rdb.Instance) (DatabaseInstance, error) {
	if instance == nil {
		return DatabaseInstance{}, errors.New("Scaleway Managed Database API returned an empty instance")
	}
	volumeType := ""
	if instance.Volume != nil {
		volumeType = string(instance.Volume.Type)
	}
	encryptionEnabled := instance.Encryption != nil && instance.Encryption.Enabled
	return DatabaseInstance{ID: instance.ID, Name: instance.Name, Region: string(instance.Region), Status: string(instance.Status), NodeType: instance.NodeType, VolumeType: volumeType, IsHA: instance.IsHaCluster, EncryptionEnabled: encryptionEnabled, Tags: append([]string(nil), instance.Tags...)}, nil
}

func scalewayDatabaseSnapshot(snapshot *rdb.Snapshot) (DatabaseSnapshot, error) {
	if snapshot == nil {
		return DatabaseSnapshot{}, errors.New("Scaleway Managed Database API returned an empty snapshot")
	}
	volumeType := ""
	if snapshot.VolumeType != nil {
		volumeType = string(snapshot.VolumeType.Type)
	}
	expiresAt := time.Time{}
	if snapshot.ExpiresAt != nil {
		expiresAt = *snapshot.ExpiresAt
	}
	createdAt := time.Time{}
	if snapshot.CreatedAt != nil {
		createdAt = *snapshot.CreatedAt
	}
	return DatabaseSnapshot{ID: snapshot.ID, InstanceID: snapshot.InstanceID, Name: snapshot.Name, Region: string(snapshot.Region), Status: string(snapshot.Status), NodeType: snapshot.NodeType, VolumeType: volumeType, CreatedAt: createdAt, ExpiresAt: expiresAt}, nil
}

func awaitScalewayRDBCall[T any](ctx context.Context, call func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		return zero, errors.New("Scaleway Managed Database call context is required")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	wait := databaseMutatingPostTime
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return zero, context.DeadlineExceeded
		}
		if remaining < wait {
			wait = remaining
		}
	}
	type result struct {
		value T
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		value, err := call()
		ch <- result{value: value, err: err}
	}()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res.value, res.err
	case <-ctx.Done():
		// Mutating POSTs can remain open after the resource exists. Return so
		// callers can reconcile by name; the abandoned goroutine ends when the
		// HTTP round-trip finishes or the process exits.
		return zero, ctx.Err()
	case <-timer.C:
		return zero, fmt.Errorf("Scaleway Managed Database call exceeded %s: %w", wait, context.DeadlineExceeded)
	}
}

func firstNonEmptyScaleway(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
