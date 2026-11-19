# MageLift cloud providers

Each directory is a Go module that builds a signed release plugin. The CLI selects the plugin from the shop’s configuration, verifies the binary and uses the SDK protocol to plan and operate the environment.

| Module | Executable | Runtimes |
| --- | --- | --- |
| [AWS](aws/) | `magelift-provider-aws` | ECS Fargate, EKS |
| [Google Cloud](gcp/) | `magelift-provider-gcp` | GKE Autopilot, GKE Standard |
| [Azure](azure/) | `magelift-provider-azure` | AKS |

Run `make provider-aws-test` for the AWS module and `make floci-aws-test` for its emulator suite. Replace `aws` with `gcp` or `azure` for the other modules. The executable lives under each module’s `cmd/` and its emulator configuration is in `docker-compose.floci.yml`. [Adding a provider](../docs/adding-a-provider.md) · [Provider coverage](../docs/capability-matrix.md)
