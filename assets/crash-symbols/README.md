# Bundled Crash Symbols

This directory documents the two Breakpad symbol files embedded by
`internal/crashreports`:

- SourceMod `sourcemod.2.l4d2.so`
- Metamod:Source `metamod.2.l4d2.so`

The files were retrieved from the AlliedModders crash symbol service. The
repository keeps only these two public runtime symbol sets; it does not bundle
Valve or L4D2 symbols. `manifest.json` records the exact source URL and hashes
used to verify the embedded copies.

The symbols remain subject to the upstream project's applicable license and
distribution terms. They are used only to improve local crash diagnostics.

To refresh one of the files, download the gzip URL from `manifest.json`, verify
the gzip hash, decompress it, and then verify the recorded raw SHA-256 before
replacing the embedded file. Do not silently replace a symbol file with a
different debug identifier.
