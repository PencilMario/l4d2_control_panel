# VPK Pre-upload Cleanup and Resumable Queue

Multi-file VPK selection opens a confirmation dialog. Cleanup is selected by default per file; direct upload remains available. Cleanup runs in a Go WebAssembly worker and removes exactly extensionless entries plus .vtf, .mp3, .wav, .vmf, and .vmx entries.

The confirmation dialog must show the complete selected VPK list before any processing starts. Each row shows filename, original size, and the selected cleanup or direct-upload mode. Bulk controls switch all rows, while each row remains independently configurable.

Prepared bytes are stored in OPFS and task metadata in IndexedDB. Refresh restores unfinished jobs without file reselection. Cleanup is serial and uploads have concurrency two. Existing server cleanup, hashes, metadata, deduplication, and legacy upload clients remain compatible.
