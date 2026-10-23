# Faust I poetry support: automated release evidence and cutover

Prepared: 2026-10-23 · Status: **automated evidence complete — human release
validation pending** · Ticket: [#2199](https://github.com/justin-hayes/mouseion/issues/2199)

This document closes the automatable part of the
[Basic poetry support](../features/basic-poetry-support.md) acceptance matrix and
links the [ADR 0103](../adr/0103-source-structure-and-poetry-evidence.md)
single-version cutover. It is release evidence, not feature acceptance. The real
NLP, catalog, provider, browser-manual and Anki runs live in
[#2200](https://github.com/justin-hayes/mouseion/issues/2200), and no deployment,
migration, backfill or production action is authorized here.

The contract remains the feature document and ADR 0103, as accepted at
`b0b817b2411457678a76936cfbdf9b652bdce889` (landed by
[#2188](https://github.com/justin-hayes/mouseion/pull/2188)). This document
sequences evidence and instructions only; it decides nothing.

## Evidence identities

| Item | Identity |
| --- | --- |
| Application revision | `9e59f5c3a83f9861225f3235d39372e130ffcd90` (tip commit; squash merge of #2266) |
| Contract | [Basic poetry support](../features/basic-poetry-support.md) and [ADR 0103](../adr/0103-source-structure-and-poetry-evidence.md) at `b0b817b` |
| Acceptance input | `https://www.gutenberg.org/ebooks/21000.epub.noimages`, 179,103 bytes, SHA-256 `93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01`; never committed |
| Committed fixture | `internal/epub/testfixtures/faust-excerpt` (short excerpts, no Gutenberg header, licence or references); `faust-excerpt.provenance.json` binds the acceptance digest and package prefix `OEBPS/8980964947130898904_21000-h-` |
| Committed NLP baseline | Locked `nlp/uv.lock`: Python 3.13.5, Stanza 1.15.0, torch `2.14.1+cpu` (not exercised by the automated gates below) |
| Go toolchain | go1.27.3 linux/amd64 |
| Generated-code tools | protoc 29.3, templ v0.3.1070, sqlc v1.31.1 |
| Lint | golangci-lint 2.14.0, ruff and mypy from the locked environment |
| Browser automation | Node 20.19.2, Playwright 1.63.0 (Chromium and WebKit projects) |

The fixture provenance test proves the committed excerpts are listed and free of
Gutenberg text, and that the recorded digest and package prefix match the
acceptance input. It does not compare excerpt bytes with a fetched EPUB: the
full work is deliberately absent from the repository.

## Gate results

All commands below were run on 2026-10-23 against the application revision above.

| Gate | Command | Result |
| --- | --- | --- |
| Go lint | `make lint-go` (inside `make lint`) | Pass: golangci-lint 2.14.0, 0 issues |
| Python lint and types | `make lint` | Pass: ruff check, ruff format check, mypy |
| Build | `make build` | Pass |
| Go unit tests | `make test` | Pass |
| Python tests | `make test` | Pass: 148 passed, 5 skipped (pinned-model gates that CI does not provision) |
| Integration | `make test-integration` | Pass, PostgreSQL-backed |
| Browser smoke | `make browser-smoke` | Pass: 634 passed, 2 skipped (desktop-only layout assertions in compact projects); Chromium full suite and WebKit native-controls matrix |
| Generated artifacts | `make gen && make templ && make sqlc`, then `git diff --exit-code` | Clean: no diff |
| New-code coverage | `make coverage-new` | Pass: the change adds no code lines, so SonarQube Cloud's 80% new-code condition does not apply |

No skip or failure is hidden in these results. The five Python skips are the
existing pinned-model release gates (French golden and model-dependent corpora),
not poetry acceptance; a skip is not reported as a run.

## Acceptance matrix status

Status values for the **Automated** column:

- **Automated** — the required result is established by an automated test that
  needs no external service.
- **Automated (fixture)** — covered by a supplied analyzer, committed fixture,
  fixture server or APKG inspection.
- **Automated (real PostgreSQL)** — covered against a real PostgreSQL database
  in the integration suite.

The **Human run still required** column names the release claim that automation
cannot establish. Those runs belong to
[#2200](https://github.com/justin-hayes/mouseion/issues/2200).

| Acceptance row | Automated evidence | Automated | Human run still required |
| --- | --- | --- | --- |
| Acquisition | `internal/epub` integration `TestImportForAcquisitionWritesV2SnapshotAndRetainsBytes` (unchanged bytes and digest, v2 snapshot identity and convention set, whole-snapshot main-text fallback); `TestImportMainTextFixturePreservesAllDeclaredUnits`; unit `TestFaustExcerptFixtureProvenance` | Automated (fixture) | Catalog/OPDS acquisition of the checksum-pinned EPUB; a digest mismatch fails that setup |
| Cross-line sentence (Director, `0:90–93`) | `internal/webapp` `TestFaustExcerptEndToEndRegression` subtests “Director cross-line sentence keeps exact text and offsets” and “study context carries exact source lines and attribution”; `internal/epub` `TestExtractV2FaustExcerpts`; `internal/enrichment` `TestFaustDirectorProviderRequestCarriesExactNewlineContext` | Automated (fixture) | Real NLP; real provider |
| Analyzer-owned boundaries (`0:195`) | `TestFaustExcerptEndToEndRegression` subtest “analyzer-owned boundaries may share one source line”; `TestSentenceStudyRowsSplitAtAuthorialBreaks` | Automated (fixture) | Real NLP reconfirmation |
| Group boundaries (Thule, `3:141` onward) | `TestFaustExcerptEndToEndRegression` subtest “Thule keeps six four-line groups and no sentence crosses a group”; `TestExtractV2FaustExcerpts`; `TestExtractXHTMLV2BreakAndGroupContract`; `TestSentenceStudyRowsSplitAtAuthorialBreaks` | Automated (fixture) | Real NLP |
| Visible text and Unicode (`5:46–47`) | `TestExtractXHTMLV2UnicodeAndVisibleText`; `TestFaustExcerptEndToEndRegression` subtest “correction markup keeps visible text only”; `TestConcordanceSentencePartsUseCodePointOffsets`; provider-payload field and forbidden-marker assertions | Automated | — |
| Role precedence (Valentin, `4:579`) | `internal/selection` `TestSpeakerLabelSpansAreStructuralNonVocabulary` and `TestSpeakerLabelSpansLeaveTheCoverageDenominator`; `internal/persistence` `TestSpeakerLabelsAreStructuralNonVocabularyAcrossCountConsumers`; `internal/webapp` `TestPoetrySpeakerLabelRefusesStudyAndLabelsConcordance`; end-to-end selection and Concordance subtests | Automated | — |
| Standalone direction (`Er schlägt das Buch auf …`) | `TestFaustExcerptEndToEndRegression` selection/study subtests; `TestConcordanceContextShowsStageDirectionAttribution`; `internal/cardexport` `TestAttributionRendersTheSpeakerNameOrStageDirection`; Playwright `poetry-presentation.spec.ts` | Automated | — |
| Mixed-role paragraph (`2:346`) | `TestFaustExcerptEndToEndRegression` selection and study subtests; `TestProjectWithholdsOnlyStudyContextOverlappingSpeakerLabels`; `TestConcordanceWithholdsStudyForSpeakerLabelSentences`; `internal/prepareddeck` `TestPreparedDeckProjectionWithholdsSpeakerLabelSentenceReferences`; Playwright `poetry-presentation.spec.ts` | Automated | — |
| Attribution | `internal/domain` `TestSnapshotStructureAttributionAt` (nearest label, heading reset, direction does not clear, unknown convention, nested direction, terminal period, invalid spans) and `TestSpeakerDisplayNameRejectsInvalidLabel`; `internal/epub` `TestExtractXHTMLV2RoleConventions` (unknown and contradictory markup establish no role); `internal/persistence` `TestSentenceStudyAttributionShowsOnlyTheSpeakerName`; `internal/cardexport` `TestApplySourceAttributionIgnoresMissingOrMismatchedEvidence` | Automated | — |
| Deliberate omissions (`4:368`, `4:580`, `0:599–602`) | `internal/sentencequality` `TestDeliberatePoetryOmissionsFailHonestly`; `TestFaustExcerptEndToEndRegression` quality-omission and APKG-absence assertions | Automated | Real-NLP omission observations |
| List/page noise | `TestMultilineSourceLineIsNotStructuralNoise`, `TestBlankSourceLinesAreNotListOrPageMarkers`, `TestListAndPageMarkersRejectOnAnySourceLine`, `TestNoBreakVerdictsMatchTheV1Policy` | Automated | — |
| Vocabulary and corrections | `TestSpeakerLabelsAreStructuralNonVocabularyAcrossCountConsumers` (projection, Browse, coverage, Reading freeze agree); `TestV1CountsAndFrozenReadingStayPinnedAcrossSpeakerAwareRebuild`; `TestProjectAppliesExactOccurrenceDecisionsWithoutChangingAnalyzerEvidence` | Automated (real PostgreSQL) | Correcting measured historical forms on a real instance |
| Web surfaces | `e2e/tests/poetry-presentation.spec.ts` in all four Chromium viewport/theme projects (375px and 1200px, light and dark, 200% text, no-JavaScript, no horizontal overflow, native disclosure, keyboard focus and the named text alternative); `internal/webapp` `TestConcordanceSummaryChunksMarkSourceLineBreaks`, `TestSentenceStudyPageRendersSourceLinesAndPerLineMorphology`, `TestSentenceStudyLineEndOrdinalsNoteOnlyLinesThatContinue`, `TestSentenceStudyTextAlternativeIsClosedByDefaultAndComplete`, whole-line and local fallback tests | Automated (fixture) | Manual browser pass; any screen-reader check actually performed |
| Provider/translation | `TestFaustDirectorProviderRequestCarriesExactNewlineContext` (exact payload field set, LF-bearing sentence, no attribution/title/orientation); `TestFaustDirectorProviderResponseKeepsValidatedAlignment` (full prose, contiguous/discontinuous alignment and unhighlighted fallback); end-to-end provider payload assertion | Automated (fixture) | Representative real provider meanings, translations and alignment |
| Anki | `internal/cardexport` `TestAttributionFieldIsAppendedAfterExistingFields`, `TestSourceLineTextEscapesSegmentsAndKeepsEmphasisWithinLines`, `TestAttributionDoesNotChangeNoteOrModelIdentity`, `TestLegacyManifestRendersProseWithoutAttribution`; end-to-end APKG field inspection (18 fields, `Attribution` appended) | Automated (fixture) | Real Anki import and re-import in a recorded client version |
| Compatibility | `internal/sourcecheck` `TestSourceStructureCheckUnchangedProseIsNoOp`, `…DetectsChangedV1Text`, `…MissingBytesIsOriginalUnavailable`, `…ExcludesImportedAnalysis`, `…RoleOnlyChangeIsUpdateAvailable`, `…FailureRetryAndExactKeyReuse`, `…RejectsMismatchedExtractor`, `…ObsoleteEvidenceIsNotRecorded`; `internal/analysis` `TestSourceStructureUpdateAdoptionSwitchesSnapshotAndConvergesOnOneRun` (double submit), `…RejectsConcurrentSourceChange`, `…RollsBackBeforeCommit`, `…RoleOnlyChangeKeepsRevision`; `internal/persistence` `TestAdoptExtractedSnapshotCreatesReusesAndGuards`; `internal/webapp` `TestReadingChooserOffersAConfirmedSourceStructureUpdate`, `TestSourceStructureUpdateConfirmationSubmitsTheCheckedOffer`, `TestSourceStructureUpdateConfirmationReportsRefusals` | Automated (real PostgreSQL) | Adoption on a real instance through the UI |
| Frozen artifacts | `TestV1CountsAndFrozenReadingStayPinnedAcrossSpeakerAwareRebuild`; `TestSourceStructureUpdateAdoptionPinsActiveReadingEvidence`; `TestPresentationOnlyRebuildKeepsFrozenAttributionWithoutEvidenceCalls`; `TestAttributionDoesNotChangeNoteOrModelIdentity` | Automated (real PostgreSQL) | Adopting a v1 Book with an active Reading on a real instance |
| Prose regressions | Full gates above; `TestExtractV2MatchesV1ForNoBreakProse`, `TestExtractXHTMLV2MatchesV1WithoutBreaks`, `TestNoBreakVerdictsMatchTheV1Policy`, `TestConcordanceNoBreakProseKeepsTheShippedLayout`, `TestSentenceStudyPageLeavesNoBreakProseLayoutUnchanged`, `TestLegacyManifestRendersProseWithoutAttribution` | Automated | — |

## What the automated evidence does not prove

- The end-to-end regression supplies its own analyzer output. It is not a real
  Stanza run and cannot report analyzer accuracy, card yield or omission rates.
- Committed excerpts are short and provenance-bound, not the checksum-pinned
  EPUB acquired through the catalog. The catalog route, the German capability,
  the unchanged downloaded bytes and the digest gate remain human checks.
- Provider tests use fixed requests and responses. They prove the payload shape
  and alignment validation, not representative provider meanings or
  translations.
- Anki evidence comes from reading `collection.anki2` out of the APKG. It is not
  an Anki import, re-import, duplicate check, field-shift check or a rendering
  review in a released client.
- Playwright drives `cmd/fixtureserver`, which binds no PostgreSQL, River, NLP,
  OPDS or other network service. Role/name assertions are accessibility-tree
  evidence, not manual screen-reader verification.
- No production instance, deployment, migration run, backfill or secret is
  touched by this evidence.

## Cutover

The single-version cutover for migration `000045_source_structure_v2` is
documented as the [single-version schema and evidence
cutover](../operations.md#single-version-schema-and-evidence-cutover) in the
operations guide: stop the old application and workers, let the new version
apply the additive DDL before it serves or writes, run one version only, and
adopt existing Books only through explicit learner confirmation. Those
instructions are prepared for human review and are not deployment permission.

## Draft shipped-state updates (not applied)

Apply these only after #2200 records passing real runs. They are drafted here so
the shipped state changes in one reviewed step, not before validation; this
document does not claim them as present state.

1. `doc/features/basic-poetry-support.md` — replace the status line with
   `Status: **Accepted specification — implemented and validated** · Date: 2026-10-17`,
   and replace the introduction that says acceptance is not an implemented
   feature and implementation “has not been executed” with a sentence recording
   the implementation tickets and the human validation run.
2. `doc/adr/0103-source-structure-and-poetry-evidence.md` — replace
   `Status: **Accepted — implementation pending**` with
   `Status: **Accepted — implemented**`.
3. `doc/product.md` — change the ADR 0103 entry from “**accepted;
   implementation pending**” to “**accepted; implemented**”, drop “no
   application change or rollout has been performed”, and point the feature
   link at the recorded release evidence.
4. `doc/README.md` — replace “accepted, implementation-pending source-line/role
   evidence” with “implemented source-line/role evidence”.
5. `doc/design/README.md` — replace “accepted, implementation-pending
   source-line, attribution and explicit source-update contract” with the
   implemented wording.
6. `doc/design/components.md` — change the accepted source-line presentation
   target status from “**Accepted — implementation pending.**” to
   “**Accepted — implemented.**”.

## Handover to the human run

[#2200](https://github.com/justin-hayes/mouseion/issues/2200) owns every row
whose **Human run still required** column is not empty. If such a run fails,
file a follow-up rather than weakening the matrix or reporting a fixture result
as the real run.
