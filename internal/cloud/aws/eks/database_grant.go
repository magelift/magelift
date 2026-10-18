package eks

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const databaseGrantCommand = `for attempt in $(seq 1 60); do
  if mysql --protocol=tcp --connect-timeout=10 --host="$MAGELIFT_DB_HOST" --port=3306 --user="$MAGELIFT_DB_ADMIN_USER" --password="$MAGELIFT_DB_ADMIN_PASSWORD" --database="$MAGELIFT_DB_NAME" --execute="$MAGELIFT_DB_GRANT_SQL"; then
    exit 0
  fi
  sleep 5
done
echo 'database privilege grant did not complete within 5 minutes' >&2
exit 1`

func newDatabaseGrantJob(
	ctx *pulumi.Context,
	runtimeName string,
	databaseWriter pulumi.StringInput,
	databaseName string,
	databaseAdminUsername string,
	adminSecretName string,
	grantStatement string,
	opts ...pulumi.ResourceOption,
) (*batchv1.Job, error) {
	return batchv1.NewJob(ctx, runtimeName+"-db-grant", &batchv1.JobArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(runtimeName + "-db-grant"),
			Labels: pulumi.StringMap{
				"magelift.io/workload": pulumi.String("database-grant"),
			},
		},
		Spec: &batchv1.JobSpecArgs{
			ActiveDeadlineSeconds: pulumi.IntPtr(900),
			BackoffLimit:          pulumi.IntPtr(2),
			Template: &corev1.PodTemplateSpecArgs{
				Spec: &corev1.PodSpecArgs{
					RestartPolicy: pulumi.StringPtr("Never"),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("grant"),
							Image: pulumi.String("mysql:8.4"),
							Command: pulumi.StringArray{
								pulumi.String("/bin/sh"), pulumi.String("-ec"), pulumi.String(databaseGrantCommand),
							},
							Env: corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name:  pulumi.String("MAGELIFT_DB_HOST"),
									Value: databaseWriter.ToStringOutput().ToStringPtrOutput(),
								},
								&corev1.EnvVarArgs{Name: pulumi.String("MAGELIFT_DB_NAME"), Value: pulumi.StringPtr(databaseName)},
								&corev1.EnvVarArgs{Name: pulumi.String("MAGELIFT_DB_ADMIN_USER"), Value: pulumi.StringPtr(databaseAdminUsername)},
								&corev1.EnvVarArgs{Name: pulumi.String("MAGELIFT_DB_GRANT_SQL"), Value: pulumi.StringPtr(grantStatement)},
								&corev1.EnvVarArgs{
									Name: pulumi.String("MAGELIFT_DB_ADMIN_PASSWORD"),
									ValueFrom: &corev1.EnvVarSourceArgs{SecretKeyRef: &corev1.SecretKeySelectorArgs{
										Name: pulumi.StringPtr(adminSecretName),
										Key:  pulumi.String("password"),
									}},
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu": pulumi.String("500m"), "memory": pulumi.String("1Gi"),
								},
								Requests: pulumi.StringMap{
									"cpu": pulumi.String("250m"), "memory": pulumi.String("1Gi"),
								},
							},
						},
					},
				},
			},
		},
	}, opts...)
}

func databaseGrantStatement(databaseName, username string) (string, error) {
	if strings.TrimSpace(databaseName) == "" || strings.TrimSpace(username) == "" {
		return "", errors.New("database name and username are required")
	}
	if hasControlCharacter(databaseName) || hasControlCharacter(username) {
		return "", errors.New("database name and username must not contain control characters")
	}
	return fmt.Sprintf(
		"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%';",
		escapeMySQLIdentifier(databaseName), escapeMySQLString(username),
	), nil
}

func escapeMySQLIdentifier(value string) string {
	return strings.ReplaceAll(value, "`", "``")
}

func escapeMySQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func hasControlCharacter(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}
