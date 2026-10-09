# Linguistic Design Notes

Mouseion treats each book a learner wants to read as a small annotated corpus.
These notes describe the linguistic decisions behind that corpus: what counts
as the text, what counts as a word, how coverage is measured, how vocabulary
and example sentences are chosen, where dictionary and model evidence come
from, and how the results are evaluated. Each section links to the feature
specification and the architecture decision record (ADR) that govern it. For the
software architecture, see [ARCHITECTURE.md](../ARCHITECTURE.md).

## 1. The text

**Unit of analysis.** An EPUB is extracted in spine order into readable units
(chapters or sections), each keeping its title, declared landmarks, and
character offsets into the source. Every token later produced by analysis
points back to an exact span in a specific unit of a specific content revision,
so any statistic or example sentence can be traced to its place in the book
([spec](features/epub-analysis-scope.md)).

**Main text versus paratext.** Front matter, bibliographies, indexes,
glossaries, and colophons inflate a book's vocabulary without being part of
the work being read. Mouseion selects the main text from the EPUB 3 landmarks
a publisher declares: from the unit marked `bodymatter` up to the first
back-matter unit. Notes, endnotes, and appendices are deliberately kept,
because learners often read them. Selection is fail-safe: if no unit or more
than one unit is marked as body matter, or the main-text run would be empty,
the whole book is analyzed. Boundaries are judged at whole-document
granularity, so the selector can keep some ancillary text but never drops a
later body document. The
selection algorithm is versioned in the analysis identity, so a change produces
a new, distinguishable analysis rather than silently altering an old one
([spec](features/main-text-selection.md),
[ADR 0066](adr/0066-main-text-selection-from-epub-structure.md)).

## 2. Annotation

