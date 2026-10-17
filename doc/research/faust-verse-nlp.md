# Faust I verse boundaries: Stanza segmentation and sentence-based study

**Research date:** 2026-10-17

**Base commit:** `6e02d180` (`docs(context): tighten prepared-deck omission definitions (#2137)`)

**Scope:** What the pinned German Stanza pipeline, the project's current
text-to-analysis path, and a public German edition of Goethe's *Faust, Der
Tragödie erster Teil* establish about verse. Covers line breaks, paragraph and
sentence segmentation, enjambment, offsets and chunking, speaker and stage
directions, elision, separable verbs, and multi-word tokens, and tests these
against the current prose-oriented quality gate. It recommends no design. The
study-policy and structure decisions belong to their owning tickets.

**Status:** Findings only. No ADR, feature doc, or application code changed.
No Stanza model was run (see [Limits](#limits-and-unrun-probes)).

## Bottom line

1. **The pinned pipeline never gives Stanza a verse line's sentence context.**
   The EPUB extractor turns every `<br>` and every block element into a
   separate block, and joins blocks with a blank line. Stanza's tokenizer
   splits on blank lines into paragraphs, and its decoder ends the current
   sentence at each paragraph boundary, whatever the punctuation. A
   verse-line break that reaches Stanza as a blank line is therefore a
   hard sentence boundary. This is verified from the pinned source, not
   executed.
2. **The current quality gate rejects most verse-line sentences.** In the
   Gutenberg text of *Faust I*, 2,683 of 5,924 non-blank lines (45%) do not end
   in `.`, `!`, or `?`, and 65 start with a lowercase letter. The gate requires
   a terminal `.`, `!`, or `?` and an uppercase or quoted start. Those rules
   are constraints for this study, not something to relax silently.
3. **Offsets are consistent across chunks only while the text is unchanged.**
   Stanza offsets are code-point indices into the text it was given. Go
   chunking cuts contiguous substrings and re-bases offsets by code-point
   count. Any transformation between extraction and analysis that changes
   length (the current `cleanSpace` does) breaks the mapping to the printed
   source unless that mapping is stored.
4. **A single newline is whitespace to Stanza.** Stanza replaces every
   whitespace character one-for-one, and only blank-line runs create paragraph
   breaks. A verse line separated by a single newline would therefore not force
   a sentence break. The current extractor never produces single newlines, so
   this would require a change outside this research.
5. **The configured German pipeline omits `mwt`, which Stanza's German default
   includes.** Contractions such as *zum*, *im*, and *am* occur in *Faust I*
   hundreds of times. Whether the pinned German models split them without the
   MWT processor is untested.

## Sources

### Primary, external

| Source | Use | Pin / retrieval |
| --- | --- | --- |
| [Stanza 1.15.0 wheel](https://files.pythonhosted.org/packages/df/bc/78a8f73009619cf1bcbd2657c1966a64990ab9f78a59de785c0cf0f2923c/stanza-1.15.0-py3-none-any.whl) | Tokenizer, decoder, and dataset source (`stanza/models/tokenization/data.py`, `utils.py`, `tokenize_processor.py`) | sha256 `1da8a072de5ff82a9776016d87e1c09fe08b35336e5f8071b79dc324aece0c7a`, retrieved 2026-10-17 |
| [Stanza resources manifest 1.15.0](https://raw.githubusercontent.com/stanfordnlp/stanza-resources/main/resources_1.15.0.json) | Per-language default processors and packages | sha256 `ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463`, retrieved 2026-10-17 |
| [Stanza: Tokenization & Sentence Segmentation](https://stanfordnlp.github.io/stanza/tokenize.html) | Documented options: `tokenize_no_ssplit`, `tokenize_pretokenized` | Web docs, retrieved 2026-10-17 |
| [Stanza: Data Objects and Annotations](https://stanfordnlp.github.io/stanza/data_objects.html) | Documented `start_char`/`end_char` on `Token`; `Word` property list | Web docs, retrieved 2026-10-17 |
| [Project Gutenberg eBook #2229, *Faust: Der Tragödie erster Teil*](https://www.gutenberg.org/cache/epub/2229/pg2229.txt) | Exact German samples; line numbers below refer to the CR-stripped copy | sha256 of the downloaded raw file `fed2edd58edd6910ee96d27ada614d34bcc41f03750d86541c41246bb74c564ff7`, release 2000-06-01, header "most recently updated June 11, 2026", credits Michael Pullen. The source edition behind this transcription is not named in the header. |

### Pinned project evidence (base commit `6e02d180`)

| Path | Evidence |
| --- | --- |
| `nlp/pyproject.toml:11`, `nlp/uv.lock:486` | `stanza==1.15.0` |
| `nlp/src/mouseion_nlp/model_config.py:10` | `DEFAULT_PROCESSORS = "tokenize,pos,lemma,depparse"` (no `mwt`) |
| `nlp/src/mouseion_nlp/model_config.py:113-130` | German uses the default processor set and no package override; Greek adds `mwt` explicitly |
| `nlp/src/mouseion_nlp/producer.py:126-145` | `stanza.Pipeline` built with `processors`, `download_method=NONE`, and no tokenizer options (`tokenize_no_ssplit`, `tokenize_pretokenized`, `speaker_delim` are unset) |
| `nlp/src/mouseion_nlp/producer.py:220-251` | Whole chunk text passed to one pipeline call; offsets copied as code-point offsets |
| `nlp/src/mouseion_nlp/producer.py:253-313` | Token offsets fall back to the token when a word lacks `start_char`; sentence offsets derive from its first and last token |
| `nlp/src/mouseion_nlp/producer.py:324-340` | Separable verbs reattach only through `compound:prt` within the same Stanza sentence, using a closed prefix list |
| `internal/epub/epub.go:527` | Unit text: `strings.Join(extractor.blocks, "\n\n")` |
| `internal/epub/epub.go:300` | Units joined by `"\n\n"` in the spine text |
| `internal/epub/epub.go:582-605` | `flush`; `isBlock` includes `br`; `cleanSpace` collapses all whitespace, including line breaks, to single spaces |
| `internal/analysis/chunk.go:7,34-67` | `DefaultMaxChunkChars = 100_000`; chunks are contiguous and cut after `.`, `!`, `?`, with a hard cut for over-long sentences |
| `internal/analysis/jobs.go:1343-1352` | Per-chunk analysis; each chunk's offsets advance by its rune count |
| `internal/analysis/jobs.go:1951` | `offsetResultLocations` adds the chunk offset to sentence and token locations |
| `internal/cardexport/cardexport.go` (`scoreBoundaryQuality`, `hasCompleteBoundary`, `containsRejection`) | Boundary knock-out: terminal `.`, `!`, or `?`, and uppercase, digit, or quoted start (non-Greek) |
| `internal/gdex/gdex.go` (`hasFiniteRootWithSubject`, `isFiniteVerb`) | Finite-root-with-subject knock-out (ADR 0062) |
| `doc/linguistics.md` (§2 table) | Says German uses the "Stanza default package" |
| `doc/adr/0013-*`, `doc/adr/0062-*`, `doc/features/sentence-quality-scoring.md` | Chunking, the finite-verb-and-subject knock-out, and the unchanged length, boundary, target, and location gates |

## Evidence classes

- **Documented:** stated on a Stanza documentation page (cited above).
- **Source-verified:** read from the pinned 1.15.0 wheel, not executed.
- **Project-observed:** read from this repository at the base commit.
- **Measured on sample text:** counted from the Gutenberg text with a
  regex heuristic. These describe the input, not the NLP output.
- **Untested risk:** not confirmed by any run.

### Stanza behavior

| Claim | Class | Evidence |
| --- | --- | --- |
| Tokenization and sentence segmentation run in one tokenize processor; `tokenize_no_ssplit` treats each blank-line-separated chunk as a sentence and disables sentence splitting | Documented | [tokenize docs](https://stanfordnlp.github.io/stanza/tokenize.html) |
| Token `start_char`/`end_char` are indices into the raw document text; documented `Word` properties do not list `start_char` | Documented | [data objects](https://stanfordnlp.github.io/stanza/data_objects.html) |
| Blank-line runs (`\n` plus optional whitespace plus `\n`) split input into paragraphs | Source-verified | `stanza/models/tokenization/data.py:193` (`NEWLINE_WHITESPACE_RE`), `:221` |
| Each paragraph is tokenized separately, and its leftover tokens are flushed as a sentence at the paragraph end, so a sentence cannot cross a blank-line boundary | Source-verified | `stanza/models/tokenization/utils.py` `decode_predictions` (lines ~568-646): the paragraph loop ends with `if len(current_sent): doc.append(...)`; sentence breaks inside a paragraph occur only on `p == 2 or p == 4` when `no_ssplit` is false |
| Whitespace, including single newlines and C1 controls, is replaced one character for one character, so character counts and offsets are preserved | Source-verified | `data.py:~234-235` (`WHITESPACE_RE.sub(' ', char)`); `utils.py` `decode_predictions` (`text = WHITESPACE_RE.sub(' ', orig_text)`) |
| Offsets are searched in the whitespace-normalized copy of the original text and recorded against the original text | Source-verified | `utils.py` `decode_predictions`, `text.index(part, char_offset)` |
| `speaker_delim` is an optional tokenizer feature: it rejoins speaker-tagged segments with `\n\n` and rebases text; it is off by default and unset in this project | Source-verified; off by default | `tokenize_processor.py` (`speaker_delim` parse; `_process_speaker_tagged_text`, `joined_text = '\n\n'.join(...)`) |
| German default processors are `tokenize: combined_nocharlm`, `mwt: combined`, `lemma: combined_nocharlm`, `pos: combined_charlm`, `depparse: combined_charlm`, `ner: germeval2014` | Documented in the resources manifest | `resources_1.15.0.json`, `de.default_processors` |
| Without the `mwt` processor, German contractions stay as single tokens | Untested risk, inferred from the Stanza docs | [tokenize docs](https://stanfordnlp.github.io/stanza/tokenize.html) (the tokenizer predicts MWTs; the MWT processor expands them) |

### Project behavior

| Claim | Class | Evidence |
| --- | --- | --- |
| Stanza receives text as blank-line-joined blocks. A `<br>` or any block element starts a new paragraph, so verse lines marked up as `<br/>` or one-`<p>`-per-line each become a paragraph | Project-observed (code) | `epub.go:527,582,600,605`; the markup of a specific edition is not checked |
| Within a block, `cleanSpace` collapses all whitespace (including carriage returns and single newlines) to single spaces, so the change in length is not recorded | Project-observed (code) | `epub.go:605` |
| German is analyzed with the default processor set and no `mwt`, while `linguistics.md` describes German as the "Stanza default package" | Project-observed; documentation/config mismatch | `model_config.py:10,114`; `doc/linguistics.md` §2 |
| Chunks are contiguous, so concatenating them reproduces the unit text, and chunk offsets are rune-based and additive | Project-observed (code) | `chunk.go:34-67`, `jobs.go:1343-1352,1951` |
| Gutenberg *Faust I* main text is about 186,000 characters, so chunking at 100,000 characters yields about 2 chunks | Measured on sample text | body joined with LF, non-blank lines only; the real EPUB text will differ |

## Faust I sample structure (measured)

Counted from the Gutenberg text, main text only (from *Zueignung* to the end
marker, first body heading onward). Counts come from a regex script
(`measure_faust.py`) that treats blank-line-separated runs as blocks and uses
the regexes noted below. Treat them as approximate.

| Measure | Count | Note |
| --- | ---: | --- |
| Non-blank lines | 5,924 | |
| Blank-line-separated blocks | 1,079 | speeches, stanzas, stage directions |
| Blocks with more than one line | 1,019 | |
| Lines ending in `.`, `!`, or `?` | 3,241 | |
| Lines not ending in `.`, `!`, or `?` | 2,683 | includes commas, semicolons, colons, dashes, closing brackets, and bare words |
| Lines starting lowercase | 65 | continuation lines |
| Block-initial all-caps speaker labels (regex `^[A-ZÄÖÜ][A-ZÄÖÜ ]{1,}\.?$`) | ~795 | approximate |
| Stage-direction-only lines (whole line in parentheses) | 98 | |
| Typographic apostrophe U+2019 | 308 | used for elision (`müssen’s`, `hab’ ich`) |
| ASCII apostrophe `'` | 0 | |
| Lines containing an elision apostrophe | 276 | |
| Longest run without `.`, `!`, or `?` | 416 characters (63 words) | well below the chunk size |
| Contractions `zum` / `im` / `am` / `vom` / `ins` (whole-word) | 42 / 100 / 51 / 26 / 28 | `im` is usually *in dem*; not all uses are contractions |

Two measures decide the outcome. Verse-line breaks are sentence boundaries
for the producer unless the representation changes, and about half of all line
ends are not sentence ends. Nearly every such line ending in a comma or
semicolon would fail the gate's terminal-punctuation rule. Lowercase starts
would fail its uppercase rule where they begin a sentence.

## Exact Faust I examples

Line numbers refer to `pg2229.txt` with CRLF removed (same numbering).

| Feature | Lines | Text (as in the source) |
| --- | --- | --- |
| Sentence spanning four verse lines, no internal full stop | 530–533 | `Habe nun, ach! Philosophie,` / `Juristerei und Medizin,` / `Und leider auch Theologie` / `Durchaus studiert, mit heißem Bemühn.` |
| Lowercase line start after a comma (stage direction) | 526–527 | `In einem hochgewölbten, engen gotischen Zimmer Faust,` / `unruhig auf seinem Sessel am Pulte.` (stage direction) |
| Speaker label followed directly by speech | 529–530 | `FAUST.` / `Habe nun, ach! Philosophie,` |
| Enjambed clause ending in a comma, then a line with elision | 836–837 | `Ich bitt Euch, Freund, es ist tief in der Nacht,` / `Wir müssen’s diesmal unterbrechen.` |
| Elision forms | 843–844 | `Erlaubt mir ein’ und andre Frage.` / `Mit Eifer hab’ ich mich der Studien beflissen;` |
| Parenthetical exit and aside | 846, 848 | `(Ab.)` / `FAUST (allein).` |
| Stanza of four lines with one terminal mark | 849–852 | `Wie nur dem Kopf nicht alle Hoffnung schwindet,` … `Und froh ist, wenn er Regenwürmer findet!` |
| Multi-line stage direction inside a block, with speaker turn after | 738–741 | `(Wagner im Schlafrock und der Nachtmütze, eine Lampe in der Hand.` / `Faust wendet sich unwillig.)` / blank / `WAGNER.` |
| Three-line sentence across verse lines ending in `?` | 1010–1012 | `Ihr Chöre, singt ihr schon den tröstlichen Gesang,` / `Der einst, um Grabes Nacht, von Engelslippen klang,` / `Gewißheit einem neuen Bunde?` |
| Separable-particle candidate on one line (`los`), untested | 858 | `Du rissest mich von der Verzweiflung los,` |

The sample uses single newlines between verse lines and blank lines between
stanzas and speeches. Whether the adopted EPUB preserves that distinction is
unknown.

## Quality-gate constraints (not relaxed here)

From ADR 0062 and the code:

- **Finite verb and subject knock-out (`ScoreSentenceQuality`):** the sentence
  root must be a finite `AUX`/`VERB` with an `nsubj`, `nsubj:pass`, or `csubj`
  dependent. Verse fragments without a finite root (for example `FAUST.`,
  `Doktoren, Magister, Schreiber und Pfaffen;`) are rejected.
- **Boundary knock-out (`hasCompleteBoundary`):** the sentence text must end in
  `.`, `!`, or `?`, and start with an uppercase letter, a digit, or a quote.
- **Length and structure:** the "too short or fragmented" and "too long"
  rejections, plus the "structural noise" rule, all apply to the sentence text.
- **Target:** the target lemma must appear as a whole word in the sentence.

Predicted effect, not measured: under the current paragraph-per-line
representation, most verse-line sentences would fail the boundary and
finite-root rules, so few lines would be eligible as study sentences. A
multi-line sentence in the source would be split into several Stanza
sentences, none of which may be eligible. These are predictions from the code
and sample counts. No Stanza run confirmed them.

## Pipeline issues to resolve before any design

1. **Paragraph rule.** A verse line rendered as a blank-line-separated block
   is a hard sentence boundary for Stanza. Any representation that keeps lines
   this way inherits the boundary.
2. **Whitespace normalization.** `cleanSpace` changes length: it drops carriage
   returns, collapses runs of spaces, and removes source line breaks. Offsets
   are recorded against the normalized unit text. A source-aligned view must
   either map back to that text or record its own offsets. Changing
   normalization changes the analysis identity and offsets for every existing
   book (see ADR 0059 and ADR 0060).
3. **Chunk cuts.** The chunker cuts after every `.`, `!`, or `?`, including
   abbreviations, so a closing quote or bracket after the mark starts the next
   chunk. A cut can fall inside a Stanza sentence wherever Stanza's segmentation
   disagrees with the chunker's punctuation rule, and the chunker never checks
   for that. The hard cut for an over-long sentence is not relevant to
   *Faust I*, whose longest run without terminal punctuation is 416 characters.
4. **Speaker labels.** `FAUST.` on its own line becomes a one-token paragraph,
   which Stanza emits as a fragment sentence with no finite root. Speaker names
   and stage directions are therefore separate Stanza sentences under the
   current representation. The optional `speaker_delim` feature is unused and
   would rebase offsets; it is not a candidate without a separate check.
5. **Elision apostrophes.** The sample uses U+2019 throughout. The producer's
   `_clean_surface` keeps `'` and `’` at word edges, so elided surfaces such as
   `müssen’s` are preserved. How the pinned German tokenizer splits these
   forms is untested.
6. **Separable verbs.** Reattachment works only within one Stanza sentence and
   only for particles that carry `compound:prt` on the verb's head. Under a
   per-line paragraph representation, a verb and particle on different lines
   cannot reattach. I did not find a cross-line instance by inspection, so this
   is a predicted consequence of the paragraph rule, not a measured one.
7. **Multi-word tokens.** German has no `mwt` in the project's processor
   string, while Stanza's German default includes it. Stanza's MWT processor
   expands tokens the tokenizer marks as multi-word; without it, tokens like
   *zum*, *im*, and *am* remain as the contracted surface. Whether the producer
   then lemmatizes them correctly is untested, and the Stanza document
   description suggests they would not be expanded.
8. **Documentation mismatch.** `doc/linguistics.md` describes German as the
   "Stanza default package". The default includes `mwt`, and the code omits
   it. Executable configuration is authoritative, so the mismatch should be
   fixed in the owning ticket if it matters.

## Options (not accepted design)

These are options for the human-owned study-policy and structure tickets. They
are not recommendations to adopt.

- **A. Keep the current per-line paragraphs.** Simplest; offsets stay
  consistent with the normalized unit text. The verse-line boundary and the
  gate issues above remain.
- **B. Single newline within a stanza, blank line between stanzas.** Stanza
  replaces the single newline with a space one character at a time, so a
  sentence can continue across verse lines and offsets are preserved. This
  requires the EPUB extractor to keep single newlines, which changes the unit
  text, its offsets, and its analysis identity. It also puts newlines into
  `Sentence.text`, so display and quality checks must handle them.
- **C. Mouseion-owned sentence grouping with `tokenize_no_ssplit`.** Group lines
  into the sentences the study should use, join groups with `\n\n`, and run
  Stanza with `tokenize_no_ssplit=True`, which makes each blank-line group one
  sentence. Stanza then segments only within a group. The grouping rule
  decides every sentence boundary, so it needs its own evaluation.
  `tokenize_no_ssplit` changes behavior for every language that uses it, so it
  would need to be configured per source or per language.
- **D. Prose analysis plus a source-line alignment.** Keep Stanza input as
  prose-like text, and store line and stanza ordinals as a separate structure
  that maps Stanza's character offsets to source lines. Study sentences and
  display use the mapping. Nothing about whitespace changes, but the mapping
  must be computed and persisted, and it must stay consistent with the
  normalized unit text.

Any option must keep the offset basis (code points in the normalized unit
text) or record a versioned offset map, and must not bypass the gate's rules.
The final acceptance cases must be drawn from the adopted EPUB edition, not
from this Gutenberg transcription.

## Limits and unrun probes

- **No Stanza model was run.** `STANZA_RESOURCES_DIR` is unset, and no
  provisioned German models exist under `~/stanza_resources` or
  `/opt/stanza_resources`. The pinned German models are large, and downloading
  them was out of bounds, so no tokenization, segmentation, parse, or
  accuracy results exist. Nothing here measures NLP quality.
- **Source reading only.** The sentence-break and whitespace behavior comes from
  reading the pinned 1.15.0 wheel. I did not execute `decode_predictions` or the
  producer.
- **No environment changes.** No dependency, lockfile, or generated file
  changed. Stanza 1.14 exists in other worktrees, but I did not use it, because
  it is not the pinned version.
- **The sample is not the adopted edition.** The Gutenberg header does not name
  its source edition. Verse layout, stanza breaks, speaker markup, and
  apostrophe forms may differ in the adopted EPUB.
- **Counts use regex heuristics** and are reported as approximate where marked.
- **Documented versus source claims.** The Stanza docs state the blank-line
  rule only for `tokenize_no_ssplit` ("sentences are split by two continuous
  newlines"). That the default mode also cannot cross a blank line is taken
  from the decoder source.

## Reproducible evaluation plan (not run)

1. **Fix the edition.** Choose the adopted German EPUB through the existing
   catalog route, and record its sha256 and extracted unit text, including the
   `\n\n` layout produced by `epub.go`.
2. **Provision the pinned models, outside this repository.** Use
   `python -m mouseion_nlp.provision` with `STANZA_RESOURCES_DIR` set to a
   scratch directory under `~/.cache/scratch/`. This downloads large models, so
   it needs approval.
3. **Segmentation probe.** Tokenize-only (`processors="tokenize"`, default
   package) over three inputs: (a) current per-line paragraphs; (b) the same
   lines joined with single newlines; (c) Mouseion-grouped sentences with
   `tokenize_no_ssplit=True`. Compare sentence counts and boundaries against a
   small hand-marked gold set drawn from the adopted edition.
4. **Offset invariant.** For every token, assert `text[start:end]` equals the
   token surface, and check that chunked and unchunked analyses give identical
   absolute offsets.
5. **Full-pipeline probe.** Run `tokenize,pos,lemma,depparse` on the same
   inputs. Record how elision apostrophes, `zum`/`im`/`am`, and the separable
   particles in the Faust samples are tokenized and lemmatized, with and
   without `mwt`.
6. **Gate simulation.** Apply `ScoreSentenceQuality` and `hasCompleteBoundary`
   to the produced sentences and count rejections by reason. Report counts. Do
   not report accuracy unless a gold set labels the sentences.

## Open questions for owning tickets

- Whether verse lines should be sentence units, sentence groups, or a separate
  structure (options A to D).
- Whether the `mwt` omission for German is intended, and what the
  documentation should say.
- Whether the adopted EPUB keeps verse lines as `<br/>`, one `<p>` per line,
  or a single-newline layout that the extractor currently collapses.
