# Rights and provenance notice (#2048)

This notice records what each source declares. It is not legal advice, and it does not resolve the licence of this repository's own research artifacts. That is a human decision.

## Sources

| Layer | Source | Declared licence | Notes |
|---|---|---|---|
| Native annotation (AGDT v2.1, v2.0, v1.6) | `PerseusDL/treebank_data` @ `bf4334f0…` | Root README: contents are CC BY-SA 3.0 US. Material to which third parties hold copyright is excluded from the Perseus/Tufts copyright statement. | The root declaration covers the annotation files. No file-level licence was separately verified here. Third-party qualifications for individual files were not reviewed. |
| Canonical running text (grc2) | `PerseusDL/canonical-greekLit` @ `01b725d8…` | `license.md`: CC BY-SA 4.0. | The file-level TEI licence was read in an earlier step and was not re-verified in the final run. |
| Guideline text (for search only) | AGDT2 guidelines (markdown 2.0, 2.5); GSAAGDT 1.1 | As declared in those documents. | Used only for keyword search. Not reproduced. |

## What is committed

- Hashes, code-point spans, ASCII keys (native sentence and node IDs; `cg2:wNNNN`), classes, reasons and counts.
- Scripts that read the pinned inputs by URL and hash. The scripts do not contain Greek text.
- Greek surfaces are **not** committed. Per-occurrence surfaces are written only to a scratch `detail/` directory.

## Share-alike and derivatives

Committing manifests derived from CC BY-SA sources raises the question of whether the derived artifacts are themselves subject to share-alike terms. This report does not decide that. The question is recorded as open for a human reviewer, together with whether the repository's own licence should apply.

## Unknowns recorded explicitly

- Applicability of the root CC BY-SA 3.0 US declaration to each layer and release.
- Whether any AGDT file carries a third-party qualification.
- The repository licence that applies to these research outputs.
