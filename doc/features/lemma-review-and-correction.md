# Learner review of analyzer lemmas

Status: **First manual-correction milestone shipped** · Date: 2026-09-29

The first manual correction milestone is available from Reading: exact observed
form lookup, per-occurrence lemma correction, direct prepared-deck projection,
corrected Book vocabulary insights, and corrected current-reading snapshots are
implemented. Automatic review flags and suggestions, and occurrence exclusion
remain future work.

## Problem and goal

An analyzer can assign the wrong lemma to an otherwise valid source occurrence.
In “Ein Reiter zielt mit seinem Speer auf einen Drachen,” Stanza assigns
`Drach` to `Drachen`; the learner's card should instead use the appropriate
vocabulary identity for the sentence. Dictionary membership cannot settle this
on its own: the current Kaikki index has no `Drach` entry, has dragon-related
senses under both `Drache` and `Drachen`, and lacks valid compounds such as
`Basketballspiel`. A fuzzy alternative or frequency prior is evidence for
review, not permission to silently change the learner's vocabulary.

Give the learner a bounded, Book-scoped way to examine high-risk analyzer
assignments and correct individual occurrences before their identities are
frozen into a current-reading vocabulary snapshot or prepared deck. Keep the
source and analyzer output intact, avoid an in-app administrator role, and
never make Reading depend on an available LLM.

## Ownership and vocabulary identity

The immutable normalized corpus retains the analyzed source, offsets, raw
lemma, analyzer-derived canonical lemma, part of speech, and analysis
provenance. Learner decisions are owned by that learner and scoped to an
occurrence in one Book's **exact analysis**; they are not edits to a shared
corpus artifact or language-wide normalization profile.

An occurrence has one effective vocabulary outcome: keep its analyzer-derived
identity, replace its canonical lemma with a learner-confirmed correction while
keeping the analyzed POS, or exclude it from vocabulary selection. A confirmed
correction may be made without a dictionary entry and without an automatic
review flag; the submitted citation form is validated and normalized by the
study language's current profile. Exclusion does not remove source evidence,
mark the identity Known, exclude other occurrences, or shrink the analyzable
token denominator used for coverage. An excluded token contributes neither
Known nor Reserved occurrences; exclusion does not artificially improve
coverage. If excluded occurrences make a coverage target unattainable from
eligible vocabulary, show the existing not-reachable outcome rather than
changing the denominator.

All owner-specific learner-facing vocabulary derived from this analysis uses
effective identities consistently: occurrence counts, the recurring floor,
Known and Reserved eligibility, representative sentence/target selection,
Book vocabulary insights, current-reading snapshot selection, dictionary and
contextual-Gloss lookup at deck freeze, and prepared-deck identity. A
correction can merge counts with another lemma, raise or lower an identity
across the default three-occurrence floor, or remove a card because the
corrected identity is already Known. Analysis and concordance retain the raw
analyzer attribution as evidence rather than silently rewriting it.

Decisions are durable for that exact analysis and attributable to the learner;
the learner can revise them freely before its snapshot or deck specification
freezes. Later corrections require the explicit recovery paths below, not an
edit to a frozen artifact. Decisions never copy automatically to a later
analysis of the Book, even when surfaces or offsets look similar. A dictionary
refresh or new suggestion may add non-blocking evidence but cannot revoke a
recorded keep, correction, or exclusion or create a retroactive Reading gate.

## Detect, suggest, review

The first automatic detector is German-only. It uses the existing local Kaikki
index and available analyzer evidence to flag **high-risk** content-word
occurrences when an absent exact lemma has a plausible indexed alternative;
absence alone is not a flag. It considers both current recurring candidates
and plausible corrections that could join other occurrences and cross the
recurrence threshold. Suggestions do not assert correctness and do not choose
an identity. The learner can manually review eligible content-word occurrences
in German, Italian, and Modern Greek; automatic flags for additional languages
need separate precision evidence. Changing POS or rescuing a token excluded by
its analyzer POS is not part of this first workflow.

Review flags and their evidence provenance are retained for the exact analysis
until resolved by the learner. When local index evidence is unavailable, show
that assessment is unavailable rather than claiming the analysis has passed a
dictionary check; no new flags are invented and only previously recorded
unresolved high-risk flags can pause Reading.

