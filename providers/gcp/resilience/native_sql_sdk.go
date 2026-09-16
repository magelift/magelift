package resilience

import (
	"context"
	"strings"
	"time"

	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

type cloudSQLSDK struct {
	service *sqladmin.Service
}

func newCloudSQLSDK(ctx context.Context) (CloudSQLAPI, error) {
	service, err := sqladmin.NewService(ctx)
	if err != nil {
		return nil, err
	}
	return cloudSQLSDK{service: service}, nil
}

// NewCloudSQLAPI builds the Cloud SQL port from Application Default
// Credentials. The CLI cleanup provider uses it to inventory and delete
// ledger-claimed instances and backups.
func NewCloudSQLAPI(ctx context.Context) (CloudSQLAPI, error) {
	return newCloudSQLSDK(ctx)
}

func (s cloudSQLSDK) CreateBackup(ctx context.Context, project, instance, description string, ttlDays int64) (CloudSQLOperation, error) {
	// Standard Cloud SQL on-demand backups use the documented backupRuns.insert
	// path in the current service surface. The newer backups.createBackup
	// collection returns 404 for standard MySQL instances in real projects.
	// Standard on-demand backups are provider-retained, so ttlDays is retained
	// in the provider-neutral port for other implementations but is not sent to
	// this legacy-compatible API.
	_ = ttlDays
	operation, err := s.service.BackupRuns.Insert(project, instance, &sqladmin.BackupRun{Description: description}).Context(ctx).Do()
	return mapCloudSQLOperation(operation), err
}

func (s cloudSQLSDK) DeleteBackup(ctx context.Context, name string) (CloudSQLOperation, error) {
	operation, err := s.service.Backups.DeleteBackup(name).Context(ctx).Do()
	return mapCloudSQLOperation(operation), err
}

func (s cloudSQLSDK) ListBackups(ctx context.Context, project string) ([]CloudSQLBackup, error) {
	backups := make([]CloudSQLBackup, 0)
	err := s.service.Backups.ListBackups("projects/"+project).Pages(ctx, func(response *sqladmin.ListBackupsResponse) error {
		for _, backup := range response.Backups {
			backups = append(backups, mapCloudSQLBackup(backup))
		}
		return nil
	})
	return backups, err
}

func (s cloudSQLSDK) GetBackup(ctx context.Context, name string) (CloudSQLBackup, error) {
	backup, err := s.service.Backups.GetBackup(name).Context(ctx).Do()
	return mapCloudSQLBackup(backup), err
}

func (s cloudSQLSDK) RestoreBackup(ctx context.Context, project, target, backup string, settings CloudSQLRestoreSettings) (CloudSQLOperation, error) {
	request := &sqladmin.InstancesRestoreBackupRequest{Backup: backup}
	if settings.DatabaseVersion != "" || settings.Tier != "" || settings.Region != "" || settings.AvailabilityType != "" || settings.KMSKey != "" || settings.DeletionProtection || len(settings.UserLabels) > 0 {
		request.RestoreInstanceSettings = &sqladmin.DatabaseInstance{DatabaseVersion: settings.DatabaseVersion, Region: settings.Region, Settings: &sqladmin.Settings{Tier: settings.Tier, AvailabilityType: settings.AvailabilityType, DeletionProtectionEnabled: settings.DeletionProtection, UserLabels: cloneMetadata(settings.UserLabels)}}
		if settings.KMSKey != "" {
			request.RestoreInstanceSettings.DiskEncryptionConfiguration = &sqladmin.DiskEncryptionConfiguration{KmsKeyName: settings.KMSKey}
		}
	}
	operation, err := s.service.Instances.RestoreBackup(project, target, request).Context(ctx).Do()
	return mapCloudSQLOperation(operation), err
}

func (s cloudSQLSDK) GetInstance(ctx context.Context, project, instance string) (CloudSQLInstance, error) {
	value, err := s.service.Instances.Get(project, instance).Context(ctx).Do()
	return mapCloudSQLInstance(value), err
}

func (s cloudSQLSDK) ListInstances(ctx context.Context, project string) ([]CloudSQLInstance, error) {
	instances := make([]CloudSQLInstance, 0)
	err := s.service.Instances.List(project).Pages(ctx, func(response *sqladmin.InstancesListResponse) error {
		for _, instance := range response.Items {
			instances = append(instances, mapCloudSQLInstance(instance))
		}
		return nil
	})
	return instances, err
}

func (s cloudSQLSDK) DeleteInstance(ctx context.Context, project, instance string) (CloudSQLOperation, error) {
	operation, err := s.service.Instances.Delete(project, instance).Context(ctx).Do()
	return mapCloudSQLOperation(operation), err
}

func (s cloudSQLSDK) GetOperation(ctx context.Context, project, operation string) (CloudSQLOperation, error) {
	operation = strings.Trim(operation, "/")
	if strings.HasPrefix(operation, "projects/") {
		parts := strings.Split(operation, "/")
		if len(parts) == 4 && parts[0] == "projects" && parts[2] == "operations" {
			project = parts[1]
			operation = parts[3]
		}
	}
	value, err := s.service.Operations.Get(project, operation).Context(ctx).Do()
	return mapCloudSQLOperation(value), err
}

func mapCloudSQLBackup(value *sqladmin.Backup) CloudSQLBackup {
	if value == nil {
		return CloudSQLBackup{}
	}
	return CloudSQLBackup{Name: value.Name, Instance: value.Instance, Description: value.Description, Type: value.Type, State: firstNonEmpty(value.State, value.Type), KMSKey: value.KmsKey, ExpiryTime: parseSQLTime(value.ExpiryTime)}
}

func mapCloudSQLInstance(value *sqladmin.DatabaseInstance) CloudSQLInstance {
	if value == nil {
		return CloudSQLInstance{}
	}
	instance := CloudSQLInstance{Project: value.Project, Name: value.Name, Region: value.Region, State: value.State, DatabaseVersion: value.DatabaseVersion, UserLabels: cloneMetadata(nil), WriteEndpoint: value.WriteEndpoint}
	if value.DiskEncryptionConfiguration != nil {
		instance.KMSKey = value.DiskEncryptionConfiguration.KmsKeyName
	}
	if value.Settings != nil {
		instance.Tier = value.Settings.Tier
		instance.AvailabilityType = value.Settings.AvailabilityType
		instance.DeletionProtection = value.Settings.DeletionProtectionEnabled
		instance.UserLabels = cloneMetadata(value.Settings.UserLabels)
		instance.BackupsEnabled = value.Settings.BackupConfiguration != nil && value.Settings.BackupConfiguration.Enabled
	}
	return instance
}

func mapCloudSQLOperation(value *sqladmin.Operation) CloudSQLOperation {
	if value == nil {
		return CloudSQLOperation{}
	}
	operation := CloudSQLOperation{Name: value.Name, Status: mapSQLStatus(value.Status), ResourceName: value.TargetLink}
	if value.BackupContext != nil {
		operation.ResourceName = value.BackupContext.Name
	}
	if value.Error != nil && len(value.Error.Errors) > 0 {
		operation.Error = value.Error.Errors[0].Message
	}
	if operation.ResourceName == "" && value.TargetId != "" {
		operation.ResourceName = value.TargetId
	}
	return operation
}

func mapSQLStatus(status string) string {
	switch strings.ToUpper(status) {
	case "PENDING", "ENQUEUED":
		return "pending"
	case "RUNNING":
		return "running"
	case "DONE", "SUCCESSFUL", "COMPLETED":
		return "succeeded"
	case "FAILED", "ERROR":
		return "failed"
	default:
		return strings.ToLower(status)
	}
}

func parseSQLTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
