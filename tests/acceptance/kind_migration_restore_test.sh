#!/usr/bin/env bash
# Opt in: MAGELIFT_RUN_KIND_RESTORE=1 bash tests/acceptance/kind_migration_restore_test.sh
# Requires Docker, kind, kubectl, Go and gzip. Pulls public container images.
set -euo pipefail

if [[ ${MAGELIFT_RUN_KIND_RESTORE:-} != 1 ]]; then
  echo 'SKIP: set MAGELIFT_RUN_KIND_RESTORE=1 to run the disposable Kind restore smoke'
  exit 0
fi
for tool in docker kind kubectl go gzip; do
  command -v "$tool" >/dev/null || { echo "missing required tool: $tool" >&2; exit 1; }
done
repo=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/magelift-kind-restore.XXXXXXXX")
export KUBECONFIG="$tmp/kubeconfig"
cluster="ml-restore-$(basename "$tmp" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g')"
created=0
cleanup() {
  status=$?
  trap - EXIT
  if [[ $created == 1 ]]; then
    kind delete cluster --name "$cluster" --kubeconfig "$KUBECONFIG" || status=1
  fi
  rm -rf "$tmp"
  exit "$status"
}
trap cleanup EXIT
if kind get clusters | grep -Fxq "$cluster"; then
  echo "cluster name already exists: $cluster" >&2
  exit 1
fi
# Delete this unique name even if creation is interrupted partway through.
created=1
kind create cluster --name "$cluster" --kubeconfig "$KUBECONFIG" --wait 180s
context="kind-$cluster"
(cd "$repo" && go build -o "$tmp/magelift" ./cmd/magelift)
k() { kubectl --kubeconfig "$KUBECONFIG" --context "$context" "$@"; }
k create namespace restore-smoke
k -n restore-smoke create secret generic db-auth --from-literal=password=synthetic-password
k apply -f - <<'YAML'
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: media, namespace: restore-smoke}
spec:
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 1Gi}}
---
apiVersion: v1
kind: Service
metadata: {name: mariadb, namespace: restore-smoke}
spec:
  selector: {app: mariadb}
  ports: [{port: 3306}]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: mariadb, namespace: restore-smoke}
spec:
  replicas: 1
  selector: {matchLabels: {app: mariadb}}
  template:
    metadata: {labels: {app: mariadb}}
    spec:
      containers:
        - name: mariadb
          image: mariadb:11.4
          env:
            - {name: MARIADB_ROOT_PASSWORD, value: synthetic-password}
            - {name: MARIADB_DATABASE, value: magento}
          ports: [{containerPort: 3306}]
          readinessProbe:
            exec: {command: [healthcheck.sh, --connect, --innodb_initialized]}
            initialDelaySeconds: 5
            periodSeconds: 5
---
apiVersion: v1
kind: Pod
metadata: {name: media-check, namespace: restore-smoke}
spec:
  securityContext: {runAsUser: 10001, runAsGroup: 10001}
  containers:
    - name: check
      image: busybox:1.37.0
      command: [sleep, '3600']
      volumeMounts: [{name: media, mountPath: /media}]
  volumes:
    - name: media
      persistentVolumeClaim: {claimName: media}
YAML
k -n restore-smoke rollout status deployment/mariadb --timeout=240s
k -n restore-smoke wait --for=condition=Ready pod/media-check --timeout=240s
uid=$(k get namespace restore-smoke -o jsonpath='{.metadata.uid}')
printf '%s\n' 'CREATE TABLE synthetic (id INT PRIMARY KEY, value VARCHAR(30)); INSERT INTO synthetic VALUES (1, '\''restored'\'');' | gzip > "$tmp/data.sql.gz"
mkdir -p "$tmp/media/catalog/nested"
printf '%s\n' 'synthetic-media' > "$tmp/media/catalog/nested/example.txt"
args=(migrate restore --context "$context" --namespace restore-smoke --namespace-uid "$uid" --database-host mariadb --database magento --database-secret db-auth --database-user root --sql "$tmp/data.sql.gz" --media "$tmp/media" --media-claim media --tls disabled --execute --yes)
"$tmp/magelift" "${args[@]}"
row=$(k -n restore-smoke exec deployment/mariadb -- env MYSQL_PWD=synthetic-password mariadb -uroot -N -B magento -e 'SELECT value FROM synthetic WHERE id=1')
[[ $row == restored ]] || { echo "unexpected database value: $row" >&2; exit 1; }
k -n restore-smoke exec media-check -- sh -c 'test "$(cat /media/catalog/nested/example.txt)" = synthetic-media && echo root-write > /media/write-check && test "$(cat /media/write-check)" = root-write && echo written-by-magento > /media/catalog/nested/write-check && test "$(cat /media/catalog/nested/write-check)" = written-by-magento'
if "$tmp/magelift" "${args[@]}" > "$tmp/retry.log" 2>&1; then
  echo 'restore unexpectedly accepted a nonempty database' >&2
  exit 1
fi
if ! grep -q 'destination database is non-empty' "$tmp/retry.log"; then
  cat "$tmp/retry.log" >&2
  exit 1
fi
helpers=$(k -n restore-smoke get pods -l app.kubernetes.io/name=magelift-restore -o name)
[[ -z $helpers ]] || { echo "restore helper pods remain: $helpers" >&2; exit 1; }
echo 'PASS: SQL, nonempty refusal, media read/write as UID/GID 10001, and helper cleanup'
