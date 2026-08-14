# Task Intent

## Requested outcome

Extend crash analysis so missing Linux system ELF modules such as `libc.so.6` can be copied from the exact managed game container that produced the report, stored as the existing binary artifact type, and symbolized from that same binary when possible.

## Scope

- Read regular files from a managed game container through Docker's archive API.
- Match only known Linux runtime-library names and fixed system-library directories.
- Verify the extracted file's Breakpad MODULE debug identifier before storing it.
- Reuse existing binary, generated-symbol, report association, download, and Stackwalk paths.
- Treat extraction as best effort; upload and Stackwalk remain available when extraction fails.

## Non-goals

- Recovering binary bytes from the minidump itself.
- Extracting arbitrary application or user paths.
- Using the Panel container's libc or the host filesystem as a substitute.
- Changing the public Accelerator upload protocol.

## Risk hints

The Docker archive response is a tar stream and may contain symlinks, directories, malformed headers, or oversized files. The container may have exited or been removed before analysis. Debug ID and architecture must be checked before an artifact is associated with a report.
