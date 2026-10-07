# Upgrade backup and Release CI implementation plan

**Goal:** Private versioned rollback snapshots without changing identity preservation;
tag-driven verified Releases without publishing this round.

**Architecture:** Whitelist snapshots before installer stop/overwrite, kernel lock,
five completed generations. Canonical packaging uses a SHA256-pinned base ZIP and
helpers from tagged source with Go1.26.6. Read-only CI tests/builds before a write
job stages a draft, checks remote assets, publishes and advances update.json.

**Tech stack:** Android shell/toybox, Python stdlib, GitHub Actions, gh CLI.
**Spec:** User requirements; operational docs UPGRADE_BACKUPS.md / RELEASE_CI.md.

## Constraints and review focus

- Main only; no tag/Release this round; no daemon/DNS/routing/WebUI behavior changes.
- Preserve live state/config/markers bytes and modes; never back up state or logs.
- Reject symlinks/special files, invalid versions, wrong hashes and toolchains.
- Failed tests/uploads/verification cannot expose a partial public release.
- Complete published assets survive pointer failure; retries cannot downgrade main.

## Tasks (inline execution)

1. Write snapshot/retention/security tests, verify red; add installer backup and
   installed version tracking, verify green and old upgrade tests.
2. Write release metadata/bundle/state-machine tests, verify red; implement pinned
   preparation, metadata-derived packaging, draft publishing and guarded pointer.
3. Add pinned-actions tag workflow, separate read/write jobs and full checks.
4. Run all tests and deterministic double-build; document mocked failures and the
   unexecuted remote CI limitation. Commit/push main only.
