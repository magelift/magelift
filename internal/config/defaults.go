package config

// DefaultResolveOptions returns the resolver defaults used by the CLI and
// library callers. The values describe topology decisions only. Credentials,
// existing-resource references, and benchmark-selected instance sizes remain
// project inputs.
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

func compatibilityDefaults(version string) map[string]any {
	line := version
	for index, character := range version {
		if character == '-' {
			line = version[:index]
			break
		}
	}
	versions := map[string]map[string]any{
		"2.4.9": {"auroraMysql": "8.0.mysql_aurora.3.12", "valkey": "9.0", "openSearch": "OpenSearch_3.1", "rabbitMq": "4.2"},
		"2.4.8": {"auroraMysql": "8.0.mysql_aurora.3.12", "valkey": "8.1", "openSearch": "OpenSearch_3.1", "rabbitMq": "4.2"},
		"2.4.7": {"auroraMysql": "8.0.mysql_aurora.3.12", "valkey": "8.1", "openSearch": "OpenSearch_2.19", "rabbitMq": "4.2"},
		"2.4.6": {"auroraMysql": "8.0.mysql_aurora.3.12", "valkey": "8.1", "openSearch": "OpenSearch_2.19", "rabbitMq": "4.2"},
	}[line]
	if versions == nil {
		return nil
	}
	return map[string]any{"target": map[string]any{"aws": map[string]any{"catalog": map[string]any{"versions": versions}}}}
}
