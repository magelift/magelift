package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/magelift/magelift/providers/azure/stack"
	"github.com/magelift/magelift/sdk"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation"
)

// storedPlan contains no credential values. Its identity is checked again
// before any lifecycle operation uses it.
type storedPlan struct {
	Envelope       sdk.Envelope `json:"envelope"`
	Spec           stack.Spec   `json:"spec"`
	BackendURL     string       `json:"backendUrl"`
	DestroyPreview bool         `json:"destroyPreview,omitempty"`
	AccountOnly    bool         `json:"accountOnly,omitempty"`
}

var stateEnvironmentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)

// Plan resolves a deliberately explicit Azure graph without contacting Azure.
func (s *Server) Plan(req *sdk.PlanRequest, resp *sdk.PlanResult) error {
	if req == nil || resp == nil {
		return errors.New("plan request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	var validation sdk.ValidateConfigResult
	if err := s.ValidateConfig(&sdk.ValidateConfigRequest{ProtocolVersion: req.ProtocolVersion, Runtime: req.Runtime, TargetBlock: req.TargetBlock}, &validation); err != nil {
		return err
	}
	if validation.Error != nil {
		resp.Error = validation.Error
		return nil
	}
	if !validation.Valid {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: strings.Join(validation.Problems, "; ")}
		return nil
	}
	var config target
	if err := yaml.Unmarshal(req.TargetBlock, &config); err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: "invalid Azure target"}
		return nil
	}
	plan, err := buildPlan(req, config)
	if err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
		return nil
	}
	opaque, err := json.Marshal(plan)
	if err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: "encode Azure plan"}
		return nil
	}
	resp.Plan = sdk.StoredPlan{StackName: req.Envelope.StackName, Provider: "azure", Runtime: "aks", Tier: sdk.ExtensionTierExperimental,
		ImageDigest: plan.Spec.ApplicationImage, StateBackendURL: plan.BackendURL, Opaque: opaque}
	if !plan.AccountOnly && !plan.Spec.DestroyOnly && !plan.Spec.ObserveOnly {
		inputs := deployInputs(plan.Spec)
		resp.Plan.DeployInputsJSON, err = json.Marshal(inputs)
		if err != nil {
			resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: "encode Azure deploy inputs"}
		}
	}
	return nil
}

func deployInputs(spec stack.Spec) sdk.DeployInputs {
	return sdk.DeployInputs{ImageDigest: spec.ApplicationImage, DatabaseName: "magento",
		ApplicationMode: spec.ApplicationMode, ApplicationVersion: spec.MagentoVersion,
		WebRuntime: spec.WebRuntime, CPURequest: "500m", MemoryRequest: "2Gi",
		CloudProject: spec.Project, Region: spec.Region, Magento: spec.Magento}
}

