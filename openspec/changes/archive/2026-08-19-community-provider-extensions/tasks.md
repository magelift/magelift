## 1. Public contract

- [x] 1.1 Define the public extension descriptor, API version, target, capability, and provenance types.
- [x] 1.2 Add validation for target IDs, capability IDs, required outputs, and certification tiers.
- [x] 1.3 Add the internal adapter from the public contract to `platform.ModuleRegistry`.

## 2. Community workflow

- [x] 2.1 Update `examples/custom-cli` to import only public extension packages.
- [x] 2.2 Add a clean-cache build test for the custom binary (run on CI or a high-memory runner; the local isolated-cache proof passes with bounded parallelism and memory).
- [x] 2.3 Add extension diagnostics to the CLI.
- [x] 2.4 Document compile-time installation, version compatibility, and certification evidence.

## 3. Security and compatibility

- [x] 3.1 Reject unsupported API versions before registration.
- [x] 3.2 Add provenance and digest fields to extension diagnostics.
- [x] 3.3 Add tests proving the default binary does not load arbitrary working-directory files.
- [x] 3.4 Record signed remote execution as a separate post-RC design, not an RC1 dependency.
