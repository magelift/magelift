package resilience

import (
	"context"
	"errors"
	"fmt"
	"strings"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type observedRDSResource struct {
	Resource          rdsResource
	ARN               string
	Status            string
	Engine            string
	EngineVersion     string
	Encrypted         bool
	DeletionProtected bool
	DBSubnetGroup     string
	InstanceClass     string
}

func (api *NativeAPI) startDatabase(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.rds == nil {
		return provider.NativeOperationObservation{}, errors.New("AWS RDS recovery API is required")
	}
	if err := validateRDSDestination(state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.startRDSCleanup(ctx, state, operationID)
	}
	resource, err := parseRDSReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.startRDSBackup(ctx, state, operationID, resource)
	case sdk.ResilienceRestore:
		return api.startRDSRestore(ctx, state, operationID, resource)
	case sdk.ResilienceIntegrityCheck:
		return api.startRDSIntegrity(ctx, state, operationID, resource)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS RDS recovery does not implement this action")
	}
}

func (api *NativeAPI) pollDatabase(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if err := validateRDSDestination(state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceCleanup:
		return api.startRDSCleanup(ctx, state, operationID)
	case sdk.ResilienceBackup:
		return api.pollRDSBackup(ctx, state, operationID)
	case sdk.ResilienceRestore:
		return api.pollRDSRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		resource, err := parseRDSReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.startRDSIntegrity(ctx, state, operationID, resource)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS RDS recovery does not implement this action")
	}
}

func (api *NativeAPI) startRDSBackup(ctx context.Context, state operationState, operationID string, resource rdsResource) (provider.NativeOperationObservation, error) {
	observed, err := api.observeRDSResource(ctx, resource, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if observed.Status != "available" {
		return provider.NativeOperationObservation{}, fmt.Errorf("AWS RDS %s %q is not available for backup: %s", resource.Kind, resource.ID, observed.Status)
	}
	if !observed.Encrypted {
		return provider.NativeOperationObservation{}, errors.New("refusing to back up an unencrypted AWS RDS resource")
	}
	if !observed.DeletionProtected {
		return provider.NativeOperationObservation{}, errors.New("refusing to back up an AWS RDS resource without deletion protection")
	}
	if resource.Kind == "instance" {
		snapshotID := rdsSnapshotID("magelift-snapshot-", state)
		snapshot, err := api.describeDBSnapshot(ctx, snapshotID)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		if snapshot != nil {
			if !rdsTagsMatch(snapshot.TagList, state.OwnershipMarker, state.DataClass) {
				return provider.NativeOperationObservation{}, errors.New("AWS RDS snapshot exists but is not owned by this operation")
			}
			if strings.EqualFold(aws.ToString(snapshot.Status), "available") {
				return rdsBackupObservation(api, state, operationID, snapshotID, false, snapshot.Encrypted), nil
			}
			return pendingOperation(state, operationID), nil
		}
		_, err = api.rds.CreateDBSnapshot(ctx, &rds.CreateDBSnapshotInput{
			DBInstanceIdentifier: aws.String(resource.ID), DBSnapshotIdentifier: aws.String(snapshotID), Tags: tagsFor(state),
		})
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("create AWS RDS DB snapshot: %w", err)
		}
		return pendingOperation(state, operationID), nil
	}
	clusterSnapshotID := rdsSnapshotID("magelift-cluster-snapshot-", state)
	snapshot, err := api.describeDBClusterSnapshot(ctx, clusterSnapshotID)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if snapshot != nil {
		if !rdsTagsMatch(snapshot.TagList, state.OwnershipMarker, state.DataClass) {
			return provider.NativeOperationObservation{}, errors.New("AWS RDS cluster snapshot exists but is not owned by this operation")
		}
		if strings.EqualFold(aws.ToString(snapshot.Status), "available") {
			return rdsBackupObservation(api, state, operationID, clusterSnapshotID, true, snapshot.StorageEncrypted), nil
		}
		return pendingOperation(state, operationID), nil
	}
	_, err = api.rds.CreateDBClusterSnapshot(ctx, &rds.CreateDBClusterSnapshotInput{
		DBClusterIdentifier: aws.String(resource.ID), DBClusterSnapshotIdentifier: aws.String(clusterSnapshotID), Tags: tagsFor(state),
	})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("create AWS RDS DB cluster snapshot: %w", err)
	}
	return pendingOperation(state, operationID), nil
}

