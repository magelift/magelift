del(.taskDefinitionArn, .revision, .status, .requiresAttributes, .compatibilities, .registeredAt, .registeredBy, .tags)
| .family = $family
| (.containerDefinitions[0]
	| .name = $name
	| .image = $image
	| .user = "0"
	| .entryPoint = ["sh", "-ec"]
	| .command = [$command]
	| .environment = ((.environment // []) + [{name: "DUMP_URL", value: $dump_url}, {name: "DB_HOST", value: $db_host}, {name: "DB_NAME", value: $db_name}])
	| .secrets = ((.secrets // []) + [{name: "DB_USER", valueFrom: ($db_secret + ":username::")}, {name: "DB_PASSWORD", valueFrom: ($db_secret + ":password::")}])
) as $seed_container
| .containerDefinitions = [$seed_container]