The Book-scoped Reading review surface shows each flagged occurrence in its
complete sentence, the observed form, analyzer lemma, why it was flagged, and
any source-attributed local alternatives. A learner may request an optional
LLM lemma suggestion while viewing an occurrence. Only the bounded target,
analyzed lemma/POS, one source sentence, and relevant lexical evidence may be
sent under the existing configured-provider boundary: never the full Book,
owner identity, catalog credentials, or reading history. This is a proposal,
not an automatic correction or a second deck-generation-time identity
decision; unavailable provider help does not prevent manual review. Do not
introduce a global frequency source merely to certify lemmas; later frequency
data, if separately justified, can rank suggestions but cannot prove them.

The learner can keep the analyzed lemma, enter a corrected canonical lemma, or
exclude that occurrence. Before confirmation, show the resulting effective
identity and the effect on that Book's recurrence, eligibility, snapshot/deck
membership, and Known matching. No numeric confidence score implies proof.
An explicit multi-occurrence action lists the contexts to be included; default
to just one, never all matching surfaces or all Books. A compact “Correct
another word” exact-observed-form lookup lists matching occurrences in this
analyzed Book with sentence context, so an unflagged error is reachable without
building full-text search or a concordance screen. The review uses ordinary
server-rendered forms and existing accessible Book/Reading patterns, not a new
admin dashboard or retired Book-detail route.

## Gates and frozen history

Unresolved **high-risk** flags for occurrences that could affect the current
snapshot/deck pause both Start reading and direct Prepare deck for that Book.
The learner resolves each through keep, correction, or exclusion. A dictionary
miss without a credible alternative does not create a mandatory task. Checking
the relevant review decisions and effective vocabulary at either freeze must
be consistent even if another request changes a decision at the same time.
Reading must not issue a live LLM call or wait on a failed local index merely
to freeze a snapshot. If evidence is unavailable, no new flags are generated;
already-recorded unresolved flags still require a decision. Flags discovered
after a snapshot freezes may be shown as non-blocking evidence but cannot
silently alter an active reading.

The first recovery path for an active reading is explicit: stop without
completion, review/correct, then start again with a new snapshot and
re-prepare any affected deck. A ready artifact from before a correction
remains historical and downloadable; its changed vocabulary cannot be fixed
by a presentation-only re-render. If a deck is already ready without an active
reading, accepting a later correction is coupled to explicit re-preparation;
the old artifact remains available. If both a snapshot and deck exist, stop,
correct, then restart and re-prepare. A completed reading and its
Known-vocabulary facts are never silently rewritten:
explicit reconciliation of historical Known vocabulary is separate work.
Changing a canonical lemma can change the Anki note identity, so re-importing
a replacement deck may leave the old imported note; explain that the learner
may need to remove it manually in Anki. Mouseion does not delete Anki notes.

## Validation

- A German `Drachen` occurrence analyzed as `Drach` is offered for review when
  credible alternative evidence exists; the learner chooses the appropriate
  lemma for its sentence. The program does not infer `Drache` from spelling
  alone or rewrite a different `Drachen` used as a kite.
- A valid unindexed compound such as `Basketballspiel` is not blocked merely
  for lacking an exact Kaikki entry. Human-reviewed German fixtures must have
  **no incorrect blocking flags** before activating the detector; missing a
  difficult case is tolerable because manual correction remains available.
- Correcting one occurrence merges or splits effective counts and honors the
  recurring floor, Known exclusions, sentence selection, Reading snapshot, and
  deck output. Exclusion leaves coverage's analyzable-token denominator intact
  and does not imply Known vocabulary.
- Owner isolation, concurrent review/freeze attempts, re-analysis, an
  unavailable index or LLM, late evidence, and the already-ready/active/finished
  recovery cases preserve the immutable corpus, historical snapshots, decks,
  completion facts, and Anki provenance.
- A learner can find an unflagged occurrence by exact observed form, inspect
  context, correct it without a dictionary entry, and explicitly select any
  additional occurrences. The review is navigable by keyboard, readable at
  narrow widths, and usable without JavaScript.

## Non-goals

- An LLM automatically changing the canonical lemma during deck generation,
  or a deck-time change being backported into already-frozen identities.
- A new administrator role, global correction rule, POS correction, general
  concordance or full-text search, or a new global-frequency import.
- Rewriting shared normalized corpus data, completed reading history, existing
  Known facts, prepared artifacts, or an imported Anki collection.
- Automatically transferring occurrence corrections to a new analysis or
  treating dictionary presence or a suggestion as proof of correctness.

See [ADR 0081](../adr/0081-learner-owned-occurrence-lemma-corrections.md) for
the ownership and freeze-boundary decision.
