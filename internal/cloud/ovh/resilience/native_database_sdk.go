package resilience

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ovh/go-ovh/ovh"
)

// ovhDatabaseSDK keeps the generic OVH API client and its response models at
// the provider boundary. The Public Cloud Database API is deliberately used
// through documented REST paths because go-ovh is a transport wrapper rather
// than a service-specific generated client.
type ovhDatabaseSDK struct {
	client  *ovh.Client
	project string
	config  NativeAPIConfig
}

func newOVHDatabaseSDK(client *ovh.Client, config NativeAPIConfig) *ovhDatabaseSDK {
	if client == nil || strings.TrimSpace(config.DatabaseProjectID) == "" {
		return nil
	}
	return &ovhDatabaseSDK{client: client, project: strings.TrimSpace(config.DatabaseProjectID), config: config}
}

func (api *ovhDatabaseSDK) GetInstance(ctx context.Context, engine, id string) (DatabaseInstance, error) {
	if err := api.validate(ctx, id); err != nil {
		return DatabaseInstance{}, err
	}
	engine, err := api.normalizeEngine(engine)
	if err != nil {
		return DatabaseInstance{}, err
	}
	var response ovhDatabaseService
	if err := api.client.GetWithContext(ctx, api.instancePath(engine, id), &response); err != nil {
		return DatabaseInstance{}, err
	}
	return api.instance(response, id, engine)
}

