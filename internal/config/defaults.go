package config

// DefaultResolveOptions returns the resolver defaults used by the CLI and
// library callers. The values describe safe, named provider shapes for the
// simple YAML path. Credentials, existing-resource references, and immutable
// application artifacts remain project inputs; advanced users can override
// every materialized provider field in YAML.
func DefaultResolveOptions() ResolveOptions {
	return ResolveOptions{Presets: map[string]map[string]any{
		"preview": {
			"target": map[string]any{"aws": map[string]any{"catalog": map[string]any{
				"aurora":  map[string]any{"engineSupportsAutoPause": true},
				"search":  map[string]any{"acceptColdStarts": true},
				"valkey":  map[string]any{"replicaCount": 0},
				"fargate": map[string]any{"desiredCount": 1},
			}}},
		},
		"standard": {
			"target": map[string]any{"aws": map[string]any{"catalog": map[string]any{
				"aurora":  map[string]any{"engineSupportsAutoPause": false},
				"search":  map[string]any{"acceptColdStarts": false},
				"valkey":  map[string]any{"replicaCount": 1},
				"fargate": map[string]any{"desiredCount": 2},
			}}},
		},
		"high-availability": {
			"target": map[string]any{"aws": map[string]any{"catalog": map[string]any{
				"aurora":  map[string]any{"engineSupportsAutoPause": false},
				"search":  map[string]any{"acceptColdStarts": false},
				"valkey":  map[string]any{"replicaCount": 2},
				"fargate": map[string]any{"desiredCount": 3},
			}}},
		},
	}}
}

// defaultProviderPreset contains only semantic values that can safely be
// resolved before a provider planner runs. It is intentionally selected from
// the target provider and runtime so resolving a GCP document never invents an
// AWS target block (or vice versa).
func defaultProviderPreset(provider, runtime, preset, applicationVersion string) (map[string]any, bool) {
	switch provider {
	case "aws":
		return defaultAWSPreset(runtime, preset, applicationVersion), true
	case "gcp":
		return defaultGCPPreset(runtime, preset, applicationVersion), true
	case "scaleway":
		return defaultScalewayPreset(preset), true
	case "ovh":
		return defaultOVHPreset(preset), true
	default:
		return nil, false
	}
}

