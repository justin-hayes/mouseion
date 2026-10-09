# Maintenance cutover: retire Set Aside (ADR 0086, issue #1593)

Status: **prepared, not executed.** Nothing in the repository authorizes running
this against production. Justin is the responsible operator and owns one
backed-up maintenance window. The structural migration (`000033`), the conversion
tool (`cmd/bookdispositioncutover`), and the tests are reviewed in the pull
request; the operation itself is performed only after that review.

## What changes, and what must not

| Changes | Preserved (must be byte-for-byte unchanged) |
| --- | --- |
| Every still-legacy `set_aside` row in `book_dispositions` becomes `inbox`; its `revision` increases by exactly one | Existing `inbox` and `to_read` rows and revisions |
| A visible `book_visibility` row (`hidden = false`, `revision = 1`) is added for each Book that has none | Every existing Hide/Unhide row, including revision |
| `book_dispositions_disposition_check` becomes `inbox`/`to_read` only and is validated | Books, aliases, covers, membership, source materials, current analyses |
| One checkpoint row plus one audit row per converted Book | Current reading rows, frozen snapshots and snapshot vocabulary (reservations) |
| | Reading history (assertion and completion), Known vocabulary |
| | Prepared-deck and Custom-deck preparations and artifacts |

Mapping rules (ADR 0086 section 5): no Book is inferred Hidden or To Read. An
unread legacy Book projects **Inbox**; a legacy Book with assertion and/or
completion history projects **Read** (the bucket is derived from history, so
only the disposition is written, as Inbox). Finish and End are unchanged: Finish
still leaves the underlying disposition Inbox, End leaves To Read.

## Stages

The work is split into separately reviewed, separately recoverable stages:

1. **Structural** (migration `000033`, applied by `bookdispositioncutover migrate`):
   adds the `NOT VALID` replacement check, the checkpoint table, and the audit
   table. It converts no data and initializes no visibility. A database with no
   legacy rows is validated by the migration itself (fresh installs need nothing
   more). New or changed rows can no longer carry `set_aside` from this point.
2. **Inventory** (`plan`): read-only, produces the cutover manifest.
3. **Data conversion** (`apply`): one bounded transaction guarded by the manifest.
4. **Validation** (`verify` plus the checks below) before reopening.

## Preconditions

1. The release containing this change is built, and the pull request has had the
   required human review (workflows, `migrations/`, `doc/adr/`, `doc/product.md`).
   The new server refuses to start while any legacy disposition remains
   (`RequireNoLegacyDispositions`), so deploying it early cannot silently run
   over unconverted data; the old release must not be left running.
2. Choose the maintenance window. Announce nothing is needed beyond the operator:
   there is no mixed-version requirement and no compatibility shelf.
3. **Stop writers.** Stop the web server (the `mouseion-web` container or
   service) **and** every worker that writes affected tables: River workers run
   inside the server process, so stopping the server stops analysis, enrichment,
   catalogue sync, and prepared-deck translation. Confirm no process other than
   the operator's shell holds a connection:

   ```sh
   psql "$MOUSEION_DATABASE_URL" --set=ON_ERROR_STOP=1 \
     --command="SELECT pid, usename, application_name, state FROM pg_stat_activity
                WHERE datname = current_database() AND pid <> pg_backend_pid()"
   ```

   Do not proceed while an application connection remains.
4. Set `MOUSEION_DATABASE_URL` to the intended database and verify host,
   database name, and environment before every command below. The application
   stays **closed** until the final checks pass.

## Backup, then restore proof

Use a protected location; the dump contains all learner data. Use PostgreSQL
client tools matching the server's major version. Supply the restore-check
password with `PGPASSWORD` or `~/.pgpass`, never in a command line.

Build the tool once from the reviewed commit and use that binary for every stage
(replace `go run ./cmd/bookdispositioncutover` below with it), recording its
checksum in the change record so the same code plans, applies, and verifies:

```sh
go build -o "$dir/bookdispositioncutover" ./cmd/bookdispositioncutover
sha256sum "$dir/bookdispositioncutover" | tee "$dir/bookdispositioncutover.sha256"
```

The row-fingerprint inventory reads the full preserved tables twice (`plan`,
then `apply` under lock). Before the window, run `plan` and `apply` once against
the restored copy from the proof below and record the elapsed times, so the
maintenance window and the lock duration are known rather than guessed.

