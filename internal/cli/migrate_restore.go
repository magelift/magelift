package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultRestoreTimeout = time.Hour

const restoreMySQLImage = "mysql:8.4@sha256:c592c15aaf4a1961e15d82eb31ea5987dda862d1c4b1e93424438c0e91dc1f8d"
const restoreMediaImage = "busybox:1.37.0@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec"

var restoreName = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)
var restoreDatabase = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type restoreOptions struct {
	Context, Namespace, NamespaceUID, DatabaseHost, Database, DatabaseSecret string
	DatabaseUser                                                             string
	SQL, Media, MediaClaim, TLS, CASecret                                    string
	Execute, AllowNonEmpty                                                   bool
	Timeout                                                                  time.Duration
}

type restoreExec func(context.Context, io.Reader, ...string) (string, error)

func migrateCommand(o *options) *cobra.Command {
	group := &cobra.Command{Use: "migrate", Short: "Move shop data to a Kubernetes environment"}
	group.AddCommand(migrateRestoreCommand(o))
	return group
}

func migrateRestoreCommand(o *options) *cobra.Command {
	var opts restoreOptions
	cmd := &cobra.Command{Use: "restore", Short: "Plan or restore a SQL dump and media into Kubernetes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := opts.validate(); err != nil {
			return invalid(err)
		}
		if !opts.Execute {
			return o.write(map[string]any{"action": "plan", "context": opts.Context, "namespace": opts.Namespace, "namespaceUID": opts.NamespaceUID, "databaseHost": opts.DatabaseHost, "database": opts.Database, "databaseSecret": opts.DatabaseSecret, "databaseUser": opts.DatabaseUser, "sql": opts.SQL, "media": opts.Media, "mediaClaim": opts.MediaClaim, "tls": opts.TLS, "caSecret": opts.CASecret, "allowNonempty": opts.AllowNonEmpty, "timeout": opts.Timeout.String()})
		}
		if !o.yes {
			return invalid(errors.New("restoring data requires --execute and --yes"))
		}
		result, err := runKubeRestore(cmd.Context(), opts, restoreKubectl)
		if err != nil {
			return err
		}
		return o.write(result)
	}}
	f := cmd.Flags()
	f.StringVar(&opts.Context, "context", "", "destination kubeconfig context")
	f.StringVar(&opts.Namespace, "namespace", "", "destination namespace")
	f.StringVar(&opts.NamespaceUID, "namespace-uid", "", "expected namespace UID")
	f.StringVar(&opts.DatabaseHost, "database-host", "", "database hostname reachable from the namespace")
	f.StringVar(&opts.Database, "database", "magento", "destination database name")
	f.StringVar(&opts.DatabaseSecret, "database-secret", "", "namespace-local Secret with password and optionally username keys")
	f.StringVar(&opts.DatabaseUser, "database-user", "", "database username when the Secret contains only a password")
	f.StringVar(&opts.SQL, "sql", "", "local SQL or SQL.gz dump")
	f.StringVar(&opts.Media, "media", "", "local media directory to merge into a PVC")
	f.StringVar(&opts.MediaClaim, "media-claim", "", "existing destination media PVC")
	f.StringVar(&opts.TLS, "tls", "verify", "database TLS: verify or disabled")
	f.StringVar(&opts.CASecret, "ca-secret", "", "namespace-local Secret containing ca.crt for database TLS")
	f.DurationVar(&opts.Timeout, "timeout", defaultRestoreTimeout, "maximum lifetime of each restore helper, for example 4h")
	f.BoolVar(&opts.Execute, "execute", false, "execute the reviewed plan")
	f.BoolVar(&opts.AllowNonEmpty, "allow-nonempty", false, "allow importing into a non-empty database")
	return cmd
}

func (o restoreOptions) validate() error {
	if o.Timeout < 0 || (o.Timeout > 0 && o.Timeout < time.Second) {
		return errors.New("restore timeout must be at least one second")
	}
	if strings.TrimSpace(o.Context) == "" || !restoreName.MatchString(o.Namespace) || strings.TrimSpace(o.NamespaceUID) == "" {
		return errors.New("context, namespace and expected namespace UID are required")
	}
	if o.SQL != "" && (!restoreName.MatchString(o.DatabaseHost) || !restoreDatabase.MatchString(o.Database) || !restoreName.MatchString(o.DatabaseSecret)) {
		return errors.New("database host, name and Secret name are required and must be valid names")
	}
	if o.TLS != "verify" && o.TLS != "disabled" {
		return errors.New("database TLS must be verify or disabled")
	}
	if o.CASecret != "" && (!restoreName.MatchString(o.CASecret) || o.TLS != "verify" || o.SQL == "") {
		return errors.New("ca-secret requires SQL, verified TLS, and a valid Secret name")
	}
	if o.SQL == "" && o.Media == "" {
		return errors.New("provide --sql or --media")
	}
	if o.SQL != "" {
		info, err := os.Stat(o.SQL)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("SQL dump must be an existing regular file")
		}
	}
	if o.Media != "" {
		info, err := os.Lstat(o.Media)
		if err != nil || !info.IsDir() {
			return errors.New("media source must be an existing directory")
		}
		if !restoreName.MatchString(o.MediaClaim) {
			return errors.New("media restore requires an existing --media-claim")
		}
		if err := validateRestoreMedia(o.Media); err != nil {
			return err
		}
	}
	return nil
}