Text is annotated by [Stanza](https://stanfordnlp.github.io/stanza/) 1.14 into
[Universal Dependencies](https://universaldependencies.org/) categories:
tokenization, multi-word-token expansion where the language needs it, universal
part-of-speech (UPOS) tags, morphological features, lemmas, and basic
dependency relations.

| Language | Pipeline |
| --- | --- |
| German (`de`) | Stanza default package: tokenize, POS, lemma, dependency parse |
| Italian (`it`) | Stanza default package: tokenize, POS, lemma, dependency parse |
| Modern Greek (`el`) | Greek Dependency Treebank (GDT) models with multi-word-token expansion, and the [Greek-BERT](https://huggingface.co/nlpaueb/bert-base-greek-uncased-v1) dependency parser |

The Greek choice followed a survey of available Modern Greek tools
([research](research/modern-greek-nlp.md),
[ADR 0073](adr/0073-modern-greek-language-support.md)). Multi-word-token
expansion matters for Greek because contractions such as *στο* and *στην*
combine a preposition with an article (*σε* + *το*); the expanded words are
counted separately, so neither inflates the other's frequency.

The full annotation is persisted: surface form, the analyzer's raw lemma, the
canonical lemma, UPOS, morphological features, dependency relation and head,
and source offsets. Everything downstream (coverage, selection, the
concordance, sentence scoring, and learner review) reads this stored corpus
instead of re-running the models
([ADR 0059](adr/0059-persisted-normalized-corpus-for-concordance.md),
[ADR 0060](adr/0060-persist-dependency-parses.md)).

## 3. What counts as a word

A vocabulary item is identified by **(language, canonical lemma, UPOS)**
([ADR 0005](adr/0005-vocabulary-identity-normalization-ranking.md)). The same
spelling with a different part of speech is a different item. The canonical
lemma comes from a versioned, per-language normalization profile applied to
the analyzer's lemma; the raw lemma is kept alongside it.

**German.** The profile `german-standard-post-1996` lowercases lemmas and
applies a small, reviewed table of equivalences from documented pre-1996
spellings to their reformed forms (*daß* → *dass*, *Fluß* → *Fluss*,
*Schloß* → *Schloss*). The authority is the official *Regelwerk* of the Rat für
deutsche Rechtschreibung. The table is deliberately not a general rule: modern
*ß* after long vowels (*Straße*) and distinct lexemes (*Maße* vs. *Masse*)
must not collapse, and each new mapping needs a fixture, a source citation, and
a profile-version review. One JSON file feeds both the Go runtime and the
Python dictionary-index builder, and a shared parity fixture keeps them
identical ([ADR 0065](adr/0065-german-pre-1996-sharp-s-canonicalization.md)).

**German separable verbs.** In *Er fängt morgen mit der Arbeit an*, the
lemmatizer returns *fangen* for *fängt* and treats *an* as a separate token.
Counted that way, the learner would study *fangen* ("catch") instead of
*anfangen* ("begin"). The NLP producer reattaches a particle to its verb when
the dependency parse marks it `compound:prt` *and* its surface form is in a
closed list of 49 separable prefixes. The relation alone isn't enough,
because it doesn't reliably separate particles from free adverbs and
homographs. The verb's canonical lemma becomes the full lexeme (*anfangen*),
and the particle is excluded from candidates
([spec](features/separable-verb-lemmatization.md),
[ADR 0061](adr/0061-german-separable-verb-lemmatization.md)).

**Modern Greek.** The `modern-greek` profile applies Unicode NFC normalization
and full case folding, which also unifies final *ς* with medial *σ*. It is
deliberately **accent-sensitive**: the tonos distinguishes words in Greek
(*πότε* "when?" vs. *ποτέ* "never"), so stripping accents would merge distinct
lexemes.

**Italian** and other languages use Unicode NFC normalization with case
folding.

**Learner corrections.** Analyzers make mistakes, and the corpus is
immutable, so corrections are stored as an owner-scoped layer of decisions
about individual occurrences (identified by their source span). A learner can
assign a different lemma or exclude an occurrence; the *effective* vocabulary
is the analysis plus these decisions. A conservative detector flags only
occurrences whose lemma is missing from the local dictionary *and* for which
sentence-level evidence supports a competing lemma that would affect recurring
vocabulary. A missing dictionary entry alone never triggers a flag
([spec](features/lemma-review-and-correction.md),
[ADR 0081](adr/0081-learner-owned-occurrence-lemma-corrections.md)).

## 4. Measuring coverage

Reading research commonly places comfortable unassisted reading at roughly
95–98% coverage of a text's running words. Mouseion reports coverage against
the learner's own vocabulary, using an explicit and reproducible definition
([ADR 0025](adr/0025-analysis-coverage-threshold-metrics.md)):

- An **analyzable token** is an occurrence kept by the analysis and selection
  filters; punctuation, proper names, and stop words are excluded. The
  denominator is the total number of analyzable tokens.
- **Known-token coverage** is the share of analyzable tokens whose identity the
  learner knows. Words that only appeared in generated decks are never counted
  as known.
- **Threshold investment** answers "how many new words to reach 95%, 97%, or
  99%?" It orders eligible unknown identities by descending frequency in the
  book (ties broken lexically) and takes the shortest prefix whose occurrences
  reach the target. The comparison uses integer arithmetic, so rounding never
  changes the answer. If a target cannot be reached, it is reported as
  unreachable rather than approximated.

A structural profile of the text (sentence count, median and 90th-percentile
sentence length, long sentences) is reported alongside, without being combined
into a single "difficulty" score that the data can't support
([ADR 0026](adr/0026-structural-text-profile.md)).

## 5. Choosing vocabulary

Deck candidates are **content words** (NOUN, VERB, ADJ, ADV) that recur:

- at least **three** occurrences in the book, or
- exactly **two** in the book and at least **ten** across the learner's other
  analyzed books in the same language, so a word that is rare here but common
  in the learner's reading still qualifies.

Known vocabulary and the vocabulary reserved by the current reading are
excluded. Cards are ordered by each word's first appearance in the book, so
studying the deck follows reading order
([ADR 0048](adr/0048-frequency-floor-deck-selection.md),
[ADR 0085](adr/0085-reading-owned-book-vocabulary.md)). Starting a book
freezes its eligible vocabulary as a snapshot; finishing it marks that snapshot
as known. Generating cards never does: having been shown a card is not
evidence of knowing the word ([ADR 0036](adr/0036-primary-goal-justified-graduation.md),
[ADR 0078](adr/0078-book-dispositions-and-current-reading.md)).

## 6. Choosing example sentences

Each card shows one complete sentence from the book with the target word in
bold ([ADR 0029](adr/0029-recognition-card-sentence-presentation.md)). Choosing
that sentence is the lexicographic problem of selecting good dictionary
examples. Mouseion adapts the rule-based
[GDEX](https://github.com/zentrum-lexikographie/gdex) approach
(Good Dictionary Examples, as implemented for German at the Zentrum für
digitale Lexikographie der deutschen Sprache), computed over the persisted
parses ([spec](features/sentence-quality-scoring.md),
[ADR 0062](adr/0062-derived-sentence-quality-scoring.md)):

- **Knock-out criteria** reject a sentence outright: no finite verb as root
  with a subject (a fragment), wrong length, broken boundaries, structural
  noise, or a missing target.
- **Gradual criteria** rank the remainder. A sentence is penalized when the
  target sits only in a subordinate clause, when it relies on deictic words
  that need outside context (German space, time, and sentence-initial deixis),
  or when it is dense with named entities. It is rewarded for falling in an
  optimal length window.
- The knock-out and gradual halves have equal weight, as in GDEX. Source order
  breaks ties, so the choice is deterministic. The score and reasons are
  recorded on the deck manifest for each card.

German has the language-specific resources; Italian and Greek use the generic
rubric until their own deixis lists exist. Because scoring is derived at
export time from stored parses, a rubric change applies to existing analyses
without re-running NLP.

## 7. Dictionary and model evidence

**Dictionary.** Glosses and morphology come from an index derived from
[Kaikki.org](https://kaikki.org/)'s Wiktextract dumps of Wiktionary, filtered
to German, Italian, and Modern Greek. Each entry carries senses, gender,
article, plural, and IPA; German verbs also get principal parts
(*geht · ging · gegangen*). The index records its provenance: dump date,
extraction date, Wiktextract commit, source, and license (CC BY-SA 3.0 / GFDL).
That makes any card's dictionary evidence traceable to a dated source
([spec](features/dictionary-gloss-enrichment.md),
[ADR 0064](adr/0064-dictionary-enrichment-provider.md),
[ADR 0067](adr/0067-recognition-card-morphology-presentation.md),
[ADR 0068](adr/0068-recognition-card-meaning-and-form-presentation.md)).

**Contextual glosses.** A dictionary lists senses; a reader needs the sense
*in this sentence*. For each card, an LLM receives the lemma, the target form,
the one example sentence, and a bounded, frozen set of Wiktionary senses with
stable IDs. It returns a short English gloss for that use and cites the senses
supporting it, or states that the gloss is inferred from context. Cited IDs are
validated. If no defensible gloss is possible, the card is omitted and
reported rather than guessed
([spec](features/contextual-gloss-preparation.md),
[ADR 0079](adr/0079-contextual-glosses-require-llm.md)).

**Alignment.** The same call translates the whole sentence and proposes which
English words correspond to the target, which may be a discontinuous span such
as a separable verb. The proposal is validated: each excerpt must occur
exactly once, in order, with word boundaries, without covering the whole
sentence. An invalid proposal is discarded in full, and the translation is
shown without emphasis
([ADR 0080](adr/0080-llm-proposed-english-target-alignment.md)).

**What models do not decide.** No model output changes a vocabulary identity,
a coverage figure, or which words are selected; those come from the
annotation, the normalization profiles, and the rules above. Only the lemma,
target form, one sentence, and public dictionary evidence leave the
installation; book titles, full texts, and reading history do not
([ADR 0007](adr/0007-enrichment-providers-caching-privacy.md)).

## 8. Concordance

The concordance is a keyword-in-context (KWIC) view over the persisted corpus:
every occurrence of a lemma across the learner's current analyses in one
language. The current reading comes first, then the other books in title
order, and each line can be expanded to the full sentence or opened for study
with its token and dependency evidence. It
queries canonical lemmas and UPOS directly, so a German separable verb is
found under *anfangen* whether it appears as *anfangen* or *fängt … an*
([spec](features/concordance-foundation.md),
[ADR 0083](adr/0083-concordance-server-rendering-and-htmx-4.md),
[ADR 0086](adr/0086-reading-working-desk-hidden-visibility-and-concordance.md)).

## 9. Evaluation and limitations

**Measured.** The separable-verb rule is evaluated by a script that runs the
real producer over a committed public-domain sample: a 101-sentence excerpt
(2,095 tokens) from Goethe's *Die Leiden des jungen Werther*. On that sample, it made 10
reattachments, skipped 2 particle candidates outside the prefix list, and one
reattachment was marked debatable on review. The report records the sample's
hash and the Stanza version and can be regenerated
([evidence](evidence/german-separable-verb-precision.md)). The contextual-gloss
and alignment contracts have deterministic fixture certification; a review of
the configured model's output is pending ([review](reviews/english-target-alignment.md)).

**Known limitations.**

- Lemmas are only as good as the models. Errors are visible in the
  concordance and correctable per occurrence, but they are not detected
  exhaustively. For example, the Greek pipeline lemmatizes *είδα* ("I saw")
  as *έδαι* rather than *βλέπω*; suppletive forms like this are hard for a
  lemmatizer to recover from the surface form.
- Proper names are excluded by UPOS tag only. Names tagged as common nouns can
  reach a deck.
- Main-text selection reads EPUB 3 landmarks only; EPUB 2 `guide` references
  are not used, so such books are analyzed whole.
- The separable-verb evaluation is small and covers one late-18th-century
  text; precision on other registers and periods is not yet measured.
- "Known" vocabulary is modeled, not verified: finishing a book marks its
  snapshot as known without testing recall.
- GDEX resources exist for German only.
