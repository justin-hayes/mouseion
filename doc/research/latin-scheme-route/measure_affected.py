"""Token- and sentence-level effects of running the OLA classical BY-SA CoNLL-U
through the existing Greek mapping (internal/ancientgreek/ogasrc.go) and the
AGDT adapter's exact-match Category (internal/syntax/agdt/agdt.go).
usage: measure_affected.py <conllu-dir> <manifest.tsv> <out.json> (counts only)"""
import sys, json, collections
d, manifest, out = sys.argv[1:4]
rows = [l.rstrip('\n').split('\t') for l in open(manifest)][1:]
urns = [r[0] for r in rows if r[1] == 'core' and 'CC-BY-SA' in r[3]]
GREEK_VALUES = {'Person': '123', 'Number': 'spd', 'Tense': 'pifla' + 'rt', 'Mood': 'isomnp', 'Voice': 'apme', 'Gender': 'mfnc', 'Case': 'ngdavl', 'Degree': 'cs'}
EXACT = {'PRED','SBJ','OBJ','ATR','ADV','PNOM','OCOMP','ATV','AtvV','APOS','COORD','AuxC','AuxP','AuxY','AuxZ','AuxV','ExD','AuxX','AuxK','AuxG'}
real_unmapped = {('Case','b'), ('Mood','g'), ('Mood','d'), ('Mood','u'), ('Voice','d'), ('Degree','p')}
a = collections.Counter(); sl = []; sents_with_other = 0; sents = 0
for u in urns:
    cur = []
    def flush():
        global sents, sents_with_other
        if not cur: return
        sents += 1; sl.append(len(cur))
        if any(c[7] not in EXACT and c[3] != 'u' for c in cur): sents_with_other += 1
        if any(c[7] not in EXACT and c[3] != 'u' and not c[7].startswith(('AuxK','AuxX','AuxG')) for c in cur): a['sentences_with_nonexact_nonpunct_label'] += 1
        cur.clear()
    for line in open(f'{d}/sentence-seg.ola.{u}.tok01_sentence-seg01.predicted.conllu', encoding='utf-8'):
        line = line.rstrip('\n')
        if line.startswith('#'): continue
        if not line: flush(); continue
        c = line.split('\t'); cur.append(c)
        if c[3] == 'u': continue
        a['nonpunct'] += 1
        kv = dict(x.split('=') for x in c[5].split('|')) if c[5] not in ('_', '') else {}
        lose = [(k, v) for k, v in kv.items() if (k, v) in real_unmapped]
        if lose: a['tokens_losing_a_feature_value'] += 1
        if any(x in (('Mood','g'), ('Mood','d'), ('Mood','u')) for x in lose): a['tokens_gerund_gerundive_supine'] += 1
        if ('Case','b') in lose: a['tokens_ablative'] += 1
        if c[3] == 'v' and kv.get('Mood') == 'p': a['participles_verb_mood_p'] += 1
        if c[3] == 'p': a['pron_p'] += 1
        if c[7] not in EXACT: a['nonpunct_tokens_label_not_exact_greek'] += 1
        if c[7].endswith('_CO') or '_CO' in c[7]: a['label_has_CO'] += 1
        if c[3] == 'n' and c[2][:1].isupper(): a['noun_with_capital_lemma'] += 1
        if c[3] == 'n': a['nouns'] += 1
        if c[3] == 'c' and c[7].split('_')[0] == 'AuxC': a['c_AuxC'] += 1
        if c[3] == 'd' and c[7].split('_')[0] == 'AuxC': a['d_AuxC'] += 1
    flush()
sl.sort()
res = dict(a); res.update(sentences=sents, sent_len_median=sl[len(sl)//2], sent_len_p95=sl[int(.95*len(sl))], sent_len_max=sl[-1],
                          sentences_over_100_tokens=sum(1 for x in sl if x > 100), sentences_with_any_nonexact_label=sents_with_other)
json.dump(res, open(out, 'w'), indent=1, sort_keys=True); print(json.dumps(res, indent=1))
