# OGA Anabasis Book 1 import proof: acquisition-gate stop

## Scope amendment — same task reopened

Justin subsequently authorized downloading the official archive for private
inspection. The [live scope amendment](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6178890938)
reopens the same bounded proof and overrides the earlier no-archive limit only.
The official download is being attempted with size/published-MD5 verification
and local SHA-256 recording; **completion is not yet established**. No access
bypass, source redistribution, new corpus, analysis beyond Book 1, provider call,
source/MVP acceptance, PR or merge is authorized by this change. The individual
accompanying upstream work XML has now been acquired and its exact Git blob and
SHA-256 verified ([new input registry](ancient-greek-oga-anabasis1-import-proof/unit-inputs.json));
its correspondence to OGA remains unestablished.

The previous gate evidence at commit
`4104c8be238fe0d4b5500f8e18522e80dd516467` remains unchanged and independently
reproducible. The following sections and the original `gate.py`, `evidence.json`
and `ledger.json` describe that **historical initial assessment**, not the newly
authorized acquisition's current verdict. The
[rights notice](ancient-greek-oga-anabasis1-import-proof/NOTICE.md) records the
pre-acquisition assessment and separate operation boundaries.

During the download, the documentary subagent mistakenly deleted the partial
file. The main session retained the original open inode; local recovery will
restore and checksum those same response bytes when the request finishes,
without starting another download. Acquisition remains pending. The new
`acquire_archive.py` has **10 invented-byte safety tests** (27 together with the
17 historical metadata tests); these do not prove archive completion or unit
coverage.

An independently inspected upstream-revision detail: Book 1 in this release has
10 chapter elements and 205 section elements, with one explicit `choice` at
**1.3.20** (`sic` / `corr`). These structure counts are not OGA coverage proof.
The retained GLAUx review set's `text_error_propagated` row instead targets
**1.3.14**, so that particular illustrative target must not be treated as a
verified witness to the correction at 1.3.20. The general annotated-revision
qualification remains; no GLAUx artifact is changed or remediation made a
prerequisite to this OGA attempt. Exact OGA correspondence is still pending.

## Historical initial gate result

