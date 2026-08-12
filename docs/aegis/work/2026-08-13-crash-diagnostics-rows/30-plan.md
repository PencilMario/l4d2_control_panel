# Crash Diagnostics Rows Plan

## Design

Use one `crash-diagnostics` vertical flow containing four `crash-diagnostic-row` sections in this order: Stackwalk, AI 诊断, 上传元数据, 崩溃模块. Each row has a compact summary with icon, title, description/status and right-side actions; expandable content stays below its own summary. Stackwalk is expanded by default. AI, metadata and modules are collapsed by default. React state controls expansion so existing button names and focus behavior remain deterministic.

Add `stackwalk.ts` with a pure `parseStackwalk` function. It returns frame entries for numbered lines, splitting `module!symbol + offset` when possible, and log entries for every other non-empty input line. Frame raw text is retained. The UI renders frame index, module, symbol, offset and `Found by` source while allowing the original Stackwalk file to be downloaded.

## Compatibility

Existing API calls, response types, download query parameters, AI reader route-in-place behavior and module artifact downloads remain unchanged. The former two-column analysis/data containers and green oversized AI action are retired from this page.

## Verification

- Parser unit tests cover compact frames, real `Found by` lines, mixed logs and unrecognised lines.
- Crash page tests cover four ordered diagnostic rows, one DOM row per parsed frame, metadata/module expansion, and the AI reader trigger.
- Run the focused Vitest file, all frontend Vitest tests, `npm run build`, and Playwright at desktop and mobile widths against the deployed panel or an equivalent fixture.