func defaultAWSPreset(runtime, preset, applicationVersion string) map[string]any {
	databaseEngine := "aurora-mysql"
	searchMode := "provisioned"
	queueMode := "ecs-rabbitmq"
	fargateCPU := 1024
	fargateMemoryMiB := 2048
	desiredWeb := 2
	valkeyReplicas := 1
	valkeyNodeType := "cache.r7g.large"
	auroraInstanceClass := "db.r7g.large"
	auroraInstanceCount := 2
	searchInstanceType := "r7g.large.search"
	searchInstanceCount := 2
	searchDedicatedMasterType := ""
	searchDedicatedMasterCount := 0
	searchEBSVolumeSizeGiB := 100
	rabbitMQInstanceType := "mq.m7g.large"
	cacheSnapshotRetentionLimit := 7
	logRetentionDays := 30
	databaseBackupDays := 7
	artifactRetentionDays := 30
	if preset == "preview" {
		databaseEngine = "rds-mysql"
		searchMode = "disabled"
		queueMode = "db"
		fargateCPU = 512
		fargateMemoryMiB = 1024
		desiredWeb = 1
		valkeyReplicas = 0
		valkeyNodeType = "cache.t4g.micro"
		auroraInstanceClass = "db.t4g.micro"
		auroraInstanceCount = 0
		searchInstanceType = ""
		searchInstanceCount = 0
		searchEBSVolumeSizeGiB = 0
		rabbitMQInstanceType = ""
		cacheSnapshotRetentionLimit = 0
		logRetentionDays = 7
		databaseBackupDays = 1
		artifactRetentionDays = 7
	}
	line := magentoReleaseLine(applicationVersion)
	if line == "2.4.6" || line == "2.4.7" {
		// Adobe's latest-patch rows do not list MySQL; default to MariaDB instead of
		// silently selecting Aurora/RDS MySQL.
		databaseEngine = "rds-mariadb"
	}
	if preset == "high-availability" {
		desiredWeb = 3
		valkeyReplicas = 2
		auroraInstanceCount = 3
		searchInstanceCount = 3
		// AWS requires three dedicated masters for the Multi-AZ with Standby
		// profile. r6g.large.search is an AWS-supported dedicated-master type
		// with enough memory for the small default catalog.
		searchDedicatedMasterType = "r6g.large.search"
		searchDedicatedMasterCount = 3
		searchEBSVolumeSizeGiB = 400
	}
	catalog := map[string]any{
		"databaseEngine":              databaseEngine,
		"searchMode":                  searchMode,
		"queueMode":                   queueMode,
		"cacheSnapshotRetentionLimit": cacheSnapshotRetentionLimit,
		"aurora": map[string]any{
			"engineSupportsAutoPause": preset == "preview",
			"instanceClass":           auroraInstanceClass,
			"instanceCount":           auroraInstanceCount,
		},
		"search": map[string]any{
			"acceptColdStarts": preset == "preview",
			"instanceType":     searchInstanceType,
			"instanceCount":    searchInstanceCount,
			"ebsVolumeType":    "gp3",
			"ebsVolumeSizeGiB": searchEBSVolumeSizeGiB,
		},
		"valkey":   map[string]any{"nodeType": valkeyNodeType, "replicaCount": valkeyReplicas},
		"rabbitMq": map[string]any{"instanceType": rabbitMQInstanceType},
		"retention": map[string]any{
			"logDays": logRetentionDays, "backupDays": databaseBackupDays, "artifactDays": artifactRetentionDays,
		},
	}
	if preset == "preview" {
		catalog["aurora"] = map[string]any{
			"minimumAcu":              0,
			"maximumAcu":              4,
			"autoPauseSeconds":        900,
			"engineSupportsAutoPause": true,
			"instanceClass":           auroraInstanceClass,
			"instanceCount":           auroraInstanceCount,
		}
		catalog["search"] = map[string]any{
			"maximumIndexingOcu": 2,
			"maximumSearchOcu":   2,
			"acceptColdStarts":   true,
			"instanceType":       "m7g.medium.search",
			"instanceCount":      1,
			"ebsVolumeType":      "gp3",
			"ebsVolumeSizeGiB":   20,
		}
	}
	if preset == "high-availability" {
		catalog["search"].(map[string]any)["dedicatedMasterType"] = searchDedicatedMasterType
		catalog["search"].(map[string]any)["dedicatedMasterCount"] = searchDedicatedMasterCount
	}
	if runtime == "ecs-fargate" {
		catalog["fargate"] = map[string]any{
			"computeMode": "fargate", "cpu": fargateCPU, "memoryMiB": fargateMemoryMiB, "desiredCount": desiredWeb,
		}
	}
	if runtime == "eks" {
		searchReplicas := 1
		queueReplicas := 1
		queueConsumers := 1
		search := "opensearch"
		queue := "rabbitmq"
		if preset == "preview" {
			search = "disabled"
			queue = "database"
			searchReplicas = 0
			queueReplicas = 0
			queueConsumers = 0
		}
		if preset == "high-availability" {
			searchReplicas = 3
			queueReplicas = 2
			queueConsumers = 2
		}
		catalog["eks"] = map[string]any{
			"computeMode":        "auto-mode",
			"kubernetesVersion":  "1.36",
			"cpuRequest":         "500m",
			"memoryRequest":      "1Gi",
			"searchMode":         search,
			"searchReplicas":     searchReplicas,
			"queueMode":          queue,
			"queueReplicas":      queueReplicas,
			"queueConsumerCount": queueConsumers,
			"desiredWebReplicas": desiredWeb,
		}
	}
	natTopology := "multi-az"
	if preset == "preview" {
		natTopology = "single-az"
	}
	return map[string]any{"target": map[string]any{"aws": map[string]any{"natMode": "nat-gateway", "natTopology": natTopology, "catalog": catalog}}}
}

