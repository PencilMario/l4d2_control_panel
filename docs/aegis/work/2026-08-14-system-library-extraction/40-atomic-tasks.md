# Atomic Tasks

- [x] Add failing Docker archive tests for regular files, symlinks, directories, and size limits.
- [x] Implement bounded Docker archive file reading with safe symlink resolution.
- [x] Add failing crash-artifact preparation tests for libc extraction, Debug ID mismatch, and missing containers.
- [x] Implement fixed-path Linux system-library candidate selection and artifact persistence.
- [x] Add failing worker test proving preparation runs before Stackwalk and remains non-fatal.
- [x] Wire the preparer into `cmd/panel` and move recovery start after Docker dependencies exist.
- [x] Bind the source game container at pre-submit and preserve it through container rebuilds.
- [x] Add direct system-library path tests and continue through mismatched candidate files.
- [x] Update README and run targeted/full tests plus build verification.