func (api *NativeAPI) pollRDSBackup(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	resource, err := parseRDSReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if resource.Kind == "instance" {
		snapshotID := rdsSnapshotID("magelift-snapshot-", state)
		snapshot, err := api.describeDBSnapshot(ctx, snapshotID)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		if snapshot == nil || !rdsTagsMatch(snapshot.TagList, state.OwnershipMarker, state.DataClass) {
			return pendingOperation(state, operationID), nil
		}
		switch strings.ToLower(aws.ToString(snapshot.Status)) {
		case "available":
			return rdsBackupObservation(api, state, operationID, snapshotID, false, snapshot.Encrypted), nil
		case "failed", "deleted":
			return failedOperation(state, operationID, "AWS RDS DB snapshot reported a terminal failure"), nil
		default:
			return pendingOperation(state, operationID), nil
		}
	}
	snapshotID := rdsSnapshotID("magelift-cluster-snapshot-", state)
	snapshot, err := api.describeDBClusterSnapshot(ctx, snapshotID)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if snapshot == nil || !rdsTagsMatch(snapshot.TagList, state.OwnershipMarker, state.DataClass) {
		return pendingOperation(state, operationID), nil
	}
	switch strings.ToLower(aws.ToString(snapshot.Status)) {
	case "available":
		return rdsBackupObservation(api, state, operationID, snapshotID, true, snapshot.StorageEncrypted), nil
	case "failed", "deleted":
		return failedOperation(state, operationID, "AWS RDS DB cluster snapshot reported a terminal failure"), nil
	default:
		return pendingOperation(state, operationID), nil
	}
}

