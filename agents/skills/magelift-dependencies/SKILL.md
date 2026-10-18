---
name: magelift-dependencies
description: >-
  Check the local tools MageLift needs for Docker, cloud access, builds, and
  imported database dumps. Use when doctor reports a missing or unreachable tool.
version: 1.0.0
---

# Check MageLift dependencies

Run the readiness check before a new workstation setup or a CI job:

```sh
magelift doctor
```

Before cloud login, use `magelift doctor --local`. It skips credential probes
and reports missing or invalid configuration alongside local checks without
acquiring a provider. It does not install tools unless separately requested.

To create a voluntary local support report:

```sh
magelift doctor --local --export doctor-metadata.json
```

The new JSON file is mode 0600; existing files and symlinks are refused. Its
fixed field allowlist includes known CLI/OS/target categories, check statuses,
counts and required/optional tool classifications. Names, paths, account IDs,
config values, free-text errors, tool
output, URLs, tokens and logs are omitted. There is no upload. The full console
report remains private diagnostic output; review raw logs separately before
sharing any, because extension data is not covered by this export contract.

The report separates required tools from optional capabilities. A cloud target
does not require Docker unless the selected command actually builds or runs a
local container. Deploy and bootstrap do not need a Pulumi CLI; that engine
runs inside the MageLift process. Read the capability and install hint beside
each failed check before changing the machine.

Review package-manager commands before accepting an installation. Signed
release operations and remote session launchers can offer to install missing
tools through the same allowlist. Docker and cloud login remain prerequisites.
In CI, `--yes` authorizes installation; otherwise missing tools produce a
readiness error without prompting outside an interactive terminal.

To prepare the workstation explicitly:

```sh
magelift doctor --install-dependencies
```

MageLift asks for confirmation and only invokes allowlisted package-manager
arguments. It does not run a shell pipeline or install an arbitrary executable.
For CI, install the tools in the runner image and keep `magelift doctor` as a
read-only gate.

When a tool is present but unreachable, fix its daemon, credentials, or PATH
first. Do not treat an unreachable Docker daemon or provider session as a
missing binary.

The first-party GCP provider is acquired only when a command activates that
target. A valid project lock takes priority over the CLI's fixed release
version. `help`, `version`, `doctor` and envelope validation do not acquire it.
For offline preparation or explicit repair, run `magelift providers install`
while online. A corrupt cache is an error; do not remove or replace a project
pin to hide a signature, digest or protocol failure. Community provider names
in YAML do not enable automatic installation. Release archives need no Go
toolchain on the user's machine.
