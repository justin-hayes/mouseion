# Archived Operational Records

Dated, one-time operator records: data cutovers performed against a running
installation, and the validation log for the retired self-hosted CI runner.
They are kept as provenance for the decisions and commands that referenced
them, not as current setup instructions. Current setup lives in the
[operations guide](../../operations.md) and the
[development guide](../../development.md).

- [Migration baseline cutover (2026-09-15)](cutover-2026-09-15.md) — the
  one-time database recreation required by
  [ADR 0070](../../adr/0070-migration-and-documentation-reboot.md).
- [Retired Browse selections cutover (2026-10-06)](2026-10-06-vocabulary-browse-selections.md)
  — the backup-first runbook for `cmd/vocabularyselectioncutover`.
- [Self-hosted CI validation (2026-09-21)](ci-validation-2026-09-21.md) —
  cache validation on the former self-hosted runner, which CI no longer uses.
