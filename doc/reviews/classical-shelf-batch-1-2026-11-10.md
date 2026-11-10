# Classical shelf batch 1 (2026-11-10)

The first reviewed batch on the Classical catalog shelf (#2418, parent #2401).
It adds six works by Plato and the remaining six books of Xenophon's
*Anabasis*: seven works and 22 reading units in all.

## Inputs

- Greek text and annotations: OGA `oga_src` v0.2.0 (the layers
  `oga-src-v0.2.0-text` and `oga-src-v0.2.0-annotations`, already pinned).
- English reference translations: Perseus `canonical-greekLit` at
  `0189b9c8d8d1f148133f2f0af309b4ee90ea4332`, the same commit as the Brownson
  pin. Each file is its own layer in `internal/classical/layers.json` and
  carries the card label and deck notice an exported deck shows for it.
- Generator run: `make classical-manifest` with all files present, then
  `make verify-classical-manifest` with and without the upstream directory.
  Both pass with 22 definitions.

## Chosen works

| Work | Edition | Split | Units | Words | Translation | Reason |
|---|---|---|---|---|---|---|
| Xenophon, *Anabasis* | Marchant 1904 | 7 books | 7 | 6,703 to 10,915 per book | Brownson (1921–22) | Completes the pilot, which shipped Book 1. Prose narrative suited to early readers. |
| Plato, *Apology* | Burnet 1905 | whole work | 1 (17–42) | 8,894 | Fowler (1914) | A short, self-contained speech; a common first Plato text. |
| Plato, *Crito* | Burnet 1905 | whole work | 1 (43–54) | 4,339 | Fowler (1914) | The shortest work in the batch; follows the *Apology*. |
| Plato, *Phaedo* | Burnet 1905 | whole work | 1 (57–118) | 22,608 | Fowler (1914) | Completes the trial-and-death sequence. |
| Plato, *Symposium* | Burnet 1910 | whole work | 1 (172–223) | 17,542 | Lamb (1925) | Requested; a widely read dialogue. |
| Plato, *Meno* | Burnet 1903 | whole work | 1 (70–100) | 10,411 | Lamb (1924) | Requested; short and widely read. |
| Plato, *Republic* | Burnet 1905 | 10 books | 10 | 6,907 to 10,336 per book | Shorey (1935–37 printing) | Requested; its books are a natural unit. |

Plato is cited by Stephanus page only in OGA (for example `17`, or `1.327`
for the *Republic*), so the shorter dialogues are read whole rather than split
at an arbitrary page. Edition years follow the printings in the Perseus CTS
records, not the first editions.

## Notes report review

- **Crossing sentences**: none in any unit.
- **Translation alignment**: every unit's translation aligns over the unit's
  full citation range.
- ***Anabasis* start sections**: Books 2, 3, 5 and 7 start at section 2 and
  Book 4 at 4.1.5. Marchant's text omits the recap sections that open those
  books, so these are the first sections OGA has, not a gap.
- **Rejected**: Herodotus, *Histories* (`tlg0016.tlg001`). The generator
  refuses it with `source_malformed (line 60286): source header is not
  recoverable`. About 300 sentences cite lettered chapters (2.121A–F,
  5.92A–G, 6.86A–D, 7.8A–D, 7.9A–C, 7.10A–H, 7.16A–C, 8.60A–C, 8.68A–C,
  8.140A–B, 9.7A–B), which the citation parser does not accept. It is left
  out rather than guessed; shelving it needs citation support for lettered
  chapters first.

## Departures from upstream metadata

Each override records why its curatorial fields differ from upstream:

- Work titles and author names match the OGA file metadata recorded in
  `layers.json`.
- Greek titles follow the Perseus CTS labels; OGA records no Greek title.
- Edition labels name the Greek edition and its CTS printing year; OGA names
  only the Perseus edition.
- The *Anabasis* override drops its pilot `books` restriction so all seven
  books ship.

## Licensing

All translations are Perseus digitisations under CC BY-SA 4.0 (from the
repository README; the Plato files carry no licence element of their own).
Fowler (1914) and Lamb (1924, 1925) are US public domain in print. Shorey's
*Republic* volume 6 (first published 1935) may still be under US copyright
until 2031 if it was renewed; it ships on the Perseus licence by maintainer
decision and is flagged **unverified** in
[`doc/third-party-data.md`](../third-party-data.md).

## Keys

The new reading-unit keys are permanent once shipped: `plato-apology`,
`plato-crito`, `plato-meno`, `plato-phaedo`, `plato-symposium`,
`plato-republic-1` to `plato-republic-10`, and `xenophon-anabasis-2` to
`xenophon-anabasis-7`.