func restoreKubectl(ctx context.Context, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &restoreLimitedWriter{buffer: &stdout, limit: 1 << 20}
	cmd.Stderr = &restoreLimitedWriter{buffer: &stderr, limit: 1 << 20}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

type restoreLimitedWriter struct {
	buffer *bytes.Buffer
	limit  int
}

func (w *restoreLimitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = w.buffer.Write(p)
	}
	return n, nil
}

func validateRestoreMedia(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("media contains unsupported file %q", path)
		}
		return nil
	})
}

func runKubeRestore(ctx context.Context, o restoreOptions, run restoreExec) (result map[string]any, retErr error) {
	base := []string{"--context", o.Context, "--namespace", o.Namespace}
	call := func(ctx context.Context, in io.Reader, args ...string) (string, error) {
		return run(ctx, in, append(append([]string{}, base...), args...)...)
	}
	verifyTarget := func() error {
		uid, err := call(ctx, nil, "get", "namespace", o.Namespace, "-o", "jsonpath={.metadata.uid}")
		if err != nil {
			return fmt.Errorf("verify destination namespace: %w", err)
		}
		if strings.TrimSpace(uid) != o.NamespaceUID {
			return errors.New("destination namespace UID does not match the reviewed target")
		}
		return nil
	}
	if err := verifyTarget(); err != nil {
		return nil, err
	}
	// A namespace can have only one SQL writer and one media writer started by MageLift.
	// The UID keeps a recreated namespace distinct during cleanup.
	digest := sha256.Sum256([]byte(o.NamespaceUID))
	prefix := "magelift-restore-" + hex.EncodeToString(digest[:8])
	createPod := func(name, image string, media bool) error {
		pod := restorePod(name, image, o, media)
		data, err := json.Marshal(pod)
		if err != nil {
			return err
		}
		_, err = call(ctx, bytes.NewReader(data), "create", "-f", "-")
		return err
	}
	waitReady := func(name string) error {
		_, err := call(ctx, nil, "wait", "--for=condition=Ready", "pod/"+name, "--timeout=180s")
		return err
	}
	cleanup := func(name string) error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, err := call(cleanupCtx, nil, "delete", "pod", name, "--ignore-not-found=true", "--wait=true", "--timeout=18s")
		if err != nil {
			return fmt.Errorf("remove helper pod %s: %w", name, err)
		}
		return nil
	}
	completed := []string{}
	defer func() {
		if retErr != nil && len(completed) > 0 {
			retErr = fmt.Errorf("restore completed %s before failing; inspect the destination before retrying: %w", strings.Join(completed, ", "), retErr)
		}
	}()
	if o.SQL != "" {
		name := prefix + "-sql"
		if err := createPod(name, restoreMySQLImage, false); err != nil {
			return nil, fmt.Errorf("create SQL helper %s (check for an earlier restore): %w", name, err)
		}
		defer func() { retErr = errors.Join(retErr, cleanup(name)) }()
		if err := waitReady(name); err != nil {
			return nil, fmt.Errorf("start SQL helper: %w", err)
		}
		// The shell is fixed; credentials are read only from the pod's Secret environment.
		tls := "--ssl-mode=VERIFY_IDENTITY --ssl-ca=/etc/pki/tls/certs/ca-bundle.crt"
		if o.TLS == "disabled" {
			tls = "--ssl-mode=DISABLED"
		} else if o.CASecret != "" {
			tls = "--ssl-mode=VERIFY_IDENTITY --ssl-ca=/tls/ca.crt"
		}
		script := `export MYSQL_PWD="$DB_PASSWORD"; exec mysql --max-allowed-packet=1073741824 --host="$DB_HOST" --user="$DB_USER" --database="$DB_NAME" ` + tls + ` "$@"`
		execArgs := []string{"exec", "-i", "pod/" + name, "--", "sh", "-c", script, "mysql"}
		tables, err := call(ctx, nil, append(execArgs, "--batch", "--skip-column-names", "--execute", "SHOW FULL TABLES")...)
		if err != nil {
			return nil, fmt.Errorf("check destination database: %w", err)
		}
		if strings.TrimSpace(tables) != "" && !o.AllowNonEmpty {
			return nil, errors.New("destination database is non-empty; use --allow-nonempty only after reviewing the dump and backup")
		}
		file, err := os.Open(o.SQL)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		var reader io.Reader = file
		if strings.HasSuffix(strings.ToLower(o.SQL), ".gz") {
			gz, gzErr := gzip.NewReader(file)
			if gzErr != nil {
				return nil, gzErr
			}
			defer gz.Close()
			reader = gz
		}
		if err := verifyTarget(); err != nil {
			return nil, err
		}
		if _, err := call(ctx, reader, execArgs...); err != nil {
			return nil, fmt.Errorf("import SQL failed; the destination database may be partially modified: %w", err)
		}
		completed = append(completed, "sql")
	}
	if o.Media != "" {
		name := prefix + "-media"
		if err := createPod(name, restoreMediaImage, true); err != nil {
			return nil, fmt.Errorf("create media helper %s (check for an earlier restore): %w", name, err)
		}
		defer func() { retErr = errors.Join(retErr, cleanup(name)) }()
		if err := waitReady(name); err != nil {
			return nil, fmt.Errorf("start media helper: %w", err)
		}
		if err := verifyTarget(); err != nil {
			return nil, err
		}
		reader, writer := io.Pipe()
		done := make(chan error, 1)
		go func() { err := writeRestoreTar(writer, o.Media); _ = writer.CloseWithError(err); done <- err }()
		_, err := call(ctx, reader, "exec", "-i", "pod/"+name, "--", "sh", "-c", "test -z \"$(find /media -type l -print -quit)\" && exec tar -xf - -C /media")
		_ = reader.Close()
		archiveErr := <-done
		if err != nil {
			return nil, fmt.Errorf("import media failed; the destination media may be partially modified: %w", err)
		}
		if archiveErr != nil {
			return nil, fmt.Errorf("import media failed; the destination media may be partially modified: %w", archiveErr)
		}
		completed = append(completed, "media")
	}
	return map[string]any{"context": o.Context, "namespace": o.Namespace, "restored": completed}, nil
}

