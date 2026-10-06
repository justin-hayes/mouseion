# Data cutover: retired Browse selections (2026-10-06)

This is a one-time, **data-only** cleanup after #1507 removed Browse selection
and Custom-deck access. It does not drop or alter schema. The rows belong to
the learner who selected each `(language, canonical_lemma, upos)` identity;
they are not Known vocabulary, a Reading snapshot, or a Custom-deck artifact.
Custom deck identities and preparations remain stored but inaccessible.

## Preconditions and safety

1. Deploy a release containing #1507 and verify the Browse selection and
   Custom-deck UI/actions are unavailable. Stop or drain every older web
   instance before continuing; an older process can still write the table.
   The cutover takes a table lock, but that is not a permanent write barrier.
2. Set `MOUSEION_DATABASE_URL` to the intended database and verify its host,
   database name, and environment before using any command below.
3. Make the backup below, test that it can be restored to an isolated database,
   and save its checksum and verification evidence with the operator's change
   record. Do not proceed on an unverified backup.

## Inventory and verified backup

Use a protected location with adequate free space; the dump contains all
learner data. Restrict file permissions and retain it under the normal backup
retention policy. These commands use PostgreSQL client tools matching the
server's supported major version:

```sh
umask 077
stamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="/secure/backups/mouseion-pre-browse-selection-cutover-${stamp}.dump"
inventory="/secure/backups/mouseion-browse-selection-inventory-${stamp}.csv"

pg_dump --format=custom --dbname="$MOUSEION_DATABASE_URL" --file="$backup"
sha256sum "$backup" | tee "${backup}.sha256"
pg_restore --list "$backup" > "${backup}.toc"
```

Restore the dump into an isolated, disposable database before proceeding;
`pg_restore --list` alone only checks that the archive can be read. For example,
create a temporary database on a non-production PostgreSQL instance and run:

```sh
createdb mouseion_cutover_restore_check
pg_restore --no-owner --no-privileges --exit-on-error --single-transaction \
  --dbname=postgres://USER:PASSWORD@HOST/mouseion_cutover_restore_check \
  "$backup"
psql --dbname=postgres://USER:PASSWORD@HOST/mouseion_cutover_restore_check \
  --tuples-only --command='SELECT count(*) FROM public.vocabulary_browse_selections'
```

Record that the restore completed and the restored selection table is queryable.
Drop only that disposable validation database after recording the result.
Keep the original dump unchanged.

Capture exact identities, timestamps, and the owner/language extent before
deletion. This inventory is sensitive learner data too:

```sh
psql --dbname="$MOUSEION_DATABASE_URL" --csv --set=ON_ERROR_STOP=1 \
  --command='SELECT owner_id, language, canonical_lemma, upos, added_at
            FROM public.vocabulary_browse_selections
            ORDER BY owner_id, language, canonical_lemma, upos' \
  > "$inventory"
sha256sum "$inventory" | tee "${inventory}.sha256"
psql --dbname="$MOUSEION_DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --command='SELECT owner_id, language, count(*) AS rows
            FROM public.vocabulary_browse_selections
            GROUP BY owner_id, language ORDER BY owner_id, language'
```

Compare the summed extent with the inventory row count (excluding its header),
then retain the CSV, checksum, backup checksum, and restore-verification result
in the operator's protected change record. Empty selections have no rows and
therefore no inventory group.

## Apply and verify

Only after the preconditions above are recorded, execute the explicit command:

```sh
set -o pipefail
go run ./cmd/vocabularyselectioncutover --apply \
  | tee "/secure/backups/mouseion-browse-selection-cutover-${stamp}.json"
```

The command serializes against writers, inventories row counts per owner and
language, deletes the rows in one transaction, checks the deleted count against
the inventory and verifies the table is empty before committing. Save the JSON
report with the inventory and backup evidence. A report with no extents and
`deleted: 0` means there was nothing to remove; running the command again after a
successful cutover is safe and reports an empty extent.

Verify the deployed application remains on the #1507 behavior and run the
post-cutover checks:

```sh
psql --dbname="$MOUSEION_DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --command='SELECT count(*) AS remaining FROM public.vocabulary_browse_selections'
```

The count must be zero. Do not remove the table or the stored Custom-deck
records/APKGs as part of this operation. This cutover never maps selected
identities to Known vocabulary or into a Book/Reading snapshot. The integration
test also verifies unchanged active and historical Reading snapshots, known
vocabulary, retained ordinary and Custom-deck APKG bytes, and safe reruns.

## Ownership, retry, and recovery

- **Owner:** the operator applying this runbook owns the backup, inventory,
  command output, verification, and retention of the recovery material.
- **Idempotency/retry:** delete and verification share one transaction. A
  failure before commit rolls back the deletion. If the command exits after a
  successful commit but before its report is retained, rerun it: it reports
  zero rows. Use the pre-cutover inventory and database logs to reconcile a
  lost report; do not infer a deletion count from a later rerun.
- **Recovery:** restore the verified full-database backup to an isolated
  database first and validate it. If production recovery is needed, coordinate
  maintenance and restore that backup using the normal disaster-recovery
  procedure; this is a full database restore, not a row-level rollback. It
  discards all database changes made after the backup unless separately
  recovered/replayed. The CSV is an audit record, not a supported restore input.
- **Rollback limitation:** there is intentionally no reverse migration or
  automatic rollback. The UI/writers are retired, and restoring rows by hand
  would recreate obsolete state with no supported consumer. Keep the backup
  according to policy in case broader database recovery is required.

This operation does not edit the immutable baseline or any shipped migration,
does not perform structural DDL, and does not delete Known vocabulary, Reading
state/history/reservations, Custom-deck identities/preparations/APKG bytes, or
packages already downloaded into external Anki clients.