**Result: no complete-unit import was attempted.** The examined official download
inventory supplies one whole-corpus archive, not an individual Anabasis package.
That route is explicitly prohibited by
[Prove a complete OGA Anabasis Book 1 import into Mouseion vocabulary workflows](https://github.com/justin-hayes/mouseion/issues/2052).
No permitted individual-work package was established, so the task stops at Gate 1.
This is a bounded negative acquisition result, **not evidence that OGA lacks
Anabasis or is linguistically unusable**, and not source/MVP acceptance.

Companion [registry](ancient-greek-oga-anabasis1-import-proof/inputs.json),
[documentary evidence](ancient-greek-oga-anabasis1-import-proof/evidence.json),
[verdict ledger](ancient-greek-oga-anabasis1-import-proof/ledger.json),
[operation-specific notice](ancient-greek-oga-anabasis1-import-proof/NOTICE.md), and
[reproducer/tests](ancient-greek-oga-anabasis1-import-proof/gate.py).

## Exact availability and acquisition assessment

Assessment date: **2026-10-14**. OGA documentation is pinned to
**`a69d917dbe117a307829977dd556e22f05ce0715`**, also the observed `main` HEAD.
This pins documentation and the example, **not a full-unit release payload**.

1. [OGA README, lines 11–41][oga] names the source releases, links the corpus
   download to Zenodo, and separately advertises hosted ANNIS queries and a
   desktop corpus. Its [example README][example] directs users to Zenodo too.
   The [complete, non-truncated Git tree][tree] has no Anabasis payload path;
   its annotation example is `ggm0001.ggm001.1st1K-grc1`, not Xenophon. No
   example was substituted for Book 1.
2. [Zenodo record 14206061][zenodo] lists exactly one file:

   | Field | Published value |
   | --- | --- |
   | Release | OGA `0.2.0`, DOI `10.5281/zenodo.14206061`, 2024-11-24 |
   | File | `opera_graeca_adnotata_v0.2.0.zip` |
   | File UUID | `96c1ae43-3193-4386-94ec-4f62263b1cae` |
   | Size | **8,629,032,478 bytes** (8.63 GB decimal) |
   | Published checksum | `md5:d4117ae52f1cc6319c6ba20af0c1cf3b` |
   | Scope | Whole-corpus archive; **not acquired** |

   These are inventory declarations, not an inspected archive-member manifest.
   No archive SHA-256, member path, unit size or complete Book 1 coverage is
   invented. The archive content endpoint is deliberately absent from the
   reproducer's request allowlist. No HEAD, GET, range/stream extraction, mirror
   or alternative transport request to that file was made.
3. [OGA's bibliographic list, lines 5562–5570][bibliography] has one target
   record: `tlg0032.tlg006.perseus-grc2`, Xenophon, *Anabasis*, Marchant,
   *Xenophontis Opera omnia Volume III Expeditio Cyri*. This establishes an
   advertised **work/edition lead**, not Book 1 endpoints, interior coverage,
   original text correspondence or annotation availability in an inspected unit.
4. The original-text lead is canonical-greekLit release `0.0.11376425141`.
   Its [tag reference][tagref] identifies annotated **tag object**
   `90ac67b935af3b46d6f910c1e173ffd99c657d78`; [peeling that object][tag] gives
   **commit** `476fb6133498e5710de7eec0b492791da86be4b4` (not the tag object).
   The [work-directory listing][listing] advertises
   `data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`, **886,347 bytes**,
   Git blob `a220f4b8accd1448a0189a595493a78aa09c707b`.
   The [CTS metadata][cts] agrees on the edition identifier and bibliography.
   **The Greek work file was not fetched.** Its internal structure, full extent,
   SHA-256 and relationship to OGA remain unverified. An individual original
   text without the accompanying OGA annotation does not satisfy this task.
5. The [official website][site] sends downloads to the same Zenodo record.
   Its [ANNIS page][annis] documents querying/visualization, not an advertised,
   revision-pinned, source-preserving individual Anabasis package. No hosted
   corpus query/export, endpoint probing or reconstruction by paginating matches
   was performed. This does **not** establish that ANNIS can never export such
   data, or that every query is a prohibited survey. A suitable supported unit
   export remains **unestablished**, not silently treated as archive access.

**Gate decision:** no demonstrated acquisition route satisfies the existing
individual-unit/work-package boundary. The observed archive route is out of
scope regardless of download feasibility. No package was acquired while deciding
its permission, and no Gate 2 work followed the stop. The bounded inventory is
not a global claim that no other file or future upstream export exists.

## Component rights and the actual decision needed

OGA [README lines 162–170][oga] explicitly declares **CC BY-SA 4.0** and refers to
the original-text repositories. The declaration expressly accompanies a README
that identifies the corpus/release. Zenodo's `other-at` shorthand is recorded
without treating it as a competing prohibition, a missing license or a veto.
Likewise, the upstream [README][upreadme] explicitly licenses repository contents
BY-SA 4.0 unless otherwise indicated, while retaining Tufts's general copyright
and varying-component notices. Neither notice automatically cancels that grant
or makes contacting anyone a prerequisite.

Those declarations support this documentary assessment; they do not establish
which exact uninspected text/header/annotation components a full-unit package
would contain or their verified derivation. **Exact package/component clearance
is unperformed, not a finding that private computation is prohibited.** No
alternative license was guessed, and no legal or Greek-classification question
is delegated to Justin. Local inspection, supported acquisition, public source
redistribution, exports/bundling and external-provider operations have separate
dispositions in [NOTICE.md](ancient-greek-oga-anabasis1-import-proof/NOTICE.md),
following [Choose the licensing and distribution boundary for classical data](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026).

The concrete scope decision returned to Justin is whether to **leave this OGA
proof deferred under the current no-archive boundary**. Continuing is not
authorized on present evidence. It would require an identified permitted unit
package (with its component review), or a separately explicit change of scope.
This task does **not** offer an archive retry as an executable next step, create
an access/survey prerequisite chain, contact anyone, or choose another corpus,
edition or Reading unit. The live source/usefulness decision remains
[Choose the smallest useful annotation-backed Ancient Greek slice](https://github.com/justin-hayes/mouseion/issues/2030).

## Unperformed layers and comparison

| Layer | This OGA task |
| --- | --- |
| Acquisition | **Stopped**: no demonstrated in-scope individual package |
| Exact full unit / edition | Bibliographic lead only; completeness and exact edition unverified |
| Binding / clean reading-text projection | Not performed |
| Required lemma / UPOS / discriminators / headwords | Not performed |
| Complete sentences / verified targets | Not performed |
| Native morphology / syntax / optional capabilities | Not assessed on Book 1 |
| Real Go selection / independent model / Known comparison | Not run |
| Counts / 3–15 selection / selected occurrence coverage | **Unmeasured, not zero** |
| Deterministic sentence inputs / cap distribution / omissions | **Unmeasured, not zero** |
| Practical examples / prediction quality | No Book 1 examples acquired or judged |
| Dictionary / Gloss / translation / Anki / client rendering | Not performed |

Consequently there are no Book 1 node, selection, sentence, card-input or omission
manifests masquerading as successful empty results. The ledger uses `null` for
those measurements. No Greek service/analyzer/model, provider call, dictionary
acquisition, card construction or learner-state change occurred.

The retained [OGA official-example format probe][stage1] and the retained
[GLAUx complete-unit proof][glaux] are unchanged. This task does not redo their
tests or promote their conclusions beyond the independent parent verification:

| Comparison axis | OGA evidence | GLAUx evidence |
| --- | --- | --- |
| Acquisition / maintenance | Documentary pin and linked sample work; no demonstrated complete-unit package under this task's limits | Individually addressable pinned annotation and text files; real full-unit reproduction exists |
| Coherent complete text/analysis | Stage 1 shows linked-format feasibility only; no Book 1 binding | Parent checked all 10,645 native-node correspondences, 10 chapters/205 sections and full source-span coverage |
| Required repair / practical quality | Full-unit requirements and prediction quality unassessed, not presumed better or worse | 63 unannotated lexical words in editorial deletions; editorial-sign presentation, headword/POS quality, a boundary case and the annotated 2019 text's 1.3.14 typo remain qualifications |
| Vocabulary / dry inputs | Unmeasured | Proposed interpretation: 1,294 identities, 373 selected, 372 deterministic inputs; **not accepted vocabulary or a ready deck** |
| Rights | Explicit declarations preserved; unacquired exact components unassessed | Exact-file declarations and prior operation-specific assessment exist, not blanket source/export approval |

In particular, GLAUx's blanket εἰμί→AUX rule affects 180 occurrences. The cited
UD evidence has **1,864 AUX and two VERB** εἰμί occurrences; a supported
rationale/refinement is still needed. Neither all 180 exclusions nor all 180
being errors follows mechanically. No such rule is copied into OGA.
Edition-aware output comparison is impossible without OGA's exact acquired text.
Different future counts would not by themselves establish errors or superiority;
neither GLAUx nor human annotation is assumed always correct. Parent
[comparison/qualifications](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6178553143)
remain controlling. No candidate or whole-Anabasis readiness is accepted here.

## Reproduce and independently inspect the stop

Python **3.13 / Unicode 15.1.0**, standard library only. One command, repo root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 doc/research/ancient-greek-oga-anabasis1-import-proof/gate.py --fetch
```

This fetches **only 13 allowlisted documentation/inventory inputs**, validates
their hashes, derives the evidence/ledger, compares both **byte-for-byte**, and
runs **17 focused tests**. It never follows response URLs, XML references or
redirects, never retries automatically, and cannot fetch archive/text/annotation
payloads through its allowlist. `--fetch` means documentary retrieval, **not
Gate 2 acquisition**. Without that flag it is offline. `--cache PATH` selects
retained inputs; `--write` explicitly regenerates the two diagnostic files.

The registry records original response byte sizes/SHA-256. API JSON also has a
canonical-JSON SHA-256 over **every parsed field**, because GitHub returns
equivalent JSON with different whitespace/content negotiation. Evidence hashes
declare that distinction. Raw documentary originals are unchanged in Git-ignored
`ancient-greek-oga-anabasis1-import-proof/private/inputs/`; no full-unit source
or normalized reading text is there. The full bibliographic inventory is not
republished: only the target record is projected publicly. Published checksums
of unacquired payloads are clearly distinguished from locally verified hashes.

API/website responses are dated snapshots, not immutable upstream objects.
Changed metadata must fail verification; it must not silently refresh this
assessment. Independent reviewers can inspect the cited inventories and the
public target-specific evidence even if raw response snapshots later change.
The Git tree and raw repository files use the declared revisions. No global
inventory size is used as Book 1 coverage evidence.

Validation on 2026-10-14: retained-input and fresh-documentary retrieval both
reproduce exact diagnostics and pass all 17 tests. Initial raw-byte-only API
verification rejected equivalent JSON formatting; canonical whole-JSON checking
was added and tests cover both serialization equivalence and changed values.
Mutation/failure tests additionally reject wrong/incomplete tree revisions, new
unreviewed target packages, missing/duplicate/wrong work or edition references,
changed release inventories, tag/commit confusion, missing declarations,
non-allowlisted payload URLs, corrupt inputs and redirects. They are **metadata
gate tests**, not span/mapping/sentence validation or claims of corpus correctness.

`git diff --check` passes. Whole-project `make lint` stops because
`golangci-lint` 2.14.0 is missing; `make coverage-new` stops because the project
`.venv`/pytest environment is absent. No full-tree lint or application coverage
success is claimed for this standalone research artifact. No production code,
schema, dependencies, ADR, glossary, earlier research branch, PR or merge changed.

[oga]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/README.md
[example]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/README.md
[tree]: https://api.github.com/repos/OperaGraecaAdnotata/OGA/git/trees/a69d917dbe117a307829977dd556e22f05ce0715?recursive=1
[bibliography]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/urn_cts/texts/urn_cts_plus_date_label.xml#L5562-L5570
[zenodo]: https://zenodo.org/api/records/14206061
[site]: https://varro.informatik.uni-leipzig.de/oga/en/index.html
[annis]: https://varro.informatik.uni-leipzig.de/oga/en/annis.html
[tagref]: https://api.github.com/repos/PerseusDL/canonical-greekLit/git/refs/tags/0.0.11376425141
[tag]: https://api.github.com/repos/PerseusDL/canonical-greekLit/git/tags/90ac67b935af3b46d6f910c1e173ffd99c657d78
[listing]: https://api.github.com/repos/PerseusDL/canonical-greekLit/contents/data/tlg0032/tlg006?ref=0.0.11376425141
[cts]: https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/data/tlg0032/tlg006/__cts__.xml
[upreadme]: https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/README.md
[stage1]: https://github.com/justin-hayes/mouseion/blob/f0034fea98458491ce24413c8a24717c41b3cfd8/doc/research/ancient-greek-oga-format-probe.md
[glaux]: https://github.com/justin-hayes/mouseion/blob/957c99f25de9ea16e139055e6a87bbcda1996cc9/doc/research/ancient-greek-glaux-anabasis1-import-proof.md