func (api *ovhDatabaseSDK) ListInstances(ctx context.Context, engine string) ([]DatabaseInstance, error) {
	if err := api.validate(ctx, "list"); err != nil {
		return nil, err
	}
	engine, err := api.normalizeEngine(engine)
	if err != nil {
		return nil, err
	}
	ids, err := api.listIdentities(ctx, api.enginePath(engine))
	if err != nil {
		return nil, err
	}
	instances := make([]DatabaseInstance, 0, len(ids))
	for _, id := range ids {
		instance, err := api.GetInstance(ctx, engine, id)
		if err != nil {
			return nil, fmt.Errorf("get OVHcloud database instance %q: %w", id, err)
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

func (api *ovhDatabaseSDK) DeleteInstance(ctx context.Context, engine, id string) error {
	if err := api.validate(ctx, id); err != nil {
		return err
	}
	engine, err := api.normalizeEngine(engine)
	if err != nil {
		return err
	}
	return api.client.DeleteWithContext(ctx, api.instancePath(engine, id), nil)
}

func (api *ovhDatabaseSDK) ListBackups(ctx context.Context, engine, instanceID string) ([]DatabaseBackup, error) {
	if err := api.validate(ctx, instanceID); err != nil {
		return nil, err
	}
	engine, err := api.normalizeEngine(engine)
	if err != nil {
		return nil, err
	}
	ids, err := api.listIdentities(ctx, api.backupListPath(engine, instanceID))
	if err != nil {
		return nil, err
	}
	backups := make([]DatabaseBackup, 0, len(ids))
	for _, id := range ids {
		backup, err := api.GetBackup(ctx, engine, instanceID, id)
		if err != nil {
			return nil, fmt.Errorf("get OVHcloud database backup %q: %w", id, err)
		}
		backups = append(backups, backup)
	}
	return backups, nil
}

func (api *ovhDatabaseSDK) GetBackup(ctx context.Context, engine, instanceID, backupID string) (DatabaseBackup, error) {
	if err := api.validate(ctx, instanceID+"/"+backupID); err != nil {
		return DatabaseBackup{}, err
	}
	engine, err := api.normalizeEngine(engine)
	if err != nil {
		return DatabaseBackup{}, err
	}
	var response ovhDatabaseBackup
	if err := api.client.GetWithContext(ctx, api.backupPath(engine, instanceID, backupID), &response); err != nil {
		return DatabaseBackup{}, err
	}
	return api.backup(response, instanceID, engine)
}

func (api *ovhDatabaseSDK) CreateInstanceFromBackup(ctx context.Context, request DatabaseRestoreRequest) (DatabaseInstance, error) {
	if err := api.validate(ctx, request.InstanceID); err != nil {
		return DatabaseInstance{}, err
	}
	engine, err := api.normalizeEngine(request.Engine)
	if err != nil {
		return DatabaseInstance{}, err
	}
	if strings.TrimSpace(request.Description) == "" || strings.TrimSpace(request.Region) == "" || (strings.TrimSpace(request.BackupID) == "" && request.PointInTime == nil) {
		return DatabaseInstance{}, errors.New("OVHcloud database restore requires description, region, and a backup identity or point in time")
	}
	if strings.TrimSpace(request.BackupID) != "" && request.PointInTime != nil {
		return DatabaseInstance{}, errors.New("OVHcloud database restore cannot combine a backup identity and point in time")
	}
	if request.NodeCount <= 0 {
		return DatabaseInstance{}, errors.New("OVHcloud database restore requires a positive node count")
	}
	requestBody := ovhDatabaseServiceCreation{
		Description: request.Description,
		Plan:        request.Plan,
		Version:     request.Version,
		ForkFrom:    ovhDatabaseForkFrom{ServiceID: request.InstanceID, BackupID: request.BackupID, PointInTime: cloneTimePointer(request.PointInTime)},
		NodesPattern: ovhDatabaseNodePattern{
			Flavor: request.Flavor, Number: request.NodeCount, Region: request.Region,
		},
	}
	for _, restriction := range request.IPRestrictions {
		requestBody.IPRestrictions = append(requestBody.IPRestrictions, ovhDatabaseIPRestriction{IP: strings.TrimSpace(restriction)})
	}
	if request.DiskGB > 0 {
		requestBody.Disk = &ovhDatabaseDisk{Size: request.DiskGB}
	}
	var response ovhDatabaseService
	if err := api.client.PostWithContext(ctx, api.enginePath(engine), requestBody, &response); err != nil {
		return DatabaseInstance{}, err
	}
	return api.instance(response, response.ID, engine)
}

func (api *ovhDatabaseSDK) listIdentities(ctx context.Context, path string) ([]string, error) {
	var raw json.RawMessage
	if err := api.client.GetWithContext(ctx, path, &raw); err != nil {
		return nil, err
	}
	return decodeOVHIdentityList(raw)
}

func (api *ovhDatabaseSDK) validate(ctx context.Context, value string) error {
	if api == nil || api.client == nil {
		return errors.New("OVHcloud Public Cloud Database SDK is not configured")
	}
	if ctx == nil {
		return errors.New("OVHcloud Public Cloud Database context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(api.project) == "" || strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("OVHcloud Public Cloud Database project and resource identity are required")
	}
	return nil
}

func (api *ovhDatabaseSDK) normalizeEngine(engine string) (string, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		engine = strings.ToLower(strings.TrimSpace(api.config.DatabaseEngine))
	}
	if engine == "valkey" {
		engine = "redis"
	}
	switch engine {
	case "mysql", "postgresql", "redis", "mongodb":
		return engine, nil
	default:
		return "", fmt.Errorf("OVHcloud Public Cloud Database engine %q is not supported by this translator", engine)
	}
}

func (api *ovhDatabaseSDK) enginePath(engine string) string {
	return "/cloud/project/" + url.PathEscape(api.project) + "/database/" + url.PathEscape(engine)
}

func (api *ovhDatabaseSDK) instancePath(engine, id string) string {
	return api.enginePath(engine) + "/" + url.PathEscape(id)
}

func (api *ovhDatabaseSDK) backupListPath(engine, id string) string {
	return api.instancePath(engine, id) + "/backup"
}

func (api *ovhDatabaseSDK) backupPath(engine, instanceID, backupID string) string {
	return api.backupListPath(engine, instanceID) + "/" + url.PathEscape(backupID)
}

func (api *ovhDatabaseSDK) instance(response ovhDatabaseService, fallbackID, engine string) (DatabaseInstance, error) {
	id := strings.TrimSpace(response.ID)
	if id == "" {
		id = strings.TrimSpace(fallbackID)
	}
	if id == "" {
		return DatabaseInstance{}, errors.New("OVHcloud database API returned no service identity")
	}
	nodeCount := response.NodeNumber
	if nodeCount == 0 {
		nodeCount = len(response.Nodes)
	}
	region := strings.TrimSpace(response.Region)
	flavor := strings.TrimSpace(response.Flavor)
	if len(response.Nodes) > 0 {
		if region == "" {
			region = strings.TrimSpace(response.Nodes[0].Region)
		}
		if flavor == "" {
			flavor = strings.TrimSpace(response.Nodes[0].Flavor)
		}
	}
	retention := response.Backups.RetentionDays
	if retention <= 0 {
		retention = ovhDatabasePlanRetentionDays(response.Plan)
	}
	pitrAvailableFrom := time.Time{}
	if response.Backups.PITR != nil {
		pitrAvailableFrom = response.Backups.PITR.UTC()
	}
	return DatabaseInstance{
		ID: id, Description: response.Description, Engine: firstNonEmptyOVH(response.Engine, engine), Version: response.Version,
		Region: region, Status: response.Status, Plan: response.Plan, Flavor: flavor,
		NodeCount: nodeCount, Encrypted: ovhEngineEncryptionKnown(response.Engine, engine),
		BackupRetentionDays: retention, PITRAvailableFrom: pitrAvailableFrom, DeletionProtection: response.DeletionProtection,
	}, nil
}

func (api *ovhDatabaseSDK) backup(response ovhDatabaseBackup, instanceID, engine string) (DatabaseBackup, error) {
	if strings.TrimSpace(response.ID) == "" {
		return DatabaseBackup{}, errors.New("OVHcloud database API returned no backup identity")
	}
	regions, err := decodeOVHRegionNames(response.Regions)
	if err != nil {
		return DatabaseBackup{}, err
	}
	if len(regions) == 0 && strings.TrimSpace(response.Region) != "" {
		regions = append(regions, response.Region)
	}
	retention := response.RetentionDays
	if retention <= 0 {
		retention = ovhDatabasePlanRetentionDays(response.Plan)
	}
	return DatabaseBackup{ID: response.ID, InstanceID: instanceID, CreatedAt: response.CreatedAt, Status: response.Status, Encrypted: ovhEngineEncryptionKnown(response.Engine, engine), Regions: regions, RetentionDays: retention}, nil
}

func decodeOVHIdentityList(raw json.RawMessage) ([]string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err == nil {
		return ids, nil
	}
	var objects []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil, fmt.Errorf("decode OVHcloud identity list: %w", err)
	}
	ids = make([]string, 0, len(objects))
	for _, object := range objects {
		id := strings.TrimSpace(object.ID)
		if id == "" {
			return nil, errors.New("OVHcloud identity list contained an entry without an id")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func decodeOVHRegionNames(raw json.RawMessage) ([]string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return names, nil
	}
	var objects []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil, fmt.Errorf("decode OVHcloud backup regions: %w", err)
	}
	names = make([]string, 0, len(objects))
	for _, object := range objects {
		name := strings.TrimSpace(object.Name)
		if name == "" {
			return nil, errors.New("OVHcloud backup region list contained an entry without a name")
		}
		names = append(names, name)
	}
	return names, nil
}

func ovhDatabasePlanRetentionDays(plan string) int {
	// Source: OVHcloud MySQL capabilities, retrieved 2026-08-13.
	// Essential/Discovery retain 2 days, Business/Production 14, Enterprise/Advanced 30.
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "essential", "discovery":
		return 2
	case "business", "production":
		return 14
	case "enterprise", "advanced":
		return 30
	default:
		return 0
	}
}

func ovhEngineEncryptionKnown(responseEngine, requestedEngine string) bool {
	engine := strings.ToLower(strings.TrimSpace(firstNonEmptyOVH(responseEngine, requestedEngine)))
	// OVHcloud's current Public Cloud Database backup documentation states that
	// MySQL, PostgreSQL, MongoDB, and Valkey backups are encrypted. Unknown
	// engines stay false so a future API addition cannot silently pass proof.
	switch engine {
	case "mysql", "postgresql", "mongodb", "redis", "valkey":
		return true
	default:
		return false
	}
}

type ovhDatabaseService struct {
	ID                 string                   `json:"id"`
	Description        string                   `json:"description"`
	Engine             string                   `json:"engine"`
	Version            string                   `json:"version"`
	Region             string                   `json:"region"`
	Status             string                   `json:"status"`
	Plan               string                   `json:"plan"`
	Flavor             string                   `json:"flavor"`
	NodeNumber         int                      `json:"nodeNumber"`
	Nodes              []ovhDatabaseNode        `json:"nodes"`
	Backups            ovhDatabaseServiceBackup `json:"backups"`
	DeletionProtection bool                     `json:"deletionProtection"`
	BackupTime         string                   `json:"backupTime"`
}

type ovhDatabaseNode struct {
	ID     string `json:"id"`
	Region string `json:"region"`
	Flavor string `json:"flavor"`
}

type ovhDatabaseServiceBackup struct {
	PITR          *time.Time `json:"pitr"`
	RetentionDays int        `json:"retentionDays"`
	Regions       []string   `json:"regions"`
	Time          string     `json:"time"`
}

type ovhDatabaseBackup struct {
	ID            string          `json:"id"`
	CreatedAt     time.Time       `json:"createdAt"`
	Description   string          `json:"description"`
	Region        string          `json:"region"`
	Regions       json.RawMessage `json:"regions"`
	Status        string          `json:"status"`
	Type          string          `json:"type"`
	Engine        string          `json:"engine"`
	Plan          string          `json:"plan"`
	RetentionDays int             `json:"retentionDays"`
}

type ovhDatabaseServiceCreation struct {
	Description    string                     `json:"description"`
	Plan           string                     `json:"plan"`
	Version        string                     `json:"version"`
	Disk           *ovhDatabaseDisk           `json:"disk,omitempty"`
	NodesPattern   ovhDatabaseNodePattern     `json:"nodesPattern"`
	ForkFrom       ovhDatabaseForkFrom        `json:"forkFrom"`
	IPRestrictions []ovhDatabaseIPRestriction `json:"ipRestrictions,omitempty"`
}

type ovhDatabaseIPRestriction struct {
	IP string `json:"ip"`
}

type ovhDatabaseNodePattern struct {
	Flavor string `json:"flavor"`
	Number int    `json:"number"`
	Region string `json:"region"`
}

type ovhDatabaseDisk struct {
	Size int `json:"size"`
}

type ovhDatabaseForkFrom struct {
	ServiceID   string     `json:"serviceId"`
	BackupID    string     `json:"backupId,omitempty"`
	PointInTime *time.Time `json:"pointInTime,omitempty"`
}