```sh
umask 077
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dir=/secure/backups
backup="$dir/mouseion-pre-set-aside-retirement-${stamp}.dump"

pg_dump --format=custom --dbname="$MOUSEION_DATABASE_URL" --file="$backup"
sha256sum "$backup" | tee "${backup}.sha256"
pg_restore --list "$backup" > "${backup}.toc"

createdb mouseion_cutover_restore_check
pg_restore --no-owner --no-privileges --exit-on-error --single-transaction \
  --dbname=postgres://USER@HOST/mouseion_cutover_restore_check "$backup"
psql --dbname=postgres://USER@HOST/mouseion_cutover_restore_check \
  --tuples-only --command='SELECT count(*) FROM public.book_dispositions'
```

Record that the restore completed and is queryable, then drop only that
disposable database. Do not proceed on an unverified backup. This dump is the
**coherent pre-reopen recovery point**: it contains the data and schema before any
stage below, so it and the manifest below are restored together or not at all.

## Stage 1: structural migration

```sh
go run ./cmd/bookdispositioncutover migrate --apply
psql "$MOUSEION_DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --command="SELECT conname, convalidated FROM pg_constraint WHERE conname = 'book_dispositions_disposition_check'"
```

`convalidated = false` is expected when legacy rows exist; `true` means there
were none. The migration is transactional: a failure applies nothing. It is
re-runnable (the migrator skips applied versions).

## Stage 2: inventory and manifest

```sh
set -o pipefail
go run ./cmd/bookdispositioncutover plan | tee "$dir/mouseion-set-aside-manifest-${stamp}.json"
sha256sum "$dir/mouseion-set-aside-manifest-${stamp}.json" | tee "$dir/mouseion-set-aside-manifest-${stamp}.json.sha256"
```

The manifest lists, per **owner** (nothing assumes a single learner): Books,
Inbox / To Read / legacy counts, Books without a disposition, missing visibility,
Hidden, and active commitments. It lists every **active commitment** with its
snapshot and disposition; every **changed row's original values** (disposition,
revision, `updated_at`, history category, projected bucket); the **expected
affected counts** (`totals`); the **preservation fingerprints** (row count and
content hash of each preserved table); and `blockers`.

Review it before continuing. Check at least:

- `owners` is the learner set you expect, and `totals.converted_dispositions`
  equals the number of Set Aside Books you expect.
- `history_category` matches your expectation (`unread` -> Inbox;
  `assertion`, `completion`, `mixed` -> Read).
- `blockers` is empty. Any blocker **stops the cutover for review** and is never
  repaired automatically. Blockers include a legacy Set Aside Book that is also
  a Current reading, an active commitment whose Book is not To Read, a
  disposition value outside `inbox`/`to_read`/`set_aside`, more than 10,000
  legacy rows, and more than 200,000 Books (unsafe volume for one transaction).

`plan` exits non-zero when blockers exist. Copy the manifest, its checksum, the
backup checksum, and the restore evidence into the protected change record.

## Stage 3: data conversion

Immediately before `apply`, repeat the `pg_stat_activity` check from the
preconditions: no application connection may have appeared since `plan` (the
tool rejects a changed database but cannot stop an old binary from starting).

```sh
set -o pipefail
go run ./cmd/bookdispositioncutover apply \
  --manifest "$dir/mouseion-set-aside-manifest-${stamp}.json" --apply \
  | tee "$dir/mouseion-set-aside-report-${stamp}.json"
```

In one transaction, with a 5 second lock timeout (a lock wait means a writer is
still running: the command fails and changes nothing), the tool:

1. locks the affected and preserved tables (`SHARE ROW EXCLUSIVE`);
2. returns the recorded outcome if a run with this exact manifest already
   committed (`already_applied: true`) and changes nothing else;
3. rebuilds the inventory and **refuses unless it equals the reviewed manifest**
   (any drift or edit, `ErrDispositionCutoverManifestMismatch`);
4. records the checkpoint, then maps only rows still `set_aside` **at the
   inventoried revision** to `inbox` (revision + 1), recording each original value;
5. proves the preserved tables are unchanged, then inserts visible visibility only
   for Books lacking a row, never touching an existing Hide;
6. proves no legacy value remains, every Book has visibility, existing Hidden rows
   are unchanged, and `VALIDATE CONSTRAINT` succeeds; then commits.

Retry safety:

- **Failure before commit** rolls everything back, including the checkpoint.
  Fix the cause and rerun the same command with the same manifest.
- **Lost acknowledgement** (committed, report not retained): rerun the same
  command. It returns the recorded checkpoint (`already_applied: true`) and does
  not repeat mapping, history, acceptance, reservation, or artifact changes, and
  never resets a Hide or a later learner edit.
- A manifest freshly planned **after** a committed conversion has no changes. Its
  application validates the constraint only and writes no second checkpoint
  (`nothing_to_convert: true`).
- Do not infer counts from a later rerun; use the saved report and manifest.

## Stage 4: validation before reopening

