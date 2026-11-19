package stack

import (
	"encoding/base64"
	"fmt"

	"github.com/pulumi/pulumi-azure-native-sdk/dbformysql/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

const mysqlAPIPackage = "azure-native_dbformysql_v20250601preview"
const azureNativePluginVersion = "3.32.0"

// Azure exposes MySQL 8.4 through the 2025-06-01-preview server API.
// Pulumi's parameterized Azure plugin reads a base64-encoded module and API version.
func newMySQLServer(ctx *pulumi.Context, name string, args *dbformysql.ServerArgs, version string, opts ...pulumi.ResourceOption) (*dbformysql.Server, error) {
	if version != "8.4" {
		return dbformysql.NewServer(ctx, name, args, opts...)
	}
	ref, err := ctx.GetOrRegisterPackageRef(mysqlAPIPackage+":"+azureNativePluginVersion, func() (*pulumirpc.RegisterPackageRequest, error) {
		parameter := base64.StdEncoding.EncodeToString([]byte(`{"module":"dbformysql","version":"v20250601preview"}`))
		return &pulumirpc.RegisterPackageRequest{Name: "azure-native", Version: azureNativePluginVersion, Parameterization: &pulumirpc.Parameterization{
			Name: mysqlAPIPackage, Version: azureNativePluginVersion, Value: []byte(parameter),
		}}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("register Azure MySQL 2025-06-01-preview API: %w", err)
	}
	opts = append(opts, pulumi.Version(azureNativePluginVersion), pulumi.Aliases([]pulumi.Alias{{Type: pulumi.String("azure-native:dbformysql:Server")}}))
	var server dbformysql.Server
	if err := ctx.RegisterPackageResource(mysqlAPIPackage+":dbformysql:Server", name, args, &server, ref, opts...); err != nil {
		return nil, err
	}
	return &server, nil
}