func defaultGCPPreset(runtime, preset, applicationVersion string) map[string]any {
	desiredWeb := 2
	availability := "REGIONAL"
	cloudSQLBackupEnabled := true
	cloudSQLBinaryLogEnabled := true
	memorystoreNodeType := "STANDARD_SMALL"
	memorystoreReplicas := 1
	searchReplicas := 1
	queueMode := "rabbitmq"
	queueReplicas := 1
	queueConsumers := 1
	if preset == "preview" {
		desiredWeb = 1
		availability = "ZONAL"
		cloudSQLBackupEnabled = false
		cloudSQLBinaryLogEnabled = false
		memorystoreNodeType = "SHARED_CORE_NANO"
		memorystoreReplicas = 0
		queueMode = "database"
		queueReplicas = 0
		queueConsumers = 0
	}
	if preset == "high-availability" {
		desiredWeb = 3
		memorystoreReplicas = 2
		searchReplicas = 3
		queueReplicas = 2
		queueConsumers = 2
	}
	gcp := map[string]any{
		"networkCidr":                     "10.20.0.0/16",
		"databaseName":                    "magento",
		"masterUsername":                  "magento",
		"cloudSqlTier":                    defaultGCPCloudSQLTier(applicationVersion, preset),
		"cloudSqlAvailability":            availability,
		"cloudSqlBackupEnabled":           cloudSQLBackupEnabled,
		"cloudSqlBinaryLogEnabled":        cloudSQLBinaryLogEnabled,
		"memorystoreEngineVersion":        defaultGCPMemorystoreEngineVersion(applicationVersion),
		"memorystoreNodeType":             memorystoreNodeType,
		"memorystoreShardCount":           1,
		"memorystoreReplicas":             memorystoreReplicas,
		"openSearchMode":                  "opensearch",
		"openSearchReplicas":              searchReplicas,
		"queueMode":                       queueMode,
		"queueReplicas":                   queueReplicas,
		"desiredWebReplicas":              desiredWeb,
		"queueConsumerCount":              queueConsumers,
		"releaseChannel":                  "REGULAR",
		"clusterIpv4Cidr":                 "/17",
		"servicesIpv4Cidr":                "/22",
		"autopilotCpuRequest":             "500m",
		"autopilotMemoryRequest":          "1Gi",
		"memorystoreMode":                 "CLUSTER_DISABLED",
		"memorystoreZoneDistributionMode": "MULTI_ZONE",
	}
	if preset != "preview" {
		gcp["enableCloudArmor"] = true
	}
	if cloudSQLBackupEnabled {
		// Cloud SQL documents seven retained backups and seven days of
		// transaction-log retention as the Enterprise defaults. Keep one
		// additional retained backup so the log-retention boundary is not
		// weaker than the documented recommendation.
		gcp["cloudSqlBackupRetentionCount"] = 8
		gcp["cloudSqlTransactionLogRetentionDays"] = 7
	}
	if runtime == "gke-standard" {
		gcp["standardNodeType"] = "e2-standard-4"
		gcp["standardNodeCount"] = 1
		gcp["standardNodeMinCount"] = 1
		gcp["standardNodeMaxCount"] = 1
		gcp["standardNodeDiskType"] = "pd-balanced"
		gcp["standardNodeDiskSizeGiB"] = 50
		gcp["standardNodeImageType"] = "COS_CONTAINERD"
	}
	return map[string]any{"target": map[string]any{"gcp": gcp}}
}

func defaultGCPMemorystoreEngineVersion(applicationVersion string) string {
	line := applicationVersion
	for index, character := range applicationVersion {
		if character == '-' {
			line = applicationVersion[:index]
			break
		}
	}
	if line == "2.4.9" {
		return "VALKEY_9_0"
	}
	return "VALKEY_8_0"
}

func defaultGCPCloudSQLTier(applicationVersion, preset string) string {
	line := applicationVersion
	for index, character := range applicationVersion {
		if character == '-' {
			line = applicationVersion[:index]
			break
		}
	}
	if line == "2.4.6" {
		if preset == "preview" {
			return "db-custom-1-3840"
		}
		return "db-custom-2-7680"
	}
	return "db-perf-optimized-N-2"
}