```sh
go run ./cmd/bookdispositioncutover verify | tee "$dir/mouseion-set-aside-verify-${stamp}.json"
```

`verify` exits non-zero unless: no legacy disposition remains; the constraint is
validated and excludes `set_aside`; every Book has visibility; the checkpoint's
converted count equals the number of audit rows; and no converted Book that is
still as-converted fails its recorded projection. Also run, and record, these
checks (all counts must be zero unless stated):

```sh
psql "$MOUSEION_DATABASE_URL" --set=ON_ERROR_STOP=1 <<'SQL'
SELECT count(*) AS legacy_dispositions FROM book_dispositions WHERE disposition NOT IN ('inbox','to_read');
SELECT count(*) AS books_without_visibility FROM books b
  WHERE NOT EXISTS (SELECT 1 FROM book_visibility v WHERE v.owner_id=b.owner_id AND v.book_id=b.id);
SELECT count(*) AS commitments_not_to_read FROM primary_goals g
  LEFT JOIN book_dispositions d ON d.owner_id=g.owner_id AND d.book_id=g.book_id
  WHERE d.disposition IS DISTINCT FROM 'to_read';
SQL
go run ./cmd/bookdispositioncutover plan > "$dir/mouseion-set-aside-postplan-${stamp}.json"
```

Compare the post-conversion plan with the manifest: every `preservation` entry
except `book_visibility` and `book_dispositions.unchanged` must match exactly
(ownership, membership, current snapshots and reservations, history, Known
vocabulary, source evidence, and artifact provenance), `changes` must be empty,
and `blockers` must be empty. The integration tests perform this comparison.

**Prove the retired routes and writers are gone before reopening.** The reviewed
release's tests establish this for the commit (record the passing
`go test ./internal/webapp/...` and `make test-integration` runs for the exact
commit being deployed). Confirm it on the deployed binary against the converted
database by starting it on a private loopback address only, probing, and
stopping it, while the public listener stays closed:

```sh
MOUSEION_HTTP_ADDR=127.0.0.1:18080 ./mouseion-server &   # plus the usual MOUSEION_* settings
# sign in with a throwaway session, then expect 404 for each retired route:
for path in /library/books/00000000-0000-0000-0000-000000000000/set-aside /reading/set-aside; do
  curl -s -o /dev/null -w "%{http_code} $path\n" -X POST "http://127.0.0.1:18080$path"
done
kill %1
```

(Unauthenticated requests may be redirected to sign-in before routing; with a
signed-in session and CSRF token the expected status is 404. If that cannot be
shown, treat the unit/integration evidence for the commit as the gate and note
the gap in the change record.) Start the public application only after
**every** check passes; then check, in a browser, that default My Books shows
Inbox / To Read / Read, that a legacy Book appears where projected, that new
catalogue-synced Books are visible Inbox, and that sync preserved dispositions.

## Ownership

The operator (Justin) owns the backup, restore proof, manifest review, command
output, validation evidence, and retention of all recovery material for the
change record. The agent that prepared this change performs none of these steps.

## Recovery

- **Before reopening** (any stage, or any failed validation): stop. Restore the
  verified full-database backup into an isolated database first and validate it;
  if production recovery is needed, restore that dump with the normal
  disaster-recovery procedure. Restore the backup and keep the manifest as the
  audit of what was attempted; they describe the same coherent pre-cutover
  point. Do not partially repair.
- **After activity resumes**: do not restore the pre-cutover dump (it would
  discard learner activity) unless broader recovery is explicitly decided. Prefer
  only **guarded forward correction** for state proven unchanged since cutover:

  ```sh
  go run ./cmd/bookdispositioncutover correct --apply \
    --cutover <cutover_id from the report> --owner <owner_id> --book <book_id> --to to_read
  ```

  It re-decides a converted Book (`inbox` or `to_read`) only when its
  disposition revision, visibility, history category, and Current reading status
  are exactly as the cutover left them; otherwise it refuses
  (`ErrDispositionCutoverStateChanged`) and never overwrites a later learner
  choice or completion. Replaying the same correction is a no-op, and a second,
  different correction of an already-corrected Book is refused too (it needs a
  deliberate manual decision, not the tool).
- **Down migrations are not presumed safe.** `000033.down` drops the checkpoint
  and audit and restores the three-value constraint, but never maps Inbox back
  to Set Aside (the original values live only in the manifest and audit).
  Do not run it against production after conversion.

## What this operation never does

It does not edit the baseline or any shipped migration, change candidate
thresholds or Known-vocabulary semantics, convert, hide, or delete any Book
beyond the disposition mapping, delete Custom-deck records or artifacts, or alter
Reading history, snapshots, or reservations.
