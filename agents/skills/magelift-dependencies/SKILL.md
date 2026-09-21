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

The report separates required tools from optional capabilities. A cloud target
does not require Docker unless the selected command actually builds or runs a
local container. Deploy and bootstrap do not need a Pulumi CLI; that engine
runs inside the MageLift process. Read the capability and install hint beside
each failed check before changing the machine.

Use the explicit installer only after reviewing the proposed package-manager
commands:

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