func buildPlan(req *sdk.PlanRequest, config target) (storedPlan, error) {
	base, err := buildAccountPlan(req, config)
	if err != nil {
		return storedPlan{}, err
	}
	if req.AccountOnly {
		return base, nil
	}
	if req.AllowExpiredPreview {
		base.Spec.DestroyOnly = true
		return base, nil
	}
	if req.RuntimeObserve {
		base.Spec.ObserveOnly = true
		return base, nil
	}
	if req.StateEnvironment != "" {
		expires, err := time.Parse(time.RFC3339, req.Envelope.ExpiresAt)
		if err != nil || !expires.After(time.Now()) {
			return storedPlan{}, errors.New("materialized Azure previews require a future RFC3339 expiration")
		}
	}
	if req.Edge.ExternalProvider != "" || (req.Edge.NativeProvider != "" && req.Edge.NativeProvider != "azure") || (req.Edge.Mode != "" && req.Edge.Mode != "none" && req.Edge.Mode != "native") || req.Edge.PurgeOnDeploy || req.Edge.PolicyReference != "" || req.Edge.WAFPolicyRef != "" || req.Edge.FailoverPolicyRef != "" || req.Edge.CachePolicyRef != "" || req.Edge.PurgePolicyRef != "" || (req.Edge.TLSMode != "" && req.Edge.TLSMode != "external") || (req.Edge.DNSMode != "" && req.Edge.DNSMode != "external") {
		return storedPlan{}, errors.New("Azure supports only an existing Kubernetes ingress class and TLS secret")
	}
	domain := strings.TrimSpace(req.Envelope.Domain)
	if len(req.Edge.Domains) > 1 {
		return storedPlan{}, errors.New("Azure ingress currently supports one domain")
	}
	if len(req.Edge.Domains) == 1 {
		if domain != "" && domain != req.Edge.Domains[0] {
			return storedPlan{}, errors.New("Azure ingress domain differs from environment domain")
		}
		domain = strings.TrimSpace(req.Edge.Domains[0])
	}
	if domain != "" && len(validation.IsDNS1123Subdomain(domain)) != 0 {
		return storedPlan{}, errors.New("Azure ingress domain must be a DNS hostname")
	}
	if domain != "" && strings.TrimSpace(config.IngressClass) == "" {
		return storedPlan{}, errors.New("Azure domain requires target.azure.ingressClass: install an ingress controller on AKS, then configure its class and DNS; for TLS also set edge.tls and target.azure.tlsSecret")
	}
	if req.Edge.TLS && (domain == "" || strings.TrimSpace(config.IngressClass) == "" || strings.TrimSpace(config.TLSSecret) == "") {
		return storedPlan{}, errors.New("Azure TLS requires a domain, ingressClass and an existing tlsSecret")
	}
	if config.TLSSecret != "" && !req.Edge.TLS {
		return storedPlan{}, errors.New("target.azure.tlsSecret requires edge.tls")
	}
	obs := req.Observability
	logsRequested := obs.Logs
	for _, signal := range obs.Signals {
		if signal != "logs" {
			return storedPlan{}, errors.New("Azure Container Insights currently supports only the logs signal")
		}
		logsRequested = true
	}
	if (obs.NativeProvider != "" && obs.NativeProvider != "azure" && obs.NativeProvider != "azure-monitor") || obs.ExternalProvider != "" || obs.Metrics || obs.Traces || len(obs.Alerts) != 0 || len(obs.Dashboards) != 0 || len(obs.SLOs) != 0 || obs.NativeReference != "" || obs.Endpoint != "" || obs.ServiceName != "" || obs.Environment != "" || obs.RetentionDays != 0 || obs.SamplingRatio != 0 || obs.RedactionPolicyRef != "" || len(obs.AlertRefs) != 0 || len(obs.CredentialRefs) != 0 || len(obs.Labels) != 0 || obs.DataResidency != "" {
		return storedPlan{}, errors.New("Azure currently supports Container Insights logs in an existing Log Analytics workspace; metrics, traces and telemetry policies require an adapter")
	}
	if (logsRequested || obs.NativeProvider != "") && strings.TrimSpace(config.MonitorWorkspaceID) == "" {
		return storedPlan{}, errors.New("Azure Container Insights logs require target.azure.monitorWorkspaceId")
	}
	if config.MonitorWorkspaceID != "" {
		parts := strings.Split(strings.Trim(config.MonitorWorkspaceID, "/"), "/")
		if len(parts) != 8 || !strings.EqualFold(parts[0], "subscriptions") || !strings.EqualFold(parts[2], "resourceGroups") || !strings.EqualFold(parts[4], "providers") || !strings.EqualFold(parts[5], "Microsoft.OperationalInsights") || !strings.EqualFold(parts[6], "workspaces") || parts[1] == "" || parts[3] == "" || parts[7] == "" {
			return storedPlan{}, errors.New("monitorWorkspaceId must be an Azure Log Analytics workspace resource ID")
		}
	}
	if req.Application.Email.Mode != "" && req.Application.Email.Mode != "disabled" {
		return storedPlan{}, errors.New("Azure Magento email integration is not yet implemented")
	}
	if req.Application.Magento.QueueTransport != "" || req.Application.Magento.QueueModule != "" {
		return storedPlan{}, errors.New("Azure custom Magento queue transports are not yet implemented")
	}
	for role := range req.Services {
		switch role {
		case "database", "cache", "search", "queue":
		default:
			return storedPlan{}, fmt.Errorf("Azure provider does not yet support services.%s", role)
		}
	}
	database, ok := req.Services["database"]
	if !ok || database.Family != "mysql" || (database.Ownership != "" && database.Ownership != "managed") || strings.TrimSpace(database.Version) == "" || database.Attached() || database.Image != "" || database.Endpoint != "" || database.Secret != "" || database.Username != "" || database.Port != 0 {
		return storedPlan{}, errors.New("Azure requires a managed MySQL database with an explicit version in services.database")
	}
	cache, ok := req.Services["cache"]
	if !ok || (cache.Family != "redis" && cache.Family != "valkey") {
		return storedPlan{}, errors.New("Azure requires a Redis or Valkey cache in services.cache")
	}
	if cache.Secret != "" || cache.Username != "" {
		return storedPlan{}, errors.New("Azure cache authentication is not yet supported")
	}
	if !cache.Attached() && cache.Ownership != "" && cache.Ownership != "self-hosted" {
		return storedPlan{}, errors.New("Azure managed cache provisioning is not yet supported")
	}
	cacheEndpoint := ""
	if cache.Attached() {
		if cache.Image != "" {
			return storedPlan{}, errors.New("Azure external cache cannot use an image")
		}
		cacheEndpoint = cache.Endpoint
		if cache.Port != 0 && cache.Port != 6379 {
			return storedPlan{}, errors.New("Azure cache binding currently requires port 6379")
		}
		if host, port, err := net.SplitHostPort(cacheEndpoint); err == nil {
			if port != "6379" {
				return storedPlan{}, errors.New("Azure cache binding currently requires port 6379")
			}
			cacheEndpoint = host
		}
		if strings.Contains(cacheEndpoint, "://") {
			return storedPlan{}, errors.New("Azure cache binding requires a hostname")
		}
	} else if (strings.TrimSpace(cache.Image) == "" && strings.TrimSpace(cache.Version) == "") || cache.Endpoint != "" || cache.Port != 0 {
		return storedPlan{}, errors.New("services.cache.image is required for self-hosted Azure cache")
	}
	search, ok := req.Services["search"]
	if !ok || search.Family != "opensearch" || search.Attached() || (search.Ownership != "" && search.Ownership != "self-hosted") || (strings.TrimSpace(search.Image) == "" && strings.TrimSpace(search.Version) == "") || search.Endpoint != "" || search.Secret != "" || search.Username != "" || search.Port != 0 {
		return storedPlan{}, errors.New("Azure requires a self-hosted OpenSearch image in services.search")
	}
	if queue, ok := req.Services["queue"]; ok && ((queue.Family != "database" && queue.Family != "db") || queue.Attached() || queue.Image != "" || queue.Endpoint != "" || queue.Secret != "" || queue.Username != "" || queue.Port != 0) {
		return storedPlan{}, errors.New("Azure currently supports the Magento database queue")
	}
	if strings.TrimSpace(config.ImageDigest) == "" {
		return storedPlan{}, errors.New("target.azure.imageDigest is required for a Magento application plan")
	}
	if err := (sdk.DeployInputs{ImageDigest: config.ImageDigest, DatabaseName: "magento"}).Validate(); err != nil {
		return storedPlan{}, err
	}
	spec := base.Spec
	spec.MySQLVersion = database.Version
	spec.ApplicationImage = config.ImageDigest
	spec.ApplicationMode = req.Application.Mode
	spec.MagentoVersion = req.Application.Version
	spec.Magento = sdk.MagentoOverlays{FrontName: req.Application.Magento.FrontName, CookieDomain: req.Application.Magento.CookieDomain,
		UnsecureBaseURL: req.Application.Magento.UnsecureBaseURL, SecureBaseURL: req.Application.Magento.SecureBaseURL,
		CORSOrigins: req.Application.Magento.CORSOrigins, StorefrontOrigin: req.Application.Magento.StorefrontOrigin,
		ConsumersMode: req.Application.Magento.ConsumersMode, ConsumerNames: req.Application.Magento.ConsumerNames,
		Variables: req.Application.Magento.Variables}
	spec.WebRuntime = req.Application.WebRuntime
	spec.CacheImage = cache.Image
	if spec.CacheImage == "" && !cache.Attached() {
		if cache.Family == "valkey" {
			spec.CacheImage = "valkey/valkey:" + cache.Version
		} else {
			spec.CacheImage = "redis:" + cache.Version
		}
	}
	spec.CacheEndpoint = cacheEndpoint
	spec.SearchImage = search.Image
	if spec.SearchImage == "" {
		spec.SearchImage = "opensearchproject/opensearch:" + sdk.OpenSearchContainerTag(search.Version)
	}
	spec.ImagePullSecret = config.ImagePullSecret
	spec.ACRResourceID = config.ACRResourceID
	spec.Domain, spec.IngressClass, spec.TLSSecret = domain, config.IngressClass, config.TLSSecret
	spec.MonitorWorkspaceID = config.MonitorWorkspaceID
	if config.PreviewPool != nil {
		if req.Envelope.EnvironmentClass != "preview" || req.Envelope.Protected || spec.HighAvailability {
			return storedPlan{}, errors.New("Azure preview pools may only be used for unprotected preview environments")
		}
		spec.PreviewPoolCluster = strings.TrimSpace(config.PreviewPool.ClusterName)
		spec.NodeSubnetID = config.PreviewPool.NodeSubnetID
		spec.MySQLSubnetID = config.PreviewPool.MySQLSubnetID
		spec.MySQLDNSZoneID = config.PreviewPool.MySQLDNSZoneID
		spec.Namespace = spec.ClusterName
		if spec.PreviewPoolCluster == "" {
			return storedPlan{}, errors.New("Azure preview pool clusterName is required")
		}
	}
	if err := spec.Validate(); err != nil {
		return storedPlan{}, err
	}
	base.Spec = spec
	return base, nil
}

