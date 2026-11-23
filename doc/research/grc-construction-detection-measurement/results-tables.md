# Results

## conditional

| | Plato | Xenophon | Pooled |
|---|---|---|---|
| Corpus hits | 1249 | 492 | 1741 |
| Precision (sampled) | 0.80 (16/20; 0.58–0.92) | 0.95 (19/20; 0.76–0.99) | 0.88 (35/40; 0.74–0.95) |
| Precision, hit-weighted | | | 0.84 |
| Recall (estimated) | 0.81 | 0.96 | 0.85 |
| Misses outside the loose query (random sentences) | 0 | 0 | |
| Abstention | 0.00 (0/1249; 0.00–0.00) | 0.00 (0/492; 0.00–0.01) | 0.00 (0/1741; 0.00–0.00) |
| Detector ceiling (parser errors forgiven): precision / recall | 0.89 (16/18; 0.67–0.97) / 0.84 | 0.95 (19/20; 0.76–0.99) / 0.97 | 0.92 (35/38; 0.79–0.97) / 0.88 |

Errors: false hit / detector:rule: 3; false hit / parser:label: 2; miss (loose) / detector:scope: 11; miss (loose) / parser:label: 3

## articular_infinitive

| | Plato | Xenophon | Pooled |
|---|---|---|---|
| Corpus hits | 543 | 82 | 625 |
| Precision (sampled) | 1.00 (20/20; 0.84–1.00) | 0.95 (19/20; 0.76–0.99) | 0.97 (39/40; 0.87–1.00) |
| Precision, hit-weighted | | | 0.99 |
| Recall (estimated) | 1.00 | 0.84 | 0.98 |
| Misses outside the loose query (random sentences) | 0 | 0 | |
| Abstention | 0.00 (0/543; 0.00–0.01) | 0.00 (0/82; 0.00–0.04) | 0.00 (0/625; 0.00–0.01) |
| Detector ceiling (parser errors forgiven): precision / recall | 1.00 (20/20; 0.84–1.00) / 1.00 | 1.00 (19/19; 0.83–1.00) / 0.85 | 1.00 (39/39; 0.91–1.00) / 0.98 |

Subtype precision (random hits plus top-up, pooled): acc 1.00 (17/17; 0.82–1.00); dat 0.90 (9/10; 0.60–0.98); gen 0.93 (13/14; 0.69–0.99); nom 0.90 (9/10; 0.60–0.98)

Errors: false hit / parser:head: 2; miss (loose) / detector:rule: 1

## result

| | Plato | Xenophon | Pooled |
|---|---|---|---|
| Corpus hits | 139 | 56 | 195 |
| Precision (sampled) | 1.00 (20/20; 0.84–1.00) | 1.00 (20/20; 0.84–1.00) | 1.00 (40/40; 0.91–1.00) |
| Precision, hit-weighted | | | 1.00 |
| Recall (estimated) | 0.94 | 0.84 | 0.91 |
| Misses outside the loose query (random sentences) | 0 | 0 | |
| Abstention | 0.01 (1/140; 0.00–0.04) | 0.02 (1/57; 0.00–0.09) | 0.01 (2/197; 0.00–0.04) |
| Detector ceiling (parser errors forgiven): precision / recall | 1.00 (20/20; 0.84–1.00) / 0.99 | 1.00 (20/20; 0.84–1.00) / 1.00 | 1.00 (40/40; 0.91–1.00) / 0.99 |

Subtype precision (random hits plus top-up, pooled): none 1.00 (10/10; 0.72–1.00); actual 1.00 (10/10; 0.72–1.00); natural 1.00 (28/28; 0.88–1.00)

Errors: miss (abstain) / detector:rule: 1; miss (abstain) / parser:head: 1; miss (loose) / parser:ellipsis: 2; miss (loose) / parser:head: 5

## purpose

| | Plato | Xenophon | Pooled |
|---|---|---|---|
| Corpus hits | 200 | 122 | 322 |
| Precision (sampled) | 0.60 (12/20; 0.39–0.78) | 0.60 (12/20; 0.39–0.78) | 0.60 (24/40; 0.45–0.74) |
| Precision, hit-weighted | | | 0.60 |
| Recall (estimated) | 1.00 | 1.00 | 1.00 |
| Misses outside the loose query (random sentences) | 0 | 0 | |
| Abstention | 0.00 (0/200; 0.00–0.02) | 0.00 (0/122; 0.00–0.03) | 0.00 (0/322; 0.00–0.01) |
| Detector ceiling (parser errors forgiven): precision / recall | 0.80 (12/15; 0.55–0.93) / 1.00 | 0.67 (12/18; 0.44–0.84) / 1.00 | 0.73 (24/33; 0.56–0.85) / 1.00 |

Errors: false hit / detector:rule: 9; false hit / parser:head: 2; false hit / parser:morph: 5

## genitive_absolute

| | Plato | Xenophon | Pooled |
|---|---|---|---|
| Corpus hits | 296 | 138 | 434 |
| Precision (sampled) | 0.89 (17/19; 0.69–0.97) | 0.90 (18/20; 0.70–0.97) | 0.90 (35/39; 0.76–0.96) |
| Precision, hit-weighted | | | 0.90 |
| Recall (estimated) | 0.75 | 0.72 | 0.74 |
| Misses outside the loose query (random sentences) | 0 | 0 | |
| Abstention | 0.05 (15/311; 0.03–0.08) | 0.01 (2/140; 0.00–0.05) | 0.04 (17/451; 0.02–0.06) |
| Detector ceiling (parser errors forgiven): precision / recall | 1.00 (17/17; 0.82–1.00) / 1.00 | 1.00 (18/18; 0.82–1.00) / 0.74 | 1.00 (35/35; 0.90–1.00) / 0.90 |

Errors: false hit / parser:head: 1; false hit / parser:label: 3; miss (excluded) / detector:rule: 2; miss (loose) / detector:rule: 4; miss (loose) / parser:label: 1; miss (loose) / parser:morph: 1

## indirect_statement

| | Plato | Xenophon | Pooled |
|---|---|---|---|
| Corpus hits | 1692 | 753 | 2445 |
| Precision (sampled) | 0.80 (16/20; 0.58–0.92) | 0.85 (17/20; 0.64–0.95) | 0.82 (33/40; 0.68–0.91) |
| Precision, hit-weighted | | | 0.82 |
| Recall (estimated) | 0.42 | 0.84 | 0.50 |
| Misses outside the loose query (random sentences) | 0 | 0 | |
| Abstention | 0.00 (0/1692; 0.00–0.00) | 0.00 (0/753; 0.00–0.01) | 0.00 (0/2445; 0.00–0.00) |
| Detector ceiling (parser errors forgiven): precision / recall | 0.89 (16/18; 0.67–0.97) / 0.66 | 0.89 (17/19; 0.69–0.97) / 0.85 | 0.89 (33/37; 0.75–0.96) / 0.71 |

Subtype precision (random hits plus top-up, pooled): hoti-hos 1.00 (10/10; 0.72–1.00); infinitive 0.81 (30/37; 0.66–0.91)

Errors: false hit / detector:rule: 4; false hit / parser:head: 3; miss (loose) / detector:list: 15; miss (loose) / detector:rule: 1; miss (loose) / parser:ellipsis: 5; miss (loose) / parser:head: 1; miss (loose) / parser:lemma: 1

