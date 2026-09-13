package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// startRDSCleanup removes only marker-owned recovery snapshots and restored
// resources. The source database is retained because the caller owns its
// lifecycle and must decide separately whether the source may be deleted.
func (api *NativeAPI) startRDSCleanup(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.rds == nil {
		return provider.NativeOperationObservation{}, errors.New("AWS RDS recovery API is required")
	}
	var source rdsResource
	var err error
	if strings.TrimSpace(state.Resource) != "" {
		source, err = parseRDSReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
	} else {
		return provider.NativeOperationObservation{}, errors.New("AWS RDS cleanup requires the source resource reference so it can preserve the source database")
	}
	owned, err := api.listOwnedRDS(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	preservedSourceMembers := make(map[string]struct{})
	if source.Kind == "cluster" {
		clusters, describeErr := api.rds.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String(source.ID)})
		if describeErr != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("describe AWS Aurora source members for cleanup: %w", describeErr)
		}
		if clusters == nil || len(clusters.DBClusters) != 1 {
			return provider.NativeOperationObservation{}, fmt.Errorf("AWS Aurora source cluster %q did not expose one member list", source.ID)
		}
		for _, member := range clusters.DBClusters[0].DBClusterMembers {
			if member.DBInstanceIdentifier != nil && strings.TrimSpace(aws.ToString(member.DBInstanceIdentifier)) != "" {
				preservedSourceMembers[aws.ToString(member.DBInstanceIdentifier)] = struct{}{}
			}
		}
	}
	// Aurora restore inventory contains a cluster member and its cluster. The
	// member must be removed first or AWS rejects cluster deletion while a
	// member is still attached. Keep this ordering provider-local; the core
	// only sees opaque inventory identities.
	sort.SliceStable(owned, func(i, j int) bool {
		return rdsCleanupPriority(owned[i].Identity) < rdsCleanupPriority(owned[j].Identity)
	})
	refs := make([]string, 0, len(owned))
	pending := false
	for _, resource := range owned {
		kind, id, parseErr := parseRDSInventoryIdentity(resource.Identity)
		if parseErr != nil {
			return provider.NativeOperationObservation{}, parseErr
		}
		if (kind == source.Kind && id == source.ID && (kind == "instance" || kind == "cluster")) || (kind == "instance" && source.Kind == "cluster" && hasString(preservedSourceMembers, id)) {
			continue
		}
		refs = append(refs, resource.Identity)
		var deleteErr error
		switch kind {
		case "instance":
			clusterMember, memberErr := awsRDSInstanceBelongsToCluster(ctx, api.rds, id)
			if memberErr != nil && !isRDSNotFound(memberErr) {
				return provider.NativeOperationObservation{}, memberErr
			}
			if !clusterMember {
				_, deleteErr = api.rds.ModifyDBInstance(ctx, &rds.ModifyDBInstanceInput{
					DBInstanceIdentifier: aws.String(id), ApplyImmediately: aws.Bool(true), DeletionProtection: aws.Bool(false),
				})
				if deleteErr != nil && !isRDSNotFound(deleteErr) && !isRDSDeletionPending(deleteErr) {
					return provider.NativeOperationObservation{}, fmt.Errorf("disable deletion protection on owned AWS RDS DB instance %q: %w", id, deleteErr)
				}
				if deleteErr != nil {
					pending = true
					continue
				}
			}
			_, deleteErr = api.rds.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
				DBInstanceIdentifier: aws.String(id), DeleteAutomatedBackups: aws.Bool(true), SkipFinalSnapshot: aws.Bool(true),
			})
		case "cluster":
			_, deleteErr = api.rds.ModifyDBCluster(ctx, &rds.ModifyDBClusterInput{
				DBClusterIdentifier: aws.String(id), ApplyImmediately: aws.Bool(true), DeletionProtection: aws.Bool(false),
			})
			if deleteErr != nil && !isRDSNotFound(deleteErr) && !isRDSDeletionPending(deleteErr) {
				return provider.NativeOperationObservation{}, fmt.Errorf("disable deletion protection on owned AWS RDS DB cluster %q: %w", id, deleteErr)
			}
			if deleteErr != nil {
				pending = true
				continue
			}
			_, deleteErr = api.rds.DeleteDBCluster(ctx, &rds.DeleteDBClusterInput{
				DBClusterIdentifier: aws.String(id), DeleteAutomatedBackups: aws.Bool(true), SkipFinalSnapshot: aws.Bool(true),
			})
		case "snapshot":
			_, deleteErr = api.rds.DeleteDBSnapshot(ctx, &rds.DeleteDBSnapshotInput{DBSnapshotIdentifier: aws.String(id)})
		case "cluster-snapshot":
			_, deleteErr = api.rds.DeleteDBClusterSnapshot(ctx, &rds.DeleteDBClusterSnapshotInput{DBClusterSnapshotIdentifier: aws.String(id)})
		default:
			return provider.NativeOperationObservation{}, fmt.Errorf("unsupported AWS RDS inventory identity %q", resource.Identity)
		}
		if deleteErr != nil && !isRDSNotFound(deleteErr) {
			if isRDSDeletionPending(deleteErr) {
				pending = true
				continue
			}
			return provider.NativeOperationObservation{}, fmt.Errorf("delete owned AWS RDS recovery resource %q: %w", resource.Identity, deleteErr)
		}
		pending = true
	}
	remaining, err := api.listOwnedRDS(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify AWS RDS recovery cleanup: %w", err)
	}
	for _, resource := range remaining {
		kind, id, parseErr := parseRDSInventoryIdentity(resource.Identity)
		if parseErr != nil {
			return provider.NativeOperationObservation{}, parseErr
		}
		if (kind == source.Kind && id == source.ID && (kind == "instance" || kind == "cluster")) || (kind == "instance" && source.Kind == "cluster" && hasString(preservedSourceMembers, id)) {
			continue
		}
		pending = true
	}
	if pending {
		return pendingOperation(state, operationID), nil
	}
	return observationWithOperation(state, operationID, refs, []string{"aws.rds.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func rdsCleanupPriority(identity string) int {
	kind, _, err := parseRDSInventoryIdentity(identity)
	if err != nil {
		return 99
	}
	switch kind {
	case "instance":
		return 0
	case "cluster":
		return 1
	case "snapshot":
		return 2
	case "cluster-snapshot":
		return 3
	default:
		return 99
	}
}

func hasString(values map[string]struct{}, value string) bool {
	_, ok := values[value]
	return ok
}

func awsRDSInstanceBelongsToCluster(ctx context.Context, api RDSAPI, id string) (bool, error) {
	output, err := api.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(id)})
	if err != nil {
		return false, fmt.Errorf("inspect AWS RDS DB instance %q for cluster membership: %w", id, err)
	}
	if output == nil || len(output.DBInstances) != 1 {
		return false, fmt.Errorf("AWS RDS DB instance %q did not expose one resource while cleaning", id)
	}
	return strings.TrimSpace(aws.ToString(output.DBInstances[0].DBClusterIdentifier)) != "", nil
}

