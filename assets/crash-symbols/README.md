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

The Panel image also includes Breakpad `dump_syms`. At runtime it scans the
shared game releases and instance overlay trees for ELF modules, stores valid
generated symbols under the local crash-report symbol root, and keeps them
content-addressed by their Breakpad MODULE identity. Those generated Valve,
Steam, and game files are deployment-local artifacts; they are intentionally
not committed as public bundled symbols.

To refresh one of the files, download the gzip URL from `manifest.json`, verify
the gzip hash, decompress it, and then verify the recorded raw SHA-256 before
replacing the embedded file. Do not silently replace a symbol file with a
different debug identifier.
