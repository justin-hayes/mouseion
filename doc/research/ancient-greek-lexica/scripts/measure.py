"""Book 1 lexicon lookup measurement (research, not acceptance).

Inputs (all under scratch, hashes checked by the caller):
  oga conllu  - pinned tlg0032.tlg006.perseus-grc2_oga020_src.conllu
  kaikki      - kaikki.org-dictionary-AncientGreek.jsonl (deprecated postprocessed)
  lsj_dir     - PerseusDL/lexica CTS_XML_TEI/perseus/pdllex/grc/lsj
  ml_xml      - Perseus 1999.04.0058 (blinskey/middle-liddell fork)
"""
import sys, re, json, glob, collections, unicodedata
sys.path.insert(0, __file__.rsplit('/', 1)[0])
from betacode import beta_to_greek, canonical

OGA, KAIKKI, LSJ_DIR, ML_XML, OUT = sys.argv[1:6]
BOOK1 = re.compile(r'ref=Xen\. Anab\._1\.(\d+)\.(\d+)')
TRAIL = re.compile(r'(\d+)$')

# Heuristic native AGDT letter -> vocabulary class (ADR 0099 POS rule is not reimplemented)
VOCAB_LETTER = {'n': 'NOUN', 'a': 'ADJ', 'd': 'ADV', 'v': 'VERB'}
KAIKKI_CLASS = {'noun': 'NOUN', 'name': 'NOUN', 'adj': 'ADJ', 'adv': 'ADV', 'verb': 'VERB'}

# ---- OGA Book 1 printed tokens -------------------------------------------
letters = collections.Counter(); tokens = []; unresolved = 0
with open(OGA, encoding='utf-8') as f:
    sent_book1 = False
    for line in f:
        line = line.rstrip('\n')
        if line.startswith('# source'):
            sent_book1 = 'Xen. Anab._1.' in line
            continue
        if not line or line.startswith('#'):
            continue
        cols = line.split('\t')
        if len(cols) != 10 or not cols[0].isdigit():
            continue
        if not sent_book1 and not BOOK1.search(cols[9]):
            continue
        m = BOOK1.search(cols[9])
        if not m:
            continue
        form, lemma, pos = cols[1], cols[2], cols[3]
        letters[pos] += 1
        tokens.append((form, lemma, pos, cols[7], m.group(1), m.group(2)))

ident = collections.Counter(); ident_occ = collections.Counter(); ident_letter = {}
ident_cap = collections.Counter()
excluded = collections.Counter()
for form, lemma, pos, deprel, ch, sec in tokens:
    head = TRAIL.sub('', lemma)
    disc = TRAIL.search(lemma)
    disc = int(disc.group(1)) if disc else None
    cls = VOCAB_LETTER.get(pos)
    if cls is None:
        excluded[pos] += 1
        continue
    if head.endswith('εἰμί') or head == 'εἰμί':
        excluded['eimi'] += 1
        continue
    canon = canonical(head)
    key = (canon, disc, cls)
    ident[key] += 1
    ident_occ[key] += 1
    if form[:1].isupper():
        ident_cap[key] += 1

n_ident = len(ident)
numbered = sum(1 for k in ident if k[1] is not None)
print(json.dumps({'book1_tokens_matched': len(tokens), 'letters': dict(letters),
                  'excluded_non_vocab_letters': dict(excluded),
                  'vocab_identities_heuristic': n_ident,
                  'numbered_identities': numbered, 'plain_identities': n_ident - numbered}, ensure_ascii=False))

# ---- Kaikki index -----------------------------------------------------------
kaikki = collections.defaultdict(list)
with open(KAIKKI, encoding='utf-8') as f:
    for line in f:
        d = json.loads(line)
        kaikki[canonical(d['word'])].append({'pos': d.get('pos'), 'etym': d.get('etymology_number'),
                                             'n_senses': len(d.get('senses', [])),
                                             'book1_anab': sum(1 for s in d.get('senses', []) for e in (s.get('examples') or [])
                                                               if 'Xenophon, Anabasis 1.' in (e.get('ref') or ''))})

# ---- Perseus TEI-P4 dictionaries (LSJ, Middle Liddell) ---------------------
def load_perseus(files, anab_rx):
    idx = collections.defaultdict(list)
    for path in files:
        s = open(path, encoding='utf-8').read()
        for m in re.finditer(r'<(entryFree|entry)([^>]*)>(.*?)</\1>', s, re.S):
            attrs, body = m.group(2), m.group(3)
            if 'type="main"' not in attrs:
                continue
            k = re.search(r'key="([^"]*)"', attrs).group(1)
            g, num, unk = beta_to_greek(k)
            urns = anab_rx.findall(body)
            idx[canonical(g)].append({'key': k, 'digit': num, 'book1_anab': len(urns), 'unknown': unk})
    return idx

lsj_files = sorted(glob.glob(LSJ_DIR + '/grc.lsj.perseus-eng*.xml'))
lsj_rx = re.compile(r'urn:cts:greekLit:tlg0032\.tlg006\.perseus-grc\d:1:[0-9:]+')
lsj = load_perseus(lsj_files, lsj_rx)
ml = load_perseus([ML_XML], re.compile(r'(?!)'))  # ML has no structured passage IDs