func (api *NativeAPI) listOwnedRDS(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("AWS RDS inventory ownership marker is required")
	}
	resources := make([]provider.InventoryResource, 0)
	instances, err := api.listRDSInstances(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		if instance.DBInstanceArn == nil {
			return nil, errors.New("AWS RDS inventory returned a DB instance without an ARN")
		}
		owned, err := api.rdsResourceOwnedStrict(ctx, aws.ToString(instance.DBInstanceArn), marker)
		if err != nil {
			return nil, fmt.Errorf("inspect AWS RDS DB instance ownership: %w", err)
		}
		if !owned {
			continue
		}
		if strings.TrimSpace(aws.ToString(instance.DBInstanceIdentifier)) == "" {
			return nil, errors.New("AWS RDS inventory returned a DB instance without an identifier")
		}
		resources = append(resources, provider.InventoryResource{Identity: "aws-rds://instance/" + aws.ToString(instance.DBInstanceIdentifier), Owned: true, Live: true})
	}
	clusters, err := api.listRDSClusters(ctx)
	if err != nil {
		return nil, err
	}
	for _, cluster := range clusters {
		if cluster.DBClusterArn == nil {
			return nil, errors.New("AWS RDS inventory returned a DB cluster without an ARN")
		}
		owned, err := api.rdsResourceOwnedStrict(ctx, aws.ToString(cluster.DBClusterArn), marker)
		if err != nil {
			return nil, fmt.Errorf("inspect AWS RDS DB cluster ownership: %w", err)
		}
		if !owned {
			continue
		}
		if strings.TrimSpace(aws.ToString(cluster.DBClusterIdentifier)) == "" {
			return nil, errors.New("AWS RDS inventory returned a DB cluster without an identifier")
		}
		resources = append(resources, provider.InventoryResource{Identity: "aws-rds://cluster/" + aws.ToString(cluster.DBClusterIdentifier), Owned: true, Live: true})
	}
	snapshots, err := api.listRDSSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if !rdsTagsMatch(snapshot.TagList, marker, "") {
			continue
		}
		if strings.TrimSpace(aws.ToString(snapshot.DBSnapshotIdentifier)) == "" {
			return nil, errors.New("AWS RDS inventory returned a DB snapshot without an identifier")
		}
		resources = append(resources, provider.InventoryResource{Identity: "aws-rds://snapshot/" + aws.ToString(snapshot.DBSnapshotIdentifier), Owned: true, Live: true})
	}
	clusterSnapshots, err := api.listRDSClusterSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range clusterSnapshots {
		if !rdsTagsMatch(snapshot.TagList, marker, "") {
			continue
		}
		if strings.TrimSpace(aws.ToString(snapshot.DBClusterSnapshotIdentifier)) == "" {
			return nil, errors.New("AWS RDS inventory returned a DB cluster snapshot without an identifier")
		}
		resources = append(resources, provider.InventoryResource{Identity: "aws-rds://cluster-snapshot/" + aws.ToString(snapshot.DBClusterSnapshotIdentifier), Owned: true, Live: true})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (api *NativeAPI) listRDSInstances(ctx context.Context) ([]rdstypes.DBInstance, error) {
	var values []rdstypes.DBInstance
	var next *string
	for {
		output, err := api.rds.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{Marker: next})
		if err != nil {
			return nil, fmt.Errorf("list AWS RDS DB instances: %w", err)
		}
		if output == nil {
			return values, nil
		}
		values = append(values, output.DBInstances...)
		if strings.TrimSpace(aws.ToString(output.Marker)) == "" {
			return values, nil
		}
		next = output.Marker
	}
}