func restorePod(name, image string, o restoreOptions, media bool) map[string]any {
	timeout := o.Timeout
	if timeout == 0 {
		timeout = defaultRestoreTimeout
	}
	seconds := int64(timeout / time.Second)
	container := map[string]any{"name": "helper", "image": image, "command": []string{"sleep", fmt.Sprint(seconds)}, "resources": map[string]any{"requests": map[string]string{"cpu": "100m", "memory": "128Mi"}}}
	spec := map[string]any{"restartPolicy": "Never", "activeDeadlineSeconds": seconds, "terminationGracePeriodSeconds": 1, "automountServiceAccountToken": false, "containers": []any{container}}
	if media {
		container["volumeMounts"] = []any{map[string]string{"name": "media", "mountPath": "/media"}}
		spec["volumes"] = []any{map[string]any{"name": "media", "persistentVolumeClaim": map[string]string{"claimName": o.MediaClaim}}}
	}
	if !media {
		if o.CASecret != "" {
			container["volumeMounts"] = []any{map[string]any{"name": "ca", "mountPath": "/tls", "readOnly": true}}
			spec["volumes"] = []any{map[string]any{"name": "ca", "secret": map[string]string{"secretName": o.CASecret}}}
		}
		ref := func(key string) map[string]any {
			return map[string]any{"secretKeyRef": map[string]string{"name": o.DatabaseSecret, "key": key}}
		}
		user := map[string]any{"name": "DB_USER", "valueFrom": ref("username")}
		if o.DatabaseUser != "" {
			user = map[string]any{"name": "DB_USER", "value": o.DatabaseUser}
		}
		container["env"] = []any{map[string]any{"name": "DB_HOST", "value": o.DatabaseHost}, map[string]any{"name": "DB_NAME", "value": o.Database}, user, map[string]any{"name": "DB_PASSWORD", "valueFrom": ref("password")}}
	}
	return map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "labels": map[string]string{"app.kubernetes.io/name": "magelift-restore"}}, "spec": spec}
}

func writeRestoreTar(output io.Writer, root string) error {
	writer := tar.NewWriter(output)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("media contains unsupported file %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		// All bundled Magento runtimes write media as this account.
		header.Uid, header.Gid = 10001, 10001
		header.Uname, header.Gname = "", ""
		header.Mode = 0644
		if info.IsDir() {
			header.Mode = 0755
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
	if err != nil {
		return err
	}
	return writer.Close()
}