# ---- measurements ---------------------------------------------------------
res = {}
def summarize(src_name, lookup):
    any_head = exact = digit = 0
    anab1_ids = 0
    for (canon, disc, cls) in ident:
        ents = lookup(canon, disc, cls)
        if ents['any']:
            any_head += 1
        if ents['compatible']:
            exact += 1
        if disc is not None and ents['digit_match']:
            digit += 1
        if ents['anab1']:
            anab1_ids += 1
    return {'identities': n_ident, 'headword_any_entry': any_head,
            'headword_pos_compatible': exact, 'numbered_identities_with_same_digit_entry': digit,
            'identities_with_book1_xen_anab_cites': anab1_ids}

def kaikki_lookup(canon, disc, cls):
    rows = kaikki.get(canon, [])
    return {'any': bool(rows), 'compatible': any(KAIKKI_CLASS.get(r['pos']) == cls for r in rows),
            'digit_match': False, 'anab1': any(r['book1_anab'] for r in rows)}

def perseus_lookup(idx):
    def f(canon, disc, cls):
        rows = idx.get(canon, [])
        if disc is None:
            same = [r for r in rows if r['digit'] is None]
        else:
            same = [r for r in rows if r['digit'] == disc]
        return {'any': bool(rows), 'compatible': False,  # no POS in Perseus dictionaries
                'digit_match': bool(same), 'anab1': any(r['book1_anab'] for r in same) if same else False,
                'plain_key': any(r['digit'] is None for r in rows),
                'homograph_keys': sum(1 for r in rows if r['digit'] is not None) > 1}
    return f

def token_weighted(lookup_fn):
    tot = hit = 0
    for k, n in ident.items():
        tot += n
        if lookup_fn(*k)['any']:
            hit += n
    return {'occurrences': tot, 'occurrences_with_any_entry': hit, 'share': round(hit / tot, 4)}

def plain_only(lookup):
    plain = [k for k in ident if k[1] is None]
    out = {'plain_identities': len(plain), 'any_entry': 0, 'pos_compatible': 0, 'book1_anab_cites': 0}
    for k in plain:
        r = lookup(*k)
        out['any_entry'] += r['any']; out['pos_compatible'] += r['compatible']; out['book1_anab_cites'] += r['anab1']
    return out

def proper_sensitivity(lookup):
    # identities whose every printed form is capitalised (names); reported separately
    names = [k for k in ident if ident_cap[k] == ident[k]]
    return {'capitalised_only_identities': len(names), 'any_entry': sum(bool(lookup(*k)['any']) for k in names)}

res['kaikki'] = summarize('kaikki', kaikki_lookup)
res['lsj'] = summarize('lsj', perseus_lookup(lsj))
res['middle_liddell'] = summarize('ml', perseus_lookup(ml))
def plain_key_summary(lookup):
    plain = [k for k in ident if k[1] is None]
    pk = sum(bool(lookup(*k).get('plain_key')) for k in plain)
    hg = sum(bool(lookup(*k).get('homograph_keys')) for k in plain)
    return {'plain_identities': len(plain), 'exact_unnumbered_key': pk, 'only_numbered_homograph_keys': sum(1 for k in plain if lookup(*k)['any'] and not lookup(*k).get('plain_key')), 'multiple_numbered_homograph_keys': hg}
res['plain_key'] = {'lsj': plain_key_summary(perseus_lookup(lsj)), 'middle_liddell': plain_key_summary(perseus_lookup(ml))}
res['plain_only'] = {'kaikki': plain_only(kaikki_lookup), 'lsj': plain_only(perseus_lookup(lsj)), 'middle_liddell': plain_only(perseus_lookup(ml))}
res['token_weighted'] = {'kaikki': token_weighted(kaikki_lookup), 'lsj': token_weighted(perseus_lookup(lsj)), 'middle_liddell': token_weighted(perseus_lookup(ml))}
res['capitalised_only'] = {'kaikki': proper_sensitivity(kaikki_lookup), 'lsj': proper_sensitivity(perseus_lookup(lsj))}
res['kaikki_homograph_headwords_among_book1'] = sum(1 for (c, d, k) in ident if len(kaikki.get(c, [])) > 1)
res['kaikki_book1_anab_cited_headwords'] = sum(1 for (c, d, k) in ident if any(r['book1_anab'] for r in kaikki.get(c, [])))
res['lsj_headword_count'] = len(lsj); res['ml_headword_count'] = len(ml)
res['lsj_unknown_beta_chars'] = sum(1 for v in lsj.values() for e in v if e['unknown'])
res['ml_unknown_beta_chars'] = sum(1 for v in ml.values() for e in v if e['unknown'])
json.dump({'counts': {'book1_tokens_matched': len(tokens), 'letters': dict(letters),
                      'excluded': dict(excluded), 'identities': n_ident, 'numbered': numbered},
           'results': res}, open(OUT, 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
print(json.dumps(res, ensure_ascii=False, indent=1))
