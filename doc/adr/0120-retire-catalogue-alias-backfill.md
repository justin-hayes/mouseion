# ADR 0120: Retire the catalogue alias backfill

Status: **Accepted** · Date: 2026-11-21 · Author: Justin + OpenCode

Amends [ADR 0044](0044-catalogue-entry-connection-scoped-identity.md) by
retiring its operator-run catalogue alias backfill, and
[ADR 0091](0091-go-package-layering.md)'s example of that command as a pgx
owner. Implemented by
[issue #2530](https://github.com/justin-hayes/mouseion/issues/2530).

## Context

ADR 0044 made a catalogue-entry alias's identity owner + connection + entry and
introduced a one-time operator command for legacy rows. The command examined
catalogue-entry aliases with a null `connection_id`, walked every language feed
of the owner's OPDS connections, and assigned the single connection whose feed
advertised the alias's source identifier. Absent, ambiguous, or unreadable
feeds failed loudly rather than guessing. Its last step validated the
`book_aliases_connection_contract` constraint, which migration `000054` now
validates instead ([#2529](https://github.com/justin-hayes/mouseion/issues/2529)).

In the only deployment the backfill's work is complete: on 2026-11-21 all 74
catalogue-entry aliases had a connection. The command therefore changes nothing
there, while still requiring maintenance whenever the catalogue reader,
connection store, or alias tables change.

## Decision

1. **Delete the operator command.** `cmd/cataloguebackfill` is removed, and the
   maintenance command is removed from the operations guide.
2. **Delete the service operation and its helpers.** The catalogue sync service
   no longer resolves legacy aliases against connection feeds, and the alias
   store interface and unfiltered feed reader used only by that operation are
   removed.
3. **Delete the persistence operations used only by it.** Listing unscoped
   catalogue-entry aliases and assigning an alias's connection after the fact
   are removed with their sqlc queries and tests. New catalogue-entry aliases
   still carry their connection at write time.
4. **Keep the record.** The backfill stays in Git history. ADR 0044's
   connection-scoped identity decision remains accepted; only its one-time
   data-migration rollout is historical.

## Consequences

- Learners and operators see no change. This decision runs no migration and
  repairs no data. Migration `000054` validates the alias connection constraint.
- No operator backfill command remains.
- The alias contract still rejects a connectionless catalogue-entry alias at
  write time.
- A future repair of legacy catalogue aliases must design its own operation
  against the then-current reader and alias tables; this decision does not
  authorize reusing the retired one.

## Alternatives considered

- **Keep the dormant command.** Rejected: it has no work left in the
  deployment, still has to be maintained against the catalogue and persistence
  APIs, and invites an operator to run a stale feed scan after future schema
  changes.
- **Fold the backfill into startup.** Rejected: ADR 0044 deliberately did not
  make it a startup side effect, and startup would then read every catalogue
  feed before becoming healthy.
- **Rewrite history to remove the backfill commit.** Rejected: the command and
  its rollout remain useful provenance and are reproducible from Git history.

## References

- [ADR 0038: Schema-change governance and migration review policy](0038-schema-change-governance.md)
- [ADR 0044: Catalogue-entry alias identity retains the catalogue connection](0044-catalogue-entry-connection-scoped-identity.md)
- [ADR 0091: Go package layering](0091-go-package-layering.md)
- [Issue #2529: Validate the catalogue alias connection constraint](https://github.com/justin-hayes/mouseion/issues/2529)
- [Issue #2530: Retire the catalogue alias backfill](https://github.com/justin-hayes/mouseion/issues/2530)
