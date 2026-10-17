# Book 1 sense prototype — first scored run

**All 24 occurrences (48 dictionary/occurrence labels) live-confirmed and frozen
in [`sense-gold.json`](sense-gold.json) before the first `evaluate.py` run. No
strategy, lexicon or abstention policy has been accepted yet.**

For [Prototype: word sense in context for Ancient Greek without an LLM](https://github.com/justin-hayes/mouseion/issues/2130).
Do not read the alignment review as sense labels.

## Measured results (initial frozen run, [`results.json`](results.json))

| Lexicon | Strategy | Correct / chosen | Abstain | Correct yield |
|---|---|---|---|---|
| Kaikki | morphology-order | 13/19 (68%) | 5/24 | 13/24 |
| Kaikki | projection | 6/9 (67%) | 15/24 | 6/24 |
| Kaikki | unique-projection | 4/5 (80%) | 19/24 | 4/24 |
| Kaikki | no-contextual-gloss | 0/0 | 24/24 | 0/24 |
| LSJ | morphology-order | 15/22 (68%) | 2/24 | 15/24 |
| LSJ | projection | 13/15 (87%) | 9/24 | 13/24 |
| LSJ | unique-projection | **7/7** | 17/24 | 7/24 |
| LSJ | no-contextual-gloss | 0/0 | 24/24 | 0/24 |

Representative failures:

- **Source order picks the first sense.** Morphology-order errs on γίγνονται
  (“come into being” / LSJ umbrella, not “be born”), ἐτύγχανε, παρήγγειλε
  (“transmit a message”), ἀπορία, and for LSJ φόβος (“panic flight”), ἑλεῖν
  (“take with the hand”) and κερδαλεώτερον (“crafty, cunning”); Kaikki φυλακάς
  becomes “detention; custody”.
- **English overlap is lured by translator wording.** Brownson's “designs upon”
  pulls ἐπιβουλεύοντος to LSJ “form designs upon, aim at” and Kaikki “plan to do
  something”; ἀπορία goes to “difficulty of passing” for both lexica.
- **Kaikki coverage gaps:** five occurrences have no identity-safe, gloss-bearing,
  compatible candidate (lookup misses for ἐγκέλευστος, κερδαλέος, πειθαρχέω,
  the πάρειμι identity gate, and ὤνια's adjective/noun POS mismatch). LSJ has two.
- **Unique gate is conservative:** LSJ unique-projection abstains on 8 targets
  because several senses overlap, and on 7 with no overlap at all.

Interpretation, bounded by the limits below: LSJ beats Kaikki under every
strategy on this sample. No strategy available **without** human alignment
(morphology-order) is accurate enough for unattended contextual glosses: about
one chosen gloss in three is wrong. Projection's gains depend on approved
human links; the earlier prototype rejected automatic alignment, so these
projection figures are an oracle upper bound, not a deployable result. 7/7 is
too small to imply a calibrated precision (a 95% Wilson interval still reaches
down to about 65%).

## Concrete artifact and evidence

- [`evidence.html`](evidence.html): single offline file, review controls plus
  guided/free-play baseline replay, full source candidates and fixed scores.
- [`inputs.json`](inputs.json): 24 selected target occurrences in the shared
  eight-sentence sample, 24 retained Kaikki source records and 23 LSJ source
  entries. These entry counts are **not coverage or accuracy rates**: POS,
  homograph identity, sense granularity and absent candidates are separate.
- [`alignment-gold.json`](alignment-gold.json): unchanged bytes from `6455853e`;
  confirmed Greek/English extents and target links only.
- [`predict.py`](predict.py) and [`predictions.json`](predictions.json): fixed
  non-LLM baselines, with source and code digests. No sense labels are read.
- [`review-proposals.json`](review-proposals.json): original agent proposals.
  [`review-confirmed.json`](review-confirmed.json) records all 24 confirmed
  occurrences; [`review-status.json`](review-status.json) preserves the exact
  live exchange in four batches. Justin explicitly accepted four Kaikki evidence
  judgments: adjective plus comparative morphology for “older”, “take, grasp,
  seize” for catching the ship, adjectival “for sale” for wares, and “gain,
  profit” plus the monthly context for pay.
- [`evaluate.py`](evaluate.py): scores only a complete confirmed label set;
  [`sense-gold.json`](sense-gold.json) froze it before the first run, which
  wrote [`results.json`](results.json). No algorithm changed after scoring.
- [`README.md`](README.md) and [`NOTICE.md`](NOTICE.md): pins, exact comparison
  contract, reproduction commands, source rights and limitations.

The fixed methods compare morphology/labels/source order, English lexical
projection, a unique-overlap abstention gate, and evidence-only/no contextual
Gloss. Projection receives the already-approved English links. It cannot rescue
or validate the automatic alignment methods rejected by the separate prototype.

## Representation observations, not measured semantic verdicts

1. **Parent meanings must survive extraction.** Kaikki's “give orders” can occur
   only as the breadcrumb of narrower children (“recommend, exhort” / “summon”).
   Extracting just the first gloss merges children; extracting just the last
   discards the parent. This prototype preserves exact prefix nodes with their
  pinned raw-row/depth provenance, without claiming universal native sense IDs.
   Before scoring, second-batch source review also exposed repeated “depart”
   glosses with different construction notes. The original `03207830` prefix
   grouping was corrected: each raw row/depth now stays separate. The first six
   confirmed IDs/meanings remain unchanged, no scores had been computed, and no
   improvement on held-out data is claimed. A unique-path gate may consequently
   abstain on repetitions; repeated paths are not asserted to be different senses.
2. **Equal spelling is not a homograph crosswalk.** The sample's πάρειμι has two
   verbal entries in each lexicon. Automatic methods retain the identity gate
   rather than merging “be present” with “pass by”. A human reference sense may
   identify the contextual use without authorizing an OGA-number crosswalk.
3. **LSJ grammar mentions are not blanket constraints.** One sense may discuss
   multiple voices or comparative examples. They are preserved for review, not
   treated as exclusive morphology requirements merely because a tag occurs.
4. **Source order and contextual wording differ.** In the first six occurrences,
   birth versus generic becoming, children as offspring versus servants, the
   comparative older versus elderly, and participial coincidence versus hitting
   a target are useful human-review distinctions; the scored run confirms that
   source order misses most of them.

No exact-match source-key candidate was retrieved for φυλακή from this bounded
LSJ extraction; this is a retrieval observation, **not a claim that the complete
LSJ lacks the word or its garrison meaning**. Unknown key notation is excluded
and disclosed, not silently stripped. All retained LSJ entries parsed without
unresolved named entities. Cross-lexicon coverage must not conflate extraction
gaps with lexicographical or contextual-sense gaps.

## Checks completed before human review

- All three research input hashes and the complete 68,196-row derived Kaikki
  subset hash match. The Go helper checks Unicode 17.0.0 and uses the real
  `ancient-greek/1` profile, not Python casefolding.
- Reused alignment bytes match the frozen commit; all proposed source-path IDs
  resolve to the correct target headword in the exact retained source inventory.
- The evaluation command refuses to score the missing sense gold as intended.
- Predictions and the offline demo regenerate byte-identically. Chromium inspection
  covered all 192 occurrence/dictionary/strategy combinations, four guided
  walkthroughs and the missing-alignment gate, draft export and memory-only review;
  no JavaScript errors or horizontal overflow at 390px were observed.
- Go full-tree lint passed (0 issues); `go test ./...` passed. Explicit Ruff
  check/format checks pass for the prototype scripts. Full `make lint` stopped
  after Go lint because this worktree lacks `.venv/bin/ruff`; `make coverage-new`
  stopped at the missing Python/pytest preflight. No production code changed.

The purposive, proposal-biased sample cannot establish Book 1 general accuracy,
an independently validated WSD system or a calibrated confidence threshold.
Human acceptance of a native sense is also distinct from acceptance of its long
dictionary wording as a short contextual card Gloss.
