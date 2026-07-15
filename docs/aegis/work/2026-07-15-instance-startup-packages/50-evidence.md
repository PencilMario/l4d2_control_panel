# Evidence

## Baseline

- Worktree: `E:/GithubKu/l4d2_control_panel/.worktrees/instance-startup-packages`
- Branch: `feature/instance-startup-packages`
- Base commit: `01baa4b`
- `go test -count=1 ./...`: PASS.
- `cd web && npm test -- --run`: PASS, 17 tests.
- `cd web && npm run build`: PASS.

Implementation evidence will be appended after each red/green slice.

## Task 1: Selected Package Persistence

- RED: `go test -count=1 ./internal/store` failed to compile because `domain.Instance.SelectedPackageID` did not exist.
- GREEN: added additive `selected_package_id`, legacy backfill from `package_version`, explicit selected/applied JSON fields and CRUD scan coverage.
- `go test -count=1 ./internal/store`: PASS.
- `go test -count=1 ./internal/store ./internal/httpapi`: PASS.
- `git diff --check`: PASS.