func defaultScalewayPreset(preset string) map[string]any {
	desiredWeb := 1
	queueConsumers := 0
	databaseHighAvailability := false
	redisClusterSize := 1
	databaseNodeType := "DB-DEV-S"
	redisNodeType := "RED1-MICRO"
	databaseBackupEnabled := false
	databaseBackupFrequency := 24
	databaseBackupRetention := 7
	databaseBackupSameRegion := true
	databaseEncryptionAtRest := true
	if preset != "preview" {
		desiredWeb = 2
		queueConsumers = 1
		databaseNodeType = "DB-GP-S"
		redisNodeType = "RED1-S"
		databaseBackupEnabled = true
	}
	if preset == "high-availability" {
		desiredWeb = 3
		queueConsumers = 2
		databaseHighAvailability = true
		redisClusterSize = 2
	}
	return map[string]any{"target": map[string]any{"scaleway": map[string]any{
		"networkCidr": "172.16.0.0/22", "databaseNodeType": databaseNodeType, "databaseHighAvailability": databaseHighAvailability, "redisNodeType": redisNodeType, "redisClusterSize": redisClusterSize,
		"databaseBackupEnabled": databaseBackupEnabled, "databaseBackupFrequencyHours": databaseBackupFrequency, "databaseBackupRetentionDays": databaseBackupRetention, "databaseBackupSameRegion": databaseBackupSameRegion, "databaseEncryptionAtRest": databaseEncryptionAtRest,
		"redisVersion": "8.6.3", "cacheMode": "redis", "kapsuleVersion": "1.36.1", "nodeType": "DEV1-M", "nodeCount": 2,
		"cpuRequest": "500m", "memoryRequest": "1Gi", "desiredWebReplicas": desiredWeb, "queueConsumerCount": queueConsumers,
	}}}
}

func defaultOVHPreset(preset string) map[string]any {
	desiredWeb := 1
	nodeCount := 1
	databaseNodeCount := 1
	valkeyNodeCount := 1
	databasePlan := "discovery"
	valkeyPlan := "discovery"
	queueConsumers := 0
	if preset != "preview" {
		desiredWeb = 2
		nodeCount = 2
		databaseNodeCount = 2
		valkeyNodeCount = 2
		databasePlan = "production"
		valkeyPlan = "production"
		queueConsumers = 1
	}
	if preset == "high-availability" {
		desiredWeb = 3
		queueConsumers = 2
	}
	return map[string]any{"target": map[string]any{"ovh": map[string]any{
		"networkCidr": "10.30.0.0/16", "databaseName": "magento", "masterUsername": "magento",
		"databaseFlavor": "b3-8", "databasePlan": databasePlan, "databaseVersion": "8.4", "databaseNodeCount": databaseNodeCount,
		"valkeyFlavor": "b3-8", "valkeyPlan": valkeyPlan, "valkeyVersion": "8.1", "valkeyNodeCount": valkeyNodeCount, "mksPlan": "standard",
		"nodeFlavor": "b3-8", "nodeCount": nodeCount, "cpuRequest": "500m", "memoryRequest": "1Gi",
		"desiredWebReplicas": desiredWeb, "queueConsumerCount": queueConsumers,
	}}}
}

func compatibilityDefaults(version string) map[string]any {
	line := version
	for index, character := range version {
		if character == '-' {
			line = version[:index]
			break
		}
	}
	versions := map[string]map[string]any{
		"2.4.9": {"auroraMysql": "8.0.mysql_aurora.3.12.0", "mysql": "8.4.10", "mariaDb": "11.8.3", "valkey": "9.0", "openSearch": "OpenSearch_3.1", "rabbitMq": "4.2"},
		"2.4.8": {"auroraMysql": "8.0.mysql_aurora.3.12.0", "mysql": "8.4.10", "mariaDb": "11.8.3", "valkey": "8.1", "openSearch": "OpenSearch_3.1", "rabbitMq": "4.2"},
		"2.4.7": {"auroraMysql": "8.0.mysql_aurora.3.12.0", "mysql": "8.4.10", "mariaDb": "10.11.13", "valkey": "8.1", "openSearch": "OpenSearch_2.19", "rabbitMq": "4.2"},
		"2.4.6": {"auroraMysql": "8.0.mysql_aurora.3.12.0", "mysql": "8.0.45", "mariaDb": "10.11.13", "valkey": "8.1", "openSearch": "OpenSearch_2.19", "rabbitMq": "4.2"},
	}[line]
	if versions == nil {
		return nil
	}
	return map[string]any{"target": map[string]any{"aws": map[string]any{"catalog": map[string]any{"versions": versions}}}}
}

// runtimeDefaults supplies the smallest release-aware build defaults. A
// project can still override these values explicitly, but a resolved build
// must never leave its Composer requirement implicit.
func runtimeDefaults(version string) map[string]any {
	line := version
	for index, character := range version {
		if character == '-' {
			line = version[:index]
			break
		}
	}
	composer := map[string]any{"version": "2.10"}
	if line == "2.4.6" {
		composer["version"] = "2.2.26+"
	}
	return map[string]any{"build": map[string]any{"composer": composer}}
}

func magentoReleaseLine(version string) string {
	for index, character := range version {
		if character == '-' {
			return version[:index]
		}
	}
	return version
}
