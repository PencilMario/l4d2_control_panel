# Atomic Tasks

- [x] Add upload-side no-AI regression coverage and preserve manual AI assertions.
- [x] Remove production upload callback that honored `AutoCrashAnalysis`.
- [x] Update the instance setting copy and remove the obsolete automatic-AI toggle.
- [x] Run the focused UI regression for the setting copy.
- [x] Make the manual analysis API default to Stackwalk-only unless `ai=true` is explicit.
- [x] Add Worker coverage for Stackwalk-only execution without AI calls.
- [x] Run focused and full local verification.
- [x] Commit the fix without unrelated worktree changes.
- [x] Deploy only Panel to 安可服 and verify health plus manual-analysis behavior.
- [x] Record evidence and residual risk.

## Closure notes

- Implementation commits: `491dd6f` and `61f87e3`.
- Remote Panel revision: `61f87e3`; game containers were preserved during the Panel-only rollout.
- Manual analysis boundary: upload/load/selection emitted no analyze request; explicit manual analysis sends `{"ai":true}`; omitted/false analysis requests remain Stackwalk-only.
- Residual risk: no new Accelerator upload was generated against production during this acceptance slice. Existing upload callback and protocol regressions remain covered by local tests.
