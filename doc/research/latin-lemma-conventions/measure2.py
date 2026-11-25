"""Convention features, merge/split analysis, unmatched-remainder taxonomy and rescue rules.
usage: LEXICA=... measure2.py <conllu-dir> <outdir>"""
import sys, os, re, json, collections, unicodedata
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
C = collections.Counter
cdir, outdir = sys.argv[1:3]
S = common.work_sets(); D = dicts.load()
def nuv(s): return common.nfd_strip(s).lower().replace('j', 'i').replace('v', 'u')
def hw_raw(which, orth=True):
    s = set()
    for k, b, o, t, i in D[which]:
        s.add(b)
        if orth: s.update(o)
    return s
LS_RAW, EL_RAW = hw_raw('ls'), hw_raw('el')
ALL_RAW = LS_RAW | EL_RAW
IDX = {nuv(x) for x in ALL_RAW}
ROMAN = re.compile(r'^[IVXLCDM]+$')
GREEK = re.compile(r'[Ͱ-Ͽἀ-῿]')
def load(urns):
    toks = C(); pos = collections.defaultdict(C); works = collections.defaultdict(set)
    forms = collections.defaultdict(C); dep = C(); rows = []
    for u in urns:
        for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'):
            if p == 'u': continue
            toks[l] += 1; pos[l][p] += 1; works[l].add(u); forms[l][f.lower()] += 1
            if 'Voice=d' in ft: dep[l] += 1
            rows.append((f, l, p, ft))
    return toks, pos, works, forms, dep, rows
out = {}
def cause(l, c, pos, forms):
    if re.search(r'\d', l): return 'numeric'
    if ROMAN.match(l): return 'roman-numeral'
    if GREEK.search(l): return 'greek-script'
    if not l.isascii(): return 'other-non-ascii'
    if '.' in l: return 'abbreviation'
    if '-' in l: return 'hyphenated'
    if len(l) == 1: return 'single-letter'
    if l[:1].isupper(): return 'proper-noun(capitalised)'
    if l.lower() in forms and forms[l.lower()] == c and True: return 'lemma=form(only)'
    return 'lowercase-other'
