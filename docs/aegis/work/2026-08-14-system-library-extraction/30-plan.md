# System Library Extraction Plan

**Goal:** Make missing Linux runtime libraries available as report binary artifacts without weakening existing crash-report or Docker security boundaries.

**Architecture:** Add a bounded Docker container-file reader that parses the archive response and rejects unsafe entries. Add a crash artifact preparer that selects the managed game container by instance ID, tries fixed system-library candidates, verifies the extracted ELF with `dump_syms`, and writes the existing binary/generated-symbol artifacts. Inject the preparer into the analysis worker before Stackwalk.

**Compatibility boundary:** Public upload endpoints, report format, artifact download endpoints, existing local game symbol indexing, and Stackwalk behavior remain compatible. Extraction failure is non-fatal.

**Verification:** Docker archive unit tests, preparer tests with fake container/file sources and fake symbol generator, crash worker integration test, `go test ./...`, and `go build ./cmd/panel`.

## Slices

1. Docker archive reader and tar safety.
2. System-library candidate matching and Debug ID verification.
3. Worker integration and Panel wiring.
4. Documentation and regression verification.
