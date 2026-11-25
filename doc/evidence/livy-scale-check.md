# Livy scale check

This is the scale check for Livy's *Ab urbe condita*
([#2687](https://github.com/justin-hayes/mouseion/issues/2687), spec
[#2674](https://github.com/justin-hayes/mouseion/issues/2674)). ADR 0128 makes
the whole Work one Book, and ADR 0132 admits Livy only behind this check. It
holds figures, method and gate results only. No source or translation text
appears here.

Run on 2026-11-25 at `ticket-2687@493111f6`.

## Result

**Livy is not admitted.** Reading and Vocabulary Browse fail their bounds:
the Reading page takes 3.5 s for each request, against a bound of 1 s.
Deck preparation's peak memory passed the 2 GiB bound in one run (2,035 MiB)
and failed it in the other (2,200 MiB), so it is not reliably within it.
Gate (iii) also fails on the committed reader. Livy is absent from the
manifest, and ADR 0128 is not amended. A follow-up issue recording the
figures and the blocking surface has been drafted for Justin to review; it is
not filed yet.

## Criteria

These criteria were fixed before anything was measured. The comparator is
the largest Greek Book, Plato's *Republic* as one Book (`plato-republic`,
89,148 words). Livy (`livy-ab-urbe-condita`, 518,255 words) is 5.81 times
its size.

A surface passes when every one of its criteria holds:

1. **Classical preparation.** Preparation covers the add, the published
   analysis, and the stored translation segments. It must complete without
   failure and finish in 10 minutes or less. Peak RSS of the server process
   must be 2 GiB or less. Time per word must be no more than twice the
   Republic's.
2. **Reading.** First paint is the full server response for `/la/reading`,
   with the Book as the Current reading and Browse page 1 included. It must
   return 200. The cold request must take 3 s or less. The warm median must
   be 1 s or less, and the warm maximum 2 s or less. **Navigation** has the
   same bounds. It covers Browse page 2, the last Browse page, a prefix
   filter, the sentence study page at the first and the last sentence, a
   Concordance lemma lookup, and My Books.
3. **Vocabulary Browse.** Browse is part of the Reading page, so criterion 2
   covers it. In addition, Browse counts must be available within the
   preparation bound.
4. **Deck preparation.** The deck must reach Ready with no provider call
   and finish in 10 minutes or less. Peak RSS must be 2 GiB or less. Time per
   card must be no more than twice the Republic's.

The absolute bounds are usability bounds:

- At 1 s, a page stops feeling immediate.
- 8 s is the server's interactive read timeout; past it, the server answers
  504.
- The host has 31 GiB, shared with PostgreSQL and the NLP service.

Preparation and deck building are background jobs, so they may take minutes.

## Method

The test ran on a throwaway stack: a Testcontainers PostgreSQL, with the web
handler and River workers in process. The preparation, translation and
prepared-deck workers were wired as `cmd/server` wires them, with no NLP
service and no translation provider. The web handler was given the services
the batch-1 deployment check used (auth, store, analysis, prepared deck,
Classical catalog). It was not given `AnalysisInsights`, `SourceStructures`,
`EPUBAcquisition`, `ReadingIntent` or `OPDS`, which `cmd/server` passes, so
the Reading page skipped the coverage work `AnalysisInsights` adds in
production. The page times below are therefore lower bounds. The dictionary index
was the L&S index derived for batch 1. The harness was a scratch integration
test kept outside the repository, and none of the running deployment's
containers was used. Each Work ran in its own process. The test went through
these steps:

1. It added the Work through the Catalog's add route.
2. It waited until the analysis was published and translation segments
   were stored.
3. It started the Book as the Current reading.
4. It waited until Browse counts were available.
5. It requested each page once cold and then five times warm.
6. It prepared the reading deck through the deck retry route.

Peak RSS is the process's VmHWM. It was reset after a forced GC, before
preparation and again before the deck, so each figure belongs to one phase.

The upstream bytes were served from local copies of the pinned files, with
HTTP Range honoured, so the times exclude the network. The local copies were
the sparse OLA archive (filled by Range from Zenodo, with CRC-32 checked),
`canonical-latinLit@8cef5162` and the OGA and `canonical-greekLit` pins.
Preparation fetched 19,352,905 bytes for Livy and 12,928,041 bytes for the
Republic.

To rerun it, build a scratch manifest that holds Livy's Definition (OLA
members above, `perseus-eng2`, a placeholder spot check) with `make
classical-manifest` and the sparse archive as upstream. Then run an
integration test in `internal/webapp` that follows the deployment check's
wiring with a transport serving the upstream directory by file name, once
per Work, with `go test -count=1 -tags=integration -run <test> -timeout 90m`.
The test is reproducible from the steps above; the scratch harness itself
was not committed.

The machine had 16 CPUs and 31 GiB of RAM. The scratch manifest held Livy with
`perseus-eng2` and a measurement-only spot check. It was built with the
empty-section reader workaround described under "Generator gates"; that
workaround was not committed.

## Figures

The Livy figures are from the first run. A second run reproduced them within
about 5%, except for the deck's peak RSS, shown as both runs' values.

| Surface | Republic (89,148 words) | Livy (518,255 words) | Ratio | Result |
|---|---|---|---|---|
| Preparation: published | 6.0 s | 26.7 s | 4.5× | |
| Preparation: translation stored | 6.1 s | 28.6 s (28.4 s) | 4.7× | pass. Per word, 0.81× the Republic's |
| Preparation: peak RSS | 484 MiB | 1,436 MiB (1,457 MiB) | 3.0× | pass |
| Stored | 6,361 sentences, 105,452 tokens, 278 segments, 67 MB | 29,938 sentences, 607,336 tokens, 20,136 segments, 270 MB | | |
| Browse counts available | 8.4 s after add, 3,178 identities | 38.3 s after add, 9,492 identities | 4.6× | pass |
| Reading, first paint (cold / warm median / warm max) | 770 / 770 / 836 ms | 3,499 / 3,522 / 3,552 ms | 4.6× | **fail** |
| Browse page 2 | 775 / 751 / 781 ms | 3,488 / 3,522 / 3,610 ms | 4.7× | **fail** |
| Browse last page | 777 / 744 / 785 ms | 3,530 / 3,508 / 3,551 ms | 4.7× | **fail** |
| Browse prefix filter | 734 / 750 / 758 ms | 3,503 / 3,507 / 3,538 ms | 4.7× | **fail** |
| Browse, all identities | 751 / 766 / 798 ms | 3,520 / 3,502 / 3,530 ms | 4.6× | **fail** |
| Sentence study, first sentence | 108 / 109 / 112 ms | 864 / 862 / 879 ms | 7.9× | pass |
| Sentence study, last sentence | 279 / 284 / 287 ms | 287 / 279 / 282 ms | 1.0× | pass |
| Concordance lemma lookup | 372 / 354 / 370 ms | 1,245 / 1,252 / 1,258 ms | 3.5× | **fail** |
| My Books (supplemental) | 175 / 161 / 164 ms | 700 / 695 / 719 ms | 4.3× | within the bound |
| Deck preparation | 7.9 s, 1,895 cards, 8.1 MB | 29.5 s, 7,117 cards, 25.9 MB | 3.7× | pass. Per card, 0.99× the Republic's |
| Deck: peak RSS | 616 MiB | 2,035 MiB (pass); 2,200 MiB in the second run (fail) | 3.3× | **fail**: over 2 GiB in one of two runs |

Supplemental, outside the preset navigation list (My Books, above, is also
supplemental):

| Page | Republic | Livy | Ratio |
|---|---|---|---|
| Reading after the deck is ready | 1,438 / 1,365 / 1,427 ms | 5,555 / 5,558 / 5,606 ms | 4.1× |

The Concordance lookup used the lemmas *sum* and εἰμί. The prefix filter
used `con` and `πρ`. Every page returned 200, and no deck preparation called
a provider.

The time sits in the Reading page's Book-scoped reads. In the second Livy
run, the store calls behind the page took these times:

- `ListVocabularyBrowsePage` took 0.84–0.88 s.
- `GetCurrentReading` took 0.64–0.69 s.

The rest of the 3.5 s (5.8 s once a deck exists) is other Book-scoped work
on the same request. Every Browse request pays it, because Browse is served
on the Reading page. The Republic itself is close to the 1 s bound and
passes it only before a deck is prepared.

## Generator gates

Livy's edition is OLA v0.2.0 `phi0914.phi001.perseus-lat2`, Weissenborn and
Mueller, Teubner 1884–1911. Its two members were fetched by Range:

- the CoNLL-U, 37,609,764 bytes, SHA-256 `4a354a08…0776c`;
- the Citation layer, 39,124,563 bytes, SHA-256 `9ef6a7bf…42419`.

The CRC-32 of each member matched its directory. The Work runs from 1.pr.1
to 45.44.21 over 35 constituent books; the named `pr` sections parse with
the widened Citation. Three translations exist at the
`canonical-latinLit@8cef5162` pin.

Gate results with the committed generator. The split-word limit is 2%, the
join floor 98%, and the dictionary floor 90%:

| Gate | Result |
|---|---|
| (i) Citations | pass: 607,336 tokens, all cited, none backwards |
| (ii) Split words | pass: 2,083 of 518,255 words, 0.402% |
| (iii) Join | **FAIL** for every translation: the reader refuses each file |
| (iv) Dictionary | pass: 309,847 of 343,025 eligible occurrences, 90.33% |

The generator run that measured all three candidates took 39 s of wall time
and peaked at 675 MiB RSS.

Each translation fails gate (iii) for its own reason:

- **`perseus-eng2`** is Spillan, Edmonds and McDevitte, Bohn 1849–68. It has
  one empty section (44.32.8), and the reader refuses the file with
  "cited section has no text".
- **`perseus-eng3`** is Roberts, Dent 1912. It has one empty section
  (3.30.1), refused the same way.
- **`perseus-eng1`** is the Loeb of Foster, Moore, Sage and Schlesinger,
  1919–59. It carries 25 summary books, such as `11s` and `31s`, which have
  no sections, and the reader refuses them with "textpart has no cited
  section". It also has six empty sections.

**D6. Empty translation section.** If the reader skipped an empty section,
leaving its sentences unjoined, both of the first two translations would
clear gate (iii)'s mechanical checks, the join floor and the tier check.
Admission would still need the spot check that ADR 0133 requires Justin to
record; the scratch run used a placeholder. These figures come from a scratch
build that was not committed:

- `perseus-eng2`: join 29,690 of 29,938 sentences (99.17%), tier A.
- `perseus-eng3`: join 29,651 of 29,938 sentences (99.04%), tier A.

This is a reader change that no decision covers, so it was not made here.
It matters only if Livy's scale blockers are fixed.

No spot check is recorded for Livy.
