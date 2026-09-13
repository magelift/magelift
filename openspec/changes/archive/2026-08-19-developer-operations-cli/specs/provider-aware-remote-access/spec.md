## ADDED Requirements

### Requirement: Remote access is capability-advertised

The `ssh` and `exec` commands MUST resolve access through the selected
provider/runtime module. The module MUST advertise the launcher, target type,
required credential references, supported services, and whether interactive
access is available.

#### Scenario: Remote access is unsupported

- **WHEN** the selected target has no verified access adapter
- **THEN** the command fails before launching a provider client, identifies the
  missing capability, and gives the supported local or provider-native
  alternative when one exists

#### Scenario: A session preview is requested

- **WHEN** an operator passes `--session-only`
- **THEN** the command returns a redacted, machine-readable target description
  without opening a session or mutating provider state

### Requirement: Remote command arguments preserve boundaries

The CLI MUST pass command arguments as an argv sequence or an equivalent
provider-safe structure. It MUST NOT concatenate user-controlled service,
container, or command values into a shell string. Context cancellation and
timeouts MUST reach both the provider client and the launcher process.

#### Scenario: A command contains shell metacharacters

- **WHEN** an operator runs a remote command containing spaces or shell
  metacharacters
- **THEN** the adapter receives the exact intended argument boundaries and no
  additional shell expression is evaluated

### Requirement: Remote access output is secret-safe and attributable

Every executed or previewed target MUST identify environment, provider/runtime,
service, container, launcher, and command intent. Output and evidence MUST
redact tokens, kubeconfigs, signed URLs, private keys, and secret values.

#### Scenario: A provider returns a credential-bearing target

- **WHEN** an adapter includes a credential reference or temporary access value
  in its target
- **THEN** the CLI removes the secret value from human output, structured
  output, logs, and evidence while retaining the non-secret reference needed to
  diagnose the access path

### Requirement: Private tunnels are capability-driven and loopback-bound

The `tunnel` command MUST resolve a logical target through an optional
provider/runtime tunnel adapter. Supported targets MAY include the application,
database, queue, search API, and management UIs, but an adapter MUST resolve
each target to a provider-owned resource or provider-native proxy before
launching. Local listeners MUST bind to loopback by default, validate local and
remote ports, and preserve argv boundaries. The command MUST NOT fall back to a
public endpoint or infer an unowned service name.

#### Scenario: A Kubernetes queue management tunnel is supported

- **WHEN** an operator requests a queue management UI tunnel for a deployed
  Kubernetes runtime
- **THEN** the adapter resolves the owned queue Service and management port,
  returns an argv-safe port-forward launcher bound to localhost, and keeps the
  session attached until cancellation

#### Scenario: GCP private Cloud SQL access is supported

- **WHEN** an operator requests a database tunnel for a GCP stack with an
  explicit Cloud SQL instance connection name
- **THEN** the adapter returns the provider-native Cloud SQL Auth Proxy with
  private-IP mode and the requested local port, without exposing database
  credentials or using the public database endpoint

#### Scenario: A service or management UI is not provisioned

- **WHEN** an operator requests a queue, search, or management UI target that
  the selected stack does not provision
- **THEN** the command fails before launching a process, identifies the missing
  capability, and does not substitute the application URL or another service

#### Scenario: A tunnel is cancelled

- **WHEN** the operator cancels a running tunnel or its context deadline expires
- **THEN** the launcher receives cancellation, temporary access files are
  removed, and the command returns a bounded diagnostic without leaking
  credentials
