#!/usr/bin/env bash
# Offline check for the disposable ECS database seed task definition.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FILTER="$ROOT/scripts/acceptance/aws-seed-task-definition.jq"

input='{"taskDefinitionArn":"arn:old","revision":3,"family":"old","containerDefinitions":[{"name":"deploy","image":"old-image","environment":[{"name":"KEEP","value":"yes"}],"secrets":[{"name":"KEEP_SECRET","valueFrom":"secret:keep"}],"user":"10001"}]}'
output=$(printf '%s\n' "$input" | jq \
	--arg family 'mlaws08-preview-seed-import' \
	--arg image 'public.ecr.aws/docker/library/mysql:8.4' \
	--arg name deploy \
	--arg command 'import database' \
	--arg dump_url 'https://example.invalid/dump' \
	--arg db_host 'database.example' \
	--arg db_name magento \
	--arg db_secret 'arn:aws:secretsmanager:eu-north-1:123:secret:database' \
	-f "$FILTER")

jq -e '
	(.taskDefinitionArn | not)
	and (.revision | not)
	and .family == "mlaws08-preview-seed-import"
	and (.containerDefinitions | length == 1)
	and .containerDefinitions[0].image == "public.ecr.aws/docker/library/mysql:8.4"
	and .containerDefinitions[0].user == "0"
	and .containerDefinitions[0].entryPoint == ["sh", "-ec"]
	and .containerDefinitions[0].command == ["import database"]
	and ([.containerDefinitions[0].environment[] | .name] | sort) == ["DB_HOST", "DB_NAME", "DUMP_URL", "KEEP"]
	and ([.containerDefinitions[0].secrets[] | .name] | sort) == ["DB_PASSWORD", "DB_USER", "KEEP_SECRET"]
	and ([.containerDefinitions[0].secrets[] | select(.name == "DB_USER") | .valueFrom] | .[0]) == "arn:aws:secretsmanager:eu-north-1:123:secret:database:username::"
	and ([.containerDefinitions[0].secrets[] | select(.name == "DB_PASSWORD") | .valueFrom] | .[0]) == "arn:aws:secretsmanager:eu-north-1:123:secret:database:password::"
' <<<"$output" >/dev/null

printf 'aws_seed_task_definition_test OK\n'