for sname in ('prose80', 'verse61', 'borderline'):
    toks, pos, works, forms, dep, rows = load(S[sname])
    nt = sum(toks.values())
    r = {}
    # --- orthography: v/j/u/i in lemmas vs forms
    lem_v = sum(c for l, c in toks.items() if 'v' in l.lower()); lem_j = sum(c for l, c in toks.items() if 'j' in l.lower())
    fo_v = fo_j = 0
    for f, l, p, ft in rows:
        fo_v += 'v' in f.lower(); fo_j += 'j' in f.lower()
    r['orthography'] = dict(tokens=nt, lemma_tokens_with_v=lem_v, lemma_types_with_v=sum('v' in l.lower() for l in toks),
        lemma_tokens_with_j=lem_j, lemma_types_with_j=sum('j' in l.lower() for l in toks), form_tokens_with_v=fo_v, form_tokens_with_j=fo_j,
        lemma_types_nonascii=sum(not l.isascii() for l in toks), lemma_tokens_nonascii=sum(c for l, c in toks.items() if not l.isascii()),
        lemma_types_digits=sum(bool(re.search(r'\d', l)) for l in toks), lemma_tokens_digits=sum(c for l, c in toks.items() if re.search(r'\d', l)),
        lemma_types_capitalised=sum(l[:1].isupper() for l in toks), lemma_tokens_capitalised=sum(c for l, c in toks.items() if l[:1].isupper()),
        lemma_types_hyphen=sum('-' in l for l in toks), lemma_tokens_hyphen=sum(c for l, c in toks.items() if '-' in l),
        lemma_types_period=sum('.' in l for l in toks), lemma_tokens_period=sum(c for l, c in toks.items() if '.' in l),
        lemma_types_diacritic=sum(any(unicodedata.combining(ch) for ch in unicodedata.normalize('NFD', l)) for l in toks),
        lemma_types_greek_script=sum(bool(GREEK.search(l)) for l in toks), lemma_tokens_greek_script=sum(c for l, c in toks.items() if GREEK.search(l)),
        lemma_types_roman=sum(bool(ROMAN.match(l)) for l in toks), lemma_tokens_roman=sum(c for l, c in toks.items() if ROMAN.match(l)),
        lemma_types_with_digit_homograph_suffix=sum(bool(re.search(r'[A-Za-z]\d$', l)) for l in toks))
    # --- POS inventory
    pc = C()
    for l, c in toks.items():
        for p, n in pos[l].items(): pc[p] += n
    r['pos_tokens'] = dict(pc)
    # --- enclitics
    enc = {}
    for e in ('que', 'ne', 've'):
        sel = [(f, l, p) for f, l, p, ft in rows if f.lower() == e]
        enc[f'split_{e}'] = dict(tokens=len(sel), lemma_pos={f'{a}|{b}': n for (a, b), n in C((l, p) for f, l, p in sel).most_common(5)})
    unsplit = {}
    for e in ('que', 'ne', 've'):
        sel = [(f.lower(), l, p) for f, l, p, ft in rows if f.lower().endswith(e) and len(f) > len(e) + 1]
        same = sum(1 for f, l, p in sel if l.lower() == f)
        # lemma is a different headword than the form: possible enclitic stripped by the lemmatiser
        strip = sum(1 for f, l, p in sel if l.lower() != f and l.lower() == f[:-len(e)])
        unsplit[e] = dict(tokens_ending=len(sel), lemma_equals_form=same, lemma_equals_form_minus_enclitic=strip)
    r['enclitics'] = dict(split=enc, unsplit_endings=unsplit)
    # lemma ending in que/ne/ve as a headword?
    def lem_end(e): 
        ts = {l: c for l, c in toks.items() if l.lower().endswith(e) and len(l) > len(e) + 1}
        return dict(types=len(ts), tokens=sum(ts.values()), matched_tokens=sum(c for l, c in ts.items() if nuv(l) in IDX),
                    matched_types=sum(1 for l in ts if nuv(l) in IDX))
    r['lemma_ending_enclitic'] = {e: lem_end(e) for e in ('que', 'ne', 've')}
    # --- verbs: citation forms
    vt = {l: c for l, c in toks.items() if pos[l].get('v', 0) * 2 >= c}
    end = C()
    for l, c in vt.items():
        k = 'o' if l.endswith('o') else 'or' if l.endswith('or') else 'm' if l.endswith('m') else 'sum-compound' if l.endswith('sum') else 're/ri(infinitive-like)' if re.search(r'(re|ri|i)$', l) else 'other'
        end[k + '_types'] += 1; end[k + '_tokens'] += c
    r['verb_lemma_endings'] = dict(end)
    # --- deponents
    deptok = sum(dep.values()); depl = {l: dep[l] for l in dep}
    d_or = sum(c for l, c in depl.items() if l.endswith('or')); d_o = sum(c for l, c in depl.items() if l.endswith('o') and not l.endswith('or'))
    dm = 0; dsib = 0; dsibt = 0
    for l, c in depl.items():
        if l.endswith('o'):
            # does the -or form exist as a headword?
            if nuv(l + 'r') in IDX: dsib += 1; dsibt += c
    r['deponents'] = dict(voice_d_tokens=deptok, voice_d_lemma_types=len(depl), lemma_ends_or_tokens=d_or, lemma_ends_o_tokens=d_o,
        lemma_ends_o_types=sum(l.endswith('o') for l in depl), lemma_o_but_or_headword_exists_types=dsib, tokens_of_those=dsibt,
        voice_d_lemma_matched_tokens=sum(c for l, c in depl.items() if nuv(l) in IDX), voice_d_lemma_matched_types=sum(1 for l in depl if nuv(l) in IDX))
    # passive-voice tokens whose lemma ends in -or (passive form as lemma)
    r['passive_lemma_or_tokens'] = sum(1 for f, l, p, ft in rows if p == 'v' and 'Voice=p' in ft and l.endswith('or') and nuv(l) in IDX)
    # --- numerals
    mt = {l: c for l, c in toks.items() if pos[l].get('m', 0) * 2 >= c}
    r['numerals'] = dict(types=len(mt), tokens=sum(mt.values()), roman_types=sum(bool(ROMAN.match(l)) for l in mt), roman_tokens=sum(c for l, c in mt.items() if ROMAN.match(l)),
        digit_types=sum(bool(re.search(r'\d', l)) for l in mt), digit_tokens=sum(c for l, c in mt.items() if re.search(r'\d', l)),
        word_types=sum(1 for l in mt if l.isalpha() and not ROMAN.match(l)), word_tokens=sum(c for l, c in mt.items() if l.isalpha() and not ROMAN.match(l)),
        word_matched_tokens=sum(c for l, c in mt.items() if l.isalpha() and not ROMAN.match(l) and nuv(l) in IDX))
    # --- proper nouns (capitalised) match
    cap = {l: c for l, c in toks.items() if l[:1].isupper() and l.isascii() and '.' not in l}
    r['proper_nouns'] = dict(types=len(cap), tokens=sum(cap.values()), matched_types=sum(1 for l in cap if nuv(l) in IDX), matched_tokens=sum(c for l, c in cap.items() if nuv(l) in IDX))
    low = {l: c for l, c in toks.items() if not l[:1].isupper() and l.isascii()}
    r['lowercase_ascii'] = dict(types=len(low), tokens=sum(low.values()), matched_types=sum(1 for l in low if nuv(l) in IDX), matched_tokens=sum(c for l, c in low.items() if nuv(l) in IDX))
    # --- cause taxonomy of unmatched
    um = {l: c for l, c in toks.items() if nuv(l) not in IDX}
    tx = collections.defaultdict(lambda: [0, 0])
    for l, c in um.items():
        k = cause(l, c, pos[l], forms[l]); tx[k][0] += 1; tx[k][1] += c
    r['unmatched'] = dict(types=len(um), tokens=sum(um.values()), by_cause={k: dict(types=v[0], tokens=v[1]) for k, v in sorted(tx.items(), key=lambda x: -x[1][1])})
    # --- rescue rules over unmatched lowercase-other + hyphenated + capital
    def rules():
        yield 'drop-hyphen', lambda l: l.replace('-', '')
        yield 'ae/oe->e', lambda l: l.replace('ae', 'e').replace('oe', 'e')
        yield 'e->ae (e for ae)', lambda l: re.sub(r'e', 'ae', l, count=1)
        yield 'strip-final-que/ne/ve', lambda l: re.sub(r'(que|ne|ve)$', '', l)
        yield 'o->or (deponent)', lambda l: l + 'r' if l.endswith('o') else l
        yield 'or->o', lambda l: l[:-1] if l.endswith('or') else l
        yield 'um/us (neuter/masc swap)', lambda l: l[:-2] + 'us' if l.endswith('um') else l
        yield 'us->um', lambda l: l[:-2] + 'um' if l.endswith('us') else l
        yield 'is/es->e', lambda l: l[:-2] + 'e' if l.endswith(('is', 'es')) else l
        yield 'ti->ci', lambda l: l.replace('ti', 'ci')
        yield 'ci->ti', lambda l: l.replace('ci', 'ti')
        yield 'ph->f / f->ph', lambda l: l.replace('ph', 'f')
        yield 'y->i', lambda l: l.replace('y', 'i')
        yield 'double-consonant collapse', lambda l: re.sub(r'([bcdfgklmnprst])\1', r'\1', l)
        yield 'single->double l/n/s/c/p/t', lambda l: re.sub(r'([a-z])(l|n|s|c|p|t|r|m)([aeiou])', lambda m: m.group(1) + m.group(2) * 2 + m.group(3), l, count=1)
        yield 'qu->c (quom/cum, quoi/cui)', lambda l: l.replace('qu', 'c')
        yield 'uo->u / vo->vu', lambda l: l.replace('uo', 'u')
        yield 'ii->i (genitive/compounds)', lambda l: l.replace('ii', 'i')
        yield 'h-drop', lambda l: l.replace('h', '')
        yield 'ch/th/rh->c/t/r', lambda l: l.replace('ch', 'c').replace('th', 't').replace('rh', 'r')
    resc = {}
    for name, fn in rules():
        rt = rk = 0
        for l, c in um.items():
            if l.isascii() and not re.search(r'\d|\.', l) and nuv(fn(nuv(l))) in IDX and nuv(fn(nuv(l))) != nuv(l):
                rt += 1; rk += c
        resc[name] = dict(types=rt, tokens=rk)
    r['rescue_rules_single'] = resc
    out[sname] = r
    # --- ranked remainder (prose80 only)
    if sname == 'prose80':
        lines = ['rank\tlemma\ttokens\tworks\tpos\tcause\tlemma_equals_form_share']
        cum = 0; tot = sum(um.values())
        ranked = sorted(um.items(), key=lambda x: (-x[1], x[0]))
        for i, (l, c) in enumerate(ranked[:300], 1):
            fs = round(forms[l].get(l.lower(), 0) / c, 2)
            lines.append(f'{i}\t{l}\t{c}\t{len(works[l])}\t{pos[l].most_common(1)[0][0]}\t{cause(l, c, pos[l], forms[l])}\t{fs}')
        open(f'{outdir}/unmatched_prose80_top300.tsv', 'w').write('\n'.join(lines) + '\n')
        cumulative = {}
        run = 0
        for i, (l, c) in enumerate(ranked, 1):
            run += c
            if i in (10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, len(ranked)): cumulative[i] = round(run / tot, 4)
        out[sname]['unmatched']['cumulative_token_share_by_top_n_types'] = cumulative
json.dump(out, open(f'{outdir}/features.json', 'w'), indent=1)
print(json.dumps(out['prose80'], indent=1)[:9000])