func rdsBackupObservation(api *NativeAPI, state operationState, operationID, snapshotID string, cluster bool, encrypted *bool) provider.NativeOperationObservation {
	scheme := "aws-rds://snapshot/"
	if cluster {
		scheme = "aws-rds://cluster-snapshot/"
	}
	backupID := scheme + snapshotID
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: aws.ToBool(encrypted), ProtectionVerified: true,
		ServiceHealthVerified: true,
		Reason:                "verified an ownership-tagged encrypted AWS RDS snapshot is available",
	}
	return observationWithOperation(state, operationID, []string{backupID}, []string{"aws.rds.snapshot", "aws.rds.encryption", "aws.rds.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func (api *NativeAPI) startRDSRestore(ctx context.Context, state operationState, _ string, source rdsResource) (provider.NativeOperationObservation, error) {
	observed, err := api.observeRDSResource(ctx, source, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if !observed.Encrypted {
		return provider.NativeOperationObservation{}, errors.New("refusing to restore from an unencrypted AWS RDS source")
	}
	backupKind, backupID, err := parseRDSBackupReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if (source.Kind == "instance") != (backupKind == "snapshot") {
		return provider.NativeOperationObservation{}, errors.New("AWS RDS restore backup type does not match the source resource type")
	}
	target := rdsResource{Kind: source.Kind, ID: rdsRestoreID(source, state)}
	state.Target = rdsReference(target)
	operationID, err := encodeOperationState(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if target.Kind == "instance" {
		current, err := api.observeRDSResource(ctx, target, "")
		if err == nil {
			if current.ARN == "" || !api.rdsResourceOwned(ctx, current.ARN, state.OwnershipMarker) {
				return provider.NativeOperationObservation{}, errors.New("refusing to reuse an unowned AWS RDS restore instance")
			}
			if strings.EqualFold(current.Status, "available") {
				return api.rdsRestoreObservation(state, operationID, target, current), nil
			}
			return pendingOperation(state, operationID), nil
		}
		if !isRDSNotFound(err) {
			return provider.NativeOperationObservation{}, err
		}
		snapshot, err := api.describeDBSnapshot(ctx, backupID)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		if snapshot == nil || !rdsTagsMatch(snapshot.TagList, state.OwnershipMarker, state.DataClass) || !strings.EqualFold(aws.ToString(snapshot.Status), "available") {
			return provider.NativeOperationObservation{}, errors.New("AWS RDS restore snapshot is missing, unowned, or not available")
		}
		input := &rds.RestoreDBInstanceFromDBSnapshotInput{
			DBInstanceIdentifier: aws.String(target.ID), DBSnapshotIdentifier: aws.String(backupID),
			DeletionProtection: aws.Bool(true), CopyTagsToSnapshot: aws.Bool(true),
			PubliclyAccessible: aws.Bool(api.config.RestorePubliclyAccessible), Tags: tagsFor(state),
		}
		if strings.TrimSpace(api.config.RestoreDBInstanceClass) != "" {
			input.DBInstanceClass = aws.String(api.config.RestoreDBInstanceClass)
		}
		if strings.TrimSpace(api.config.RestoreDBSubnetGroup) != "" {
			input.DBSubnetGroupName = aws.String(api.config.RestoreDBSubnetGroup)
		}
		if len(api.config.RestoreVPCSecurityGroupIDs) > 0 {
			input.VpcSecurityGroupIds = append([]string(nil), api.config.RestoreVPCSecurityGroupIDs...)
		}
		_, err = api.rds.RestoreDBInstanceFromDBSnapshot(ctx, input)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("restore AWS RDS DB instance: %w", err)
		}
		return pendingOperation(state, operationID), nil
	}
	return api.startAuroraRestore(ctx, state, operationID, target, backupID)
}

func (api *NativeAPI) startAuroraRestore(ctx context.Context, state operationState, operationID string, target rdsResource, backupID string) (provider.NativeOperationObservation, error) {
	current, err := api.observeRDSResource(ctx, target, "")
	if err == nil {
		if current.ARN == "" || !api.rdsResourceOwned(ctx, current.ARN, state.OwnershipMarker) {
			return provider.NativeOperationObservation{}, errors.New("refusing to reuse an unowned AWS Aurora restore cluster")
		}
		return api.pollAuroraRestore(ctx, state, operationID, target, current)
	}
	if !isRDSNotFound(err) {
		return provider.NativeOperationObservation{}, err
	}
	snapshot, err := api.describeDBClusterSnapshot(ctx, backupID)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if snapshot == nil || !rdsTagsMatch(snapshot.TagList, state.OwnershipMarker, state.DataClass) || !strings.EqualFold(aws.ToString(snapshot.Status), "available") || strings.TrimSpace(aws.ToString(snapshot.Engine)) == "" {
		return provider.NativeOperationObservation{}, errors.New("AWS Aurora restore snapshot is missing, unowned, or not available")
	}
	input := &rds.RestoreDBClusterFromSnapshotInput{
		DBClusterIdentifier: aws.String(target.ID), SnapshotIdentifier: aws.String(backupID), Engine: snapshot.Engine,
		DeletionProtection: aws.Bool(true), CopyTagsToSnapshot: aws.Bool(true), Tags: tagsFor(state),
		BackupRetentionPeriod: aws.Int32(int32(api.retentionDays())),
	}
	if strings.TrimSpace(api.config.RestoreAuroraSubnetGroup) != "" {
		input.DBSubnetGroupName = aws.String(api.config.RestoreAuroraSubnetGroup)
	} else if strings.TrimSpace(api.config.RestoreDBSubnetGroup) != "" {
		input.DBSubnetGroupName = aws.String(api.config.RestoreDBSubnetGroup)
	}
	if len(api.config.RestoreVPCSecurityGroupIDs) > 0 {
		input.VpcSecurityGroupIds = append([]string(nil), api.config.RestoreVPCSecurityGroupIDs...)
	}
	_, err = api.rds.RestoreDBClusterFromSnapshot(ctx, input)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("restore AWS Aurora DB cluster: %w", err)
	}
	return pendingOperation(state, operationID), nil
}

func (api *NativeAPI) pollRDSRestore(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	target, err := parseRDSReference(state.Target)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	current, err := api.observeRDSResource(ctx, target, "")
	if err != nil {
		if isRDSNotFound(err) {
			return pendingOperation(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	if current.ARN == "" || !api.rdsResourceOwned(ctx, current.ARN, state.OwnershipMarker) {
		return provider.NativeOperationObservation{}, errors.New("AWS RDS restore resource is not owned by this operation")
	}
	if target.Kind == "cluster" {
		return api.pollAuroraRestore(ctx, state, operationID, target, current)
	}
	switch strings.ToLower(current.Status) {
	case "available":
		return api.rdsRestoreObservation(state, operationID, target, current), nil
	case "failed", "deleting", "deleted":
		return failedOperation(state, operationID, "AWS RDS restored DB instance reported a terminal failure"), nil
	default:
		return pendingOperation(state, operationID), nil
	}
}

func (api *NativeAPI) pollAuroraRestore(ctx context.Context, state operationState, operationID string, target rdsResource, cluster observedRDSResource) (provider.NativeOperationObservation, error) {
	if !strings.EqualFold(cluster.Status, "available") {
		if strings.EqualFold(cluster.Status, "failed") {
			return failedOperation(state, operationID, "AWS Aurora restored DB cluster reported a terminal failure"), nil
		}
		return pendingOperation(state, operationID), nil
	}
	instanceID := target.ID + "-instance"
	instance, err := api.observeRDSResource(ctx, rdsResource{Kind: "instance", ID: instanceID}, "")
	if err != nil {
		if !isRDSNotFound(err) {
			return provider.NativeOperationObservation{}, err
		}
		class := api.config.RestoreAuroraInstanceClass
		if strings.TrimSpace(class) == "" {
			class = api.config.RestoreDBInstanceClass
		}
		if strings.TrimSpace(class) == "" {
			return provider.NativeOperationObservation{}, errors.New("AWS Aurora restore requires RestoreAuroraInstanceClass or RestoreDBInstanceClass")
		}
		input := &rds.CreateDBInstanceInput{
			DBInstanceIdentifier: aws.String(instanceID), DBInstanceClass: aws.String(class), Engine: aws.String(cluster.Engine),
			DBClusterIdentifier: aws.String(target.ID), PubliclyAccessible: aws.Bool(api.config.RestorePubliclyAccessible), Tags: tagsFor(state),
		}
		if strings.TrimSpace(api.config.RestoreAuroraSubnetGroup) != "" {
			input.DBSubnetGroupName = aws.String(api.config.RestoreAuroraSubnetGroup)
		}
		if _, err := api.rds.CreateDBInstance(ctx, input); err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("create AWS Aurora restore DB instance: %w", err)
		}
		return pendingOperation(state, operationID), nil
	}
	if instance.ARN == "" || !api.rdsResourceOwned(ctx, instance.ARN, state.OwnershipMarker) {
		return provider.NativeOperationObservation{}, errors.New("AWS Aurora restore DB instance is not owned by this operation")
	}
	if strings.EqualFold(instance.Status, "available") {
		return api.rdsRestoreObservation(state, operationID, rdsResource{Kind: "cluster", ID: target.ID}, observedRDSResource{Resource: target, ARN: cluster.ARN, Status: "available", Engine: cluster.Engine, Encrypted: cluster.Encrypted, DeletionProtected: cluster.DeletionProtected}), nil
	}
	return pendingOperation(state, operationID), nil
}

func (api *NativeAPI) startRDSIntegrity(ctx context.Context, state operationState, operationID string, resource rdsResource) (provider.NativeOperationObservation, error) {
	observed, err := api.observeRDSResource(ctx, resource, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if !strings.EqualFold(observed.Status, "available") || !observed.Encrypted || !observed.DeletionProtected {
		return provider.NativeOperationObservation{}, errors.New("AWS RDS integrity check requires an available encrypted deletion-protected resource")
	}
	if api.config.Verifier == nil {
		return provider.NativeOperationObservation{}, errors.New("AWS RDS integrity check requires an application-owned recovery verifier")
	}
	verification, err := api.config.Verifier.Verify(ctx, RecoveryVerificationRequest{DataClass: state.DataClass, Resource: rdsReference(resource), FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify AWS RDS recovery fixture: %w", err)
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup,
		FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: observed.Encrypted,
		ProtectionVerified: observed.DeletionProtected, ManifestVerified: verification.ManifestVerified,
		CountsVerified: verification.CountsVerified, ApplicationReadsVerified: verification.ApplicationReadsVerified,
		PermissionsVerified: verification.PermissionsVerified, SecretReferencesVerified: verification.SecretReferencesVerified,
		ServiceHealthVerified: verification.ServiceHealthVerified || strings.EqualFold(observed.Status, "available"),
		Reason:                "verified RDS control-plane health and application-owned recovery fixture",
	}
	return observationWithOperation(state, operationID, []string{rdsReference(resource)}, []string{"aws.rds.integrity", "aws.rds.encryption", "aws.rds.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) rdsRestoreObservation(state operationState, operationID string, target rdsResource, observed observedRDSResource) provider.NativeOperationObservation {
	backupID := state.Backup
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, RestoreID: rdsReference(target),
		FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: observed.Encrypted,
		ProtectionVerified: observed.DeletionProtected, ServiceHealthVerified: strings.EqualFold(observed.Status, "available"),
		Reason: "restored an ownership-scoped RDS resource and verified the provider health state",
	}
	return observationWithOperation(state, operationID, []string{rdsReference(target)}, []string{"aws.rds.restore", "aws.rds.encryption", "aws.rds.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func (api *NativeAPI) observeRDSResource(ctx context.Context, resource rdsResource, marker string) (observedRDSResource, error) {
	if resource.Kind == "instance" {
		output, err := api.rds.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(resource.ID)})
		if err != nil {
			return observedRDSResource{}, fmt.Errorf("describe AWS RDS DB instance %q: %w", resource.ID, err)
		}
		if output == nil || len(output.DBInstances) != 1 {
			return observedRDSResource{}, fmt.Errorf("AWS RDS DB instance %q was not found", resource.ID)
		}
		instance := output.DBInstances[0]
		observed := observedRDSResource{Resource: resource, ARN: aws.ToString(instance.DBInstanceArn), Status: aws.ToString(instance.DBInstanceStatus), Engine: aws.ToString(instance.Engine), EngineVersion: aws.ToString(instance.EngineVersion), Encrypted: aws.ToBool(instance.StorageEncrypted), DeletionProtected: aws.ToBool(instance.DeletionProtection), InstanceClass: aws.ToString(instance.DBInstanceClass)}
		if instance.DBSubnetGroup != nil {
			observed.DBSubnetGroup = aws.ToString(instance.DBSubnetGroup.DBSubnetGroupName)
		}
		if strings.TrimSpace(marker) != "" {
			if observed.ARN == "" || !api.rdsResourceOwned(ctx, observed.ARN, marker) {
				return observedRDSResource{}, errors.New("AWS RDS DB instance is not owned by the requested marker")
			}
		}
		return observed, nil
	}
	output, err := api.rds.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String(resource.ID)})
	if err != nil {
		return observedRDSResource{}, fmt.Errorf("describe AWS RDS DB cluster %q: %w", resource.ID, err)
	}
	if output == nil || len(output.DBClusters) != 1 {
		return observedRDSResource{}, fmt.Errorf("AWS RDS DB cluster %q was not found", resource.ID)
	}
	cluster := output.DBClusters[0]
	observed := observedRDSResource{Resource: resource, ARN: aws.ToString(cluster.DBClusterArn), Status: aws.ToString(cluster.Status), Engine: aws.ToString(cluster.Engine), EngineVersion: aws.ToString(cluster.EngineVersion), Encrypted: aws.ToBool(cluster.StorageEncrypted), DeletionProtected: aws.ToBool(cluster.DeletionProtection), DBSubnetGroup: aws.ToString(cluster.DBSubnetGroup), InstanceClass: aws.ToString(cluster.DBClusterInstanceClass)}
	if strings.TrimSpace(marker) != "" {
		if observed.ARN == "" || !api.rdsResourceOwned(ctx, observed.ARN, marker) {
			return observedRDSResource{}, errors.New("AWS RDS DB cluster is not owned by the requested marker")
		}
	}
	return observed, nil
}

func (api *NativeAPI) rdsResourceOwned(ctx context.Context, arn, marker string) bool {
	owned, err := api.rdsResourceOwnedStrict(ctx, arn, marker)
	return err == nil && owned
}

func (api *NativeAPI) rdsResourceOwnedStrict(ctx context.Context, arn, marker string) (bool, error) {
	if strings.TrimSpace(arn) == "" {
		return false, errors.New("AWS RDS resource ARN is required for ownership inspection")
	}
	output, err := api.rds.ListTagsForResource(ctx, &rds.ListTagsForResourceInput{ResourceName: aws.String(arn)})
	if err != nil {
		return false, err
	}
	return output != nil && rdsTagsMatch(output.TagList, marker, ""), nil
}

func rdsTagsMatch(tags []rdstypes.Tag, marker, class string) bool {
	if strings.TrimSpace(tagValue(tags, ownershipTagKey)) == "" || tagValue(tags, ownershipTagKey) != marker {
		return false
	}
	return class == "" || tagValue(tags, classTagKey) == class
}

func rdsSnapshotID(prefix string, state operationState) string {
	return strings.ToLower(prefix + shortDigest(state.OwnershipMarker+"\x00"+state.IdempotencyKey+"\x00"+state.Resource))
}

func rdsRestoreID(source rdsResource, state operationState) string {
	prefix := "magelift-restore-"
	if source.Kind == "cluster" {
		prefix = "magelift-cluster-restore-"
	}
	return strings.ToLower(prefix + shortDigest(source.ID+"\x00"+state.OwnershipMarker+"\x00"+state.IdempotencyKey))
}

func parseRDSBackupReference(reference string) (kind, id string, err error) {
	reference = strings.TrimSpace(reference)
	for _, prefix := range []string{"aws-rds://snapshot/", "rds://snapshot/"} {
		if strings.HasPrefix(reference, prefix) && strings.TrimSpace(strings.TrimPrefix(reference, prefix)) != "" {
			return "snapshot", strings.TrimPrefix(reference, prefix), nil
		}
	}
	for _, prefix := range []string{"aws-rds://cluster-snapshot/", "rds://cluster-snapshot/"} {
		if strings.HasPrefix(reference, prefix) && strings.TrimSpace(strings.TrimPrefix(reference, prefix)) != "" {
			return "cluster-snapshot", strings.TrimPrefix(reference, prefix), nil
		}
	}
	return "", "", fmt.Errorf("AWS RDS backup reference %q must identify an owned snapshot", reference)
}

func (api *NativeAPI) describeDBSnapshot(ctx context.Context, id string) (*rdstypes.DBSnapshot, error) {
	output, err := api.rds.DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{DBSnapshotIdentifier: aws.String(id), SnapshotType: aws.String("manual")})
	if err != nil {
		if isRDSNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("describe AWS RDS DB snapshot %q: %w", id, err)
	}
	if output == nil || len(output.DBSnapshots) == 0 {
		return nil, nil
	}
	return &output.DBSnapshots[0], nil
}

func (api *NativeAPI) describeDBClusterSnapshot(ctx context.Context, id string) (*rdstypes.DBClusterSnapshot, error) {
	output, err := api.rds.DescribeDBClusterSnapshots(ctx, &rds.DescribeDBClusterSnapshotsInput{DBClusterSnapshotIdentifier: aws.String(id), SnapshotType: aws.String("manual")})
	if err != nil {
		if isRDSNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("describe AWS RDS DB cluster snapshot %q: %w", id, err)
	}
	if output == nil || len(output.DBClusterSnapshots) == 0 {
		return nil, nil
	}
	return &output.DBClusterSnapshots[0], nil
}

func isRDSNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "notfound") || strings.Contains(message, "not found") || strings.Contains(message, "does not exist") || strings.Contains(message, "404")
}

func validateRDSDestination(state operationState) error {
	switch state.Destination {
	case "", sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated:
		return nil
	default:
		return capabilityError(state, "AWS RDS recovery supports only same-region and same-region-isolated destinations; cross-region snapshot copy and restore are not implemented")
	}
}
