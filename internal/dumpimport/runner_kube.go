package dumpimport

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// RunnerKube selects the VPC-adjacent kubectl-exec transport for private-IP
// Cloud SQL (D-03 / MIGRATE-04 managed dump). Host/compose remain the default.
const RunnerKube = "kube"

// KubeExec runs a single external command for the kube runner. Tests inject a
// fake; production uses kubectl (or an equivalent) via DefaultKubeExec.
//
// name is the binary; env is additional process environment (not logged).
// Password must travel via env/stdin — never append -pPASSWORD to argv (T-07-08).
type KubeExec func(ctx context.Context, stdin io.Reader, name string, args []string, env []string) (stdout, stderr string, err error)

// DefaultKubeExec runs name with args, optional stdin, and extra env vars.
func DefaultKubeExec(ctx context.Context, stdin io.Reader, name string, args []string, env []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// kubeMySQL pipes SQL into mysql running inside a GKE-adjacent pod via kubectl
// exec, so the client shares the VPC with private-IP Cloud SQL (Ipv4Enabled false).
//
// Fallback when the Magento/web image lacks a mysql client: schedule a short-lived
// Job (or sidecar) from a mysql-client image in the same namespace/VPC, wait for the
// pod, then exec into that Job pod with the same argv shape below. Unit tests inject
// KubeExec to capture argv without a live cluster — do not require kubectl here.
type kubeMySQL struct {
	namespace string
	pod       string // pod name or "deploy/<name>" resource
	selector  string // label selector when pod empty
	container string
	user      string
	password  string
	host      string
	port      int
	kubeconfig string
	exec      KubeExec
}

func (k *kubeMySQL) ExecSQL(ctx context.Context, database string, stdin io.Reader) (string, error) {
	args, err := k.kubectlArgs(ctx, database, nil)
	if err != nil {
		return "", err
	}
	_, stderr, err := k.run(ctx, k.withPasswordStdin(stdin), args)
	return stderr, err
}

func (k *kubeMySQL) Query(ctx context.Context, database, sql string) (string, error) {
	extra := []string{"-N", "-e", sql}
	args, err := k.kubectlArgs(ctx, database, extra)
	if err != nil {
		return "", err
	}
	stdout, stderr, err := k.run(ctx, k.withPasswordStdin(nil), args)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("dumpimport: mysql query failed: %s", msg)
	}
	return strings.TrimSpace(stdout), nil
}

func (k *kubeMySQL) run(ctx context.Context, stdin io.Reader, args []string) (stdout, stderr string, err error) {
	fn := k.exec
	if fn == nil {
		fn = DefaultKubeExec
	}
	return fn(ctx, stdin, "kubectl", args, nil)
}

// withPasswordStdin prefixes a base64 password line so the in-pod shell can
// export MYSQL_PWD without putting the secret on kubectl argv (CR-01 / T-07-08).
func (k *kubeMySQL) withPasswordStdin(sql io.Reader) io.Reader {
	line := base64.StdEncoding.EncodeToString([]byte(k.password)) + "\n"
	if sql == nil {
		return strings.NewReader(line)
	}
	return io.MultiReader(strings.NewReader(line), sql)
}

func (k *kubeMySQL) kubectlArgs(ctx context.Context, database string, mysqlExtra []string) ([]string, error) {
	target, err := k.resolveTarget(ctx)
	if err != nil {
		return nil, err
	}
	args := []string{"exec", "-i", "-n", k.namespace, target}
	if k.kubeconfig != "" {
		// Prepend global flags before the subcommand verb for kubectl.
		args = append([]string{"--kubeconfig", k.kubeconfig}, args...)
	}
	if k.container != "" {
		args = append(args, "-c", k.container)
	}
	args = append(args, "--")
	// Decode password from first stdin line inside the pod — never MYSQL_PWD= on argv.
	const script = `read -r _ml_b64
MYSQL_PWD=$(printf '%s' "$_ml_b64" | base64 -d)
export MYSQL_PWD
unset _ml_b64
host=$1; port=$2; user=$3; db=$4
shift 4
if [ -n "$db" ]; then
  exec mysql -h "$host" -P "$port" -u "$user" "$db" "$@"
fi
exec mysql -h "$host" -P "$port" -u "$user" "$@"`
	args = append(args, "sh", "-c", script, "mysql", k.host, strconv.Itoa(k.port), k.user, database)
	args = append(args, mysqlExtra...)
	return args, nil
}

func (k *kubeMySQL) resolveTarget(ctx context.Context) (string, error) {
	if pod := strings.TrimSpace(k.pod); pod != "" {
		return pod, nil
	}
	sel := strings.TrimSpace(k.selector)
	if sel == "" {
		return "", errors.New("dumpimport: kube runner requires Pod or PodSelector")
	}
	args := []string{"get", "pods", "-n", k.namespace, "-l", sel, "-o", "jsonpath={.items[0].metadata.name}"}
	if k.kubeconfig != "" {
		args = append([]string{"--kubeconfig", k.kubeconfig}, args...)
	}
	stdout, stderr, err := k.run(ctx, nil, args)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("dumpimport: resolve kube pod for selector %q: %s", sel, msg)
	}
	name := strings.TrimSpace(stdout)
	if name == "" {
		return "", fmt.Errorf("dumpimport: no pods match selector %q in namespace %q", sel, k.namespace)
	}
	return name, nil
}

func newKubeRunner(opts Options) (*kubeMySQL, error) {
	ns := strings.TrimSpace(opts.Namespace)
	if ns == "" {
		ns = "default"
	}
	pod := strings.TrimSpace(opts.Pod)
	sel := strings.TrimSpace(opts.PodSelector)
	if pod == "" && sel == "" {
		return nil, errors.New("dumpimport: kube runner requires Pod or PodSelector")
	}
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		return nil, errors.New("dumpimport: kube runner requires Host (Cloud SQL private IP)")
	}
	execFn := opts.KubeExec
	if execFn == nil {
		if _, err := exec.LookPath("kubectl"); err != nil {
			return nil, errors.New("dumpimport: kubectl not found on PATH (required for kube runner)")
		}
		execFn = DefaultKubeExec
	}
	return &kubeMySQL{
		namespace:  ns,
		pod:        pod,
		selector:   sel,
		container:  strings.TrimSpace(opts.Container),
		user:       opts.User,
		password:   opts.Password,
		host:       host,
		port:       opts.Port,
		kubeconfig: strings.TrimSpace(opts.Kubeconfig),
		exec:       execFn,
	}, nil
}
