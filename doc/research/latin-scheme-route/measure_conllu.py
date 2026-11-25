"""Whole-layer inventory of the OLA v0.2.0 CoNLL-U files for the classical
Works, read against what the Greek importer (internal/ancientgreek) and the
AGDT adapter (internal/syntax/agdt) assume.
usage: measure_conllu.py <dir-of-conllu> <manifest.tsv> <out.json>
Only counts are written."""
import sys, os, json, re, collections, unicodedata
conllu_dir, manifest, out = sys.argv[1:4]
rows = [l.rstrip('\n').split('\t') for l in open(manifest)][1:]
meta = {r[0]: dict(cls=r[1], repo=r[2], licence=r[3], tokens=int(r[5]), words=int(r[6])) for r in rows}
# What the Greek code accepts (internal/ancientgreek/ogasrc.go, internal/syntax/agdt/agdt.go).
GREEK_POS = set('nvadlgcrpmiux')
GREEK_FEATS = {  # featureMappings in ogasrc.go (value letters)
    'Person': '123', 'Number': 'spd', 'Tense': 'pifla rt'.replace(' ', ''),
    'Mood': 'isomnp', 'Voice': 'apme', 'Gender': 'mfnc', 'Case': 'ngdavl', 'Degree': 'cs'}
GREEK_CAT = {'PRED','SBJ','OBJ','ATR','ADV','PNOM','OCOMP','ATV','AtvV','APOS','COORD','AuxC','AuxP','AuxY','AuxZ','AuxV','ExD','AuxX','AuxK','AuxG'}
FEAT = {'person':'Person','number':'Number','tense':'Tense','mood':'Mood','voice':'Voice','gender':'Gender','case':'Case','degree':'Degree'}

def scope(urn):
    m = meta[urn]
    if m['cls'] == 'core': return 'classical'
    return m['cls']

agg = collections.defaultdict(lambda: collections.defaultdict(collections.Counter))
works = collections.defaultdict(list)
per = {}
for urn in sorted(meta):
    if meta[urn]['cls'] not in ('core', 'borderline'): continue
    path = f'{conllu_dir}/sentence-seg.ola.{urn}.tok01_sentence-seg01.predicted.conllu'
    sc = scope(urn) + ('-bysa' if 'CC-BY-SA' in meta[urn]['licence'] else '-nc')
    a = agg[sc]
    sents = 0; toks = 0; roots_bad = 0; headbad = 0; dup = 0; punct_root = 0
    labelsets = set(); posets = set()
    with open(path, encoding='utf-8') as f:
        cur = []
        def flush():
            global sents, toks, roots_bad, headbad, punct_root
            if not cur: return
            sents += 1; toks += len(cur)
            ids = {int(c[0]) for c in cur}
            roots = [c for c in cur if c[6] == '0']
            if len(roots) != 1: roots_bad += 1
            else:
                r = roots[0]
                a['rootrel'][r[7]] += 1
                if r[3] == 'u': punct_root += 1
            if any(int(c[6]) != 0 and int(c[6]) not in ids for c in cur): headbad += 1
            a['sentlen'][min(len(cur)//10*10, 200)] += 1
            if any(c[7].split('_')[0] == 'ExD' or '_ExD' in c[7] for c in cur): a['sent_with_ExD']['n'] += 1
            a['sent']['n'] += 1
            cur.clear()
        for line in f:
            line = line.rstrip('\n')
            if line.startswith('#'):
                a['comment'][line.split(' = ')[0]] += 1; continue
            if not line:
                flush(); continue
            c = line.split('\t')
            a['cols'][len(c)] += 1
            cur.append(c)
            form, lemma, pos, xpos, feats, head, rel, misc = c[1], c[2], c[3], c[4], c[5], c[6], c[7], c[9]
            a['tok']['n'] += 1
            a['pos'][pos] += 1
            a['xpos'][xpos] += 1
            a['miscshape'][re.sub(r'\d+', 'N', misc)] += 1
            base = rel.split('_')[0] if not rel.startswith('_') else rel
            suffix = rel[len(base):]
            a['rel_base'][base] += 1
            a['rel_suffix'][suffix or '(none)'] += 1
            if rel not in GREEK_CAT: a['rel_not_exact_greek_cat']['n'] += 1
            if rel not in GREEK_CAT and base in GREEK_CAT: a['rel_suffix_only_miss']['n'] += 1
            if pos != 'u':
                a['nonpunct']['n'] += 1
            if pos not in GREEK_POS: a['pos_unknown_to_greek'][pos] += 1
            if feats == '': a['feats_empty_string']['n'] += 1
            if feats not in ('_', ''):
                for kv in feats.split('|'):
                    k, v = kv.split('=')
                    a['feat_kv'][f'{k}={v}'] += 1
                    if k in GREEK_FEATS and v not in GREEK_FEATS[k]: a['feat_unmapped_by_greek'][f'{k}={v}'] += 1
                    if k not in GREEK_FEATS: a['feat_key_unknown'][k] += 1
            if re.search(r'\d$', lemma): a['lemma_trailing_digit']['n'] += 1
            if lemma[:1].isupper(): a['lemma_initial_upper'][pos] += 1
            if pos != 'u':
                for ch in lemma:
                    if not ('a' <= ch.lower() <= 'z'): a['lemma_nonaz_char'][unicodedata.name(ch, 'U+%04X' % ord(ch))] += 1
                if 'j' in lemma.lower(): a['lemma_has_j']['n'] += 1
                if lemma.lower() in ('_', 'unknown', '-', ''): a['lemma_missing']['n'] += 1
            for ch in form:
                if ord(ch) > 127: a['form_nonascii'][unicodedata.name(ch, 'U+%04X' % ord(ch))] += 1
            if form.lower() != form and pos == 'n': a['noun_form_capital']['n'] += 1
            if re.search(r'[\[\]<>{}()]', form): a['form_bracket']['n'] += 1
            if len(form) == 1 and form.isalpha(): a['form_single_letter']['n'] += 1
            if pos == '-' or form in ('_', '[0]'): a['ellipsis_like']['n'] += 1
        flush()
    per[urn] = dict(scope=sc, sentences=sents, tokens=toks, bad_root_sentences=roots_bad, bad_head_sentences=headbad, punct_root=punct_root)
    assert toks == meta[urn]['tokens'], (urn, toks, meta[urn]['tokens'])
res = {sc: {k: dict(v) if k not in ('sentlen',) else dict(sorted(v.items())) for k, v in d.items()} for sc, d in agg.items()}
json.dump(dict(aggregate=res, per_work=per), open(out, 'w'), indent=1, sort_keys=True, default=str)
for sc in res: print(sc, res[sc]['tok'], res[sc]['sent'])
