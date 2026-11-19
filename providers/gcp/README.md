# GCP self-hosted service operations

A self-hosted MariaDB service runs as one StatefulSet with a persistent volume.
It does not provide automatic failover or backups. Stable-environment claims
are retained when the StatefulSet is deleted; preview claims are deleted with
their StatefulSet. Configure and test off-cluster backups before storing data.

For a logical restore, stop the shop's web, cron and consumer workloads, then
import the SQL backup into the MariaDB StatefulSet in the target namespace.
Replace the namespace and StatefulSet name with the values shown by
`kubectl get statefulsets -n NAMESPACE`:

```sh
kubectl exec -i -n NAMESPACE statefulset/DB_STATEFULSET -- sh -c \
  'export MYSQL_PWD="$MARIADB_PASSWORD"; exec mariadb --user="$MARIADB_USER" "$MARIADB_DATABASE"' < backup.sql
```

Restore media separately. Set `target.gcp.encryptionKeySecret` to the Secret
Manager secret containing the original Magento crypt key before deploying a
restored database, including in a preview that uses copied data. Without an
explicit key, MageLift creates a different key for each environment. Resume
workloads only after checking the database and media.

Self-hosted cache and session stores each use a single Valkey or Redis StatefulSet.
Their default data volume is 20 GiB, and their default maxmemory is 512 MiB
with 512 MiB of pod memory headroom. Set `storageGiB`, `memoryMiB`, and
`evictionPolicy` on each service to change those values. Store data and
credentials are isolated per preview unless an existing service or crypt key
is explicitly attached.