func buildAccountPlan(req *sdk.PlanRequest, config target) (storedPlan, error) {
	tenant := strings.TrimSpace(config.TenantID)
	if tenant == "" {
		tenant = strings.TrimSpace(os.Getenv("AZURE_TENANT_ID"))
	}
	if tenant == "" || strings.TrimSpace(req.Envelope.Region) == "" || strings.TrimSpace(req.Envelope.Project) == "" || strings.TrimSpace(req.Envelope.Environment) == "" || strings.TrimSpace(req.Envelope.StackName) == "" {
		return storedPlan{}, errors.New("Azure plan requires a tenant ID, region, project, environment and stack name")
	}
	identity := strings.Join([]string{config.SubscriptionID, config.ResourceGroup, req.Envelope.Project, req.Envelope.Environment}, "/")
	digest := sha256.Sum256([]byte(identity))
	name := hex.EncodeToString(digest[:10])
	stateName := name
	if req.StateEnvironment != "" {
		if req.Envelope.EnvironmentClass != "preview" || !stateEnvironmentPattern.MatchString(req.StateEnvironment) {
			return storedPlan{}, errors.New("Azure stateEnvironment must be a stable name on a preview environment")
		}
		stateIdentity := strings.Join([]string{config.SubscriptionID, config.ResourceGroup, req.Envelope.Project, req.StateEnvironment}, "/")
		stateDigest := sha256.Sum256([]byte(stateIdentity))
		stateName = hex.EncodeToString(stateDigest[:10])
	}
	backendURL := strings.TrimSpace(config.StateBackendURL)
	if backendURL == "" {
		backendURL = "azblob://magelift-" + stateName
	}
	parsed, err := url.Parse(backendURL)
	if err != nil || parsed.Scheme != "azblob" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return storedPlan{}, errors.New("target.azure.stateBackendUrl must name an azblob:// state container")
	}
	spec := stack.Spec{SubscriptionID: config.SubscriptionID, TenantID: tenant, ResourceGroup: config.ResourceGroup,
		Region: req.Envelope.Region, Project: req.Envelope.Project, Environment: req.Envelope.Environment,
		ClusterName: "ml-" + name, MySQLServerName: "ml-" + name, MySQLUsername: "magelift",
		StorageAccount: "ml" + name, VaultName: "ml-" + name, HighAvailability: req.Envelope.Preset == "high-availability"}
	return storedPlan{Envelope: req.Envelope, Spec: spec, BackendURL: backendURL, DestroyPreview: req.AllowExpiredPreview, AccountOnly: req.AccountOnly}, nil
}