func (api *NativeAPI) listRDSClusters(ctx context.Context) ([]rdstypes.DBCluster, error) {
	var values []rdstypes.DBCluster
	var next *string
	for {
		output, err := api.rds.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{Marker: next})
		if err != nil {
			return nil, fmt.Errorf("list AWS RDS DB clusters: %w", err)
		}
		if output == nil {
			return values, nil
		}
		values = append(values, output.DBClusters...)
		if strings.TrimSpace(aws.ToString(output.Marker)) == "" {
			return values, nil
		}
		next = output.Marker
	}
}

func (api *NativeAPI) listRDSSnapshots(ctx context.Context) ([]rdstypes.DBSnapshot, error) {
	var values []rdstypes.DBSnapshot
	var next *string
	for {
		output, err := api.rds.DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{SnapshotType: aws.String("manual"), Marker: next})
		if err != nil {
			return nil, fmt.Errorf("list AWS RDS DB snapshots: %w", err)
		}
		if output == nil {
			return values, nil
		}
		values = append(values, output.DBSnapshots...)
		if strings.TrimSpace(aws.ToString(output.Marker)) == "" {
			return values, nil
		}
		next = output.Marker
	}
}

func (api *NativeAPI) listRDSClusterSnapshots(ctx context.Context) ([]rdstypes.DBClusterSnapshot, error) {
	var values []rdstypes.DBClusterSnapshot
	var next *string
	for {
		output, err := api.rds.DescribeDBClusterSnapshots(ctx, &rds.DescribeDBClusterSnapshotsInput{SnapshotType: aws.String("manual"), Marker: next})
		if err != nil {
			return nil, fmt.Errorf("list AWS RDS DB cluster snapshots: %w", err)
		}
		if output == nil {
			return values, nil
		}
		values = append(values, output.DBClusterSnapshots...)
		if strings.TrimSpace(aws.ToString(output.Marker)) == "" {
			return values, nil
		}
		next = output.Marker
	}
}

func parseRDSInventoryIdentity(identity string) (kind, id string, err error) {
	identity = strings.TrimSpace(identity)
	for _, candidate := range []struct {
		prefix string
		kind   string
	}{
		{prefix: "aws-rds://instance/", kind: "instance"},
		{prefix: "aws-rds://cluster/", kind: "cluster"},
		{prefix: "aws-rds://snapshot/", kind: "snapshot"},
		{prefix: "aws-rds://cluster-snapshot/", kind: "cluster-snapshot"},
	} {
		if strings.HasPrefix(identity, candidate.prefix) {
			id = strings.TrimPrefix(identity, candidate.prefix)
			if id == "" || strings.ContainsAny(id, "/\r\n\x00") {
				return "", "", fmt.Errorf("AWS RDS inventory identity %q has an invalid resource ID", identity)
			}
			return candidate.kind, id, nil
		}
	}
	return "", "", fmt.Errorf("AWS RDS inventory identity %q is not a recognized provider reference", identity)
}

func isRDSDeletionPending(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invaliddbinstance_state") || strings.Contains(message, "invalid db instance state") || strings.Contains(message, "invaliddbclusterstate") || strings.Contains(message, "invalid db cluster state") || strings.Contains(message, "snapshot is not in available") || strings.Contains(message, "in progress")
}
