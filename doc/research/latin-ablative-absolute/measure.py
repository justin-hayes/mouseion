"""Ablative-absolute rule agreement with gold. usage: python3 -I measure.py <data_dir> <out.json> [dump.txt]
Aggregates go to the JSON. The optional dump is scratch-only (forms, for reading false hits); never commit it."""
import sys, json, collections, glob, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lib
D, OUT = sys.argv[1], sys.argv[2]
DUMP = open(sys.argv[3], 'w') if len(sys.argv) > 3 else None
PROSE = {'phi0448.phi001','phi0474.phi013','phi0631.phi001','phi0972.phi001','phi1348.abo012','phi1351.phi005'}
VERSE = {'phi0620.phi001','phi0690.phi003','phi0959.phi006','phi0975.phi001'}
R = {}

def pct(a, b): return round(a / b, 4) if b else None
def summary(tp, nO, nG, extra=None):
    lo, hi = lib.wilson(tp, nO); rlo, rhi = lib.wilson(tp, nG)
    d = dict(ola_hits=nO, gold_hits=nG, agree=tp, precision=pct(tp, nO), wilson_lo=round(lo, 4), wilson_hi=round(hi, 4),
             recall=pct(tp, nG), recall_lo=round(rlo, 4), recall_hi=round(rhi, 4))
    if extra: d.update(extra)
    return d

def agdt_class(tr, k):
    t = tr.t[k]; subs = lib.sbj_agdt(tr, k)
    return '|'.join(['pos=' + t.pos, 'mood=' + (t.mood or '-'), 'case=' + ('abl' if t.case == 'b' else (t.case or '-')),
                     'rel=' + tr.base(k), 'sbj=' + ('abl' if any(tr.t[s].case == 'b' for s in subs) else ('other' if subs else 'none'))])
def ud_class(tr, k):
    t = tr.t[k]; subs = [c for c in tr.ch[k] if tr.t[c].rel.split(':')[0] in ('nsubj', 'csubj')]
    return '|'.join(['upos=' + t.pos, 'form=' + t.mfeats.get('VerbForm', '-'), 'case=' + t.mfeats.get('Case', '-'),
                     'rel=' + t.rel, 'sbj=' + ('abl' if any(tr.t[s].mfeats.get('Case') == 'Abl' for s in subs) else ('other' if subs else 'none'))])

def read_hf_lists(path):
    out = collections.defaultdict(set); doc = None; cur = []
    def flush():
        if cur: out[doc].add(tuple(cur))
    for line in open(path, encoding='utf8'):
        if line.startswith('# doc_id'): flush(); cur = []; doc = line.split('=', 1)[1].strip().split(':')[-1]; doc = '.'.join(doc.split('.')[:2]); continue
        if line.startswith('#'): continue
        line = line.rstrip('\n')
        if not line: flush(); cur = []; continue
        c = line.split('\t'); f = lib.norm_form(c[1])
        if f: cur.append(f)
    flush(); return out
HF = {s: read_hf_lists(f'{D}/hf/{s}.120.conllu') for s in ('test', 'dev')}

# ---------------- B: LDT 2.1 gold ----------------
LDT = {'phi0448.phi001': 'phi0448.phi001.perseus-lat1.tb.xml', 'phi0474.phi013': 'phi0474.phi013.perseus-lat1.tb.xml',
       'phi0631.phi001': 'phi0631.phi001.perseus-lat1.tb.xml', 'phi0972.phi001': 'phi0972.phi001.perseus-lat1.xml',
       'phi1348.abo012': 'phi1348.abo012.perseus-lat1.tb.xml', 'phi1351.phi005': 'phi1351.phi005.perseus-lat1.tb.xml',
       'phi0620.phi001': 'phi0620.phi001.perseus-lat1.tb.xml', 'phi0690.phi003': 'phi0690.phi003.perseus-lat1.tb.xml',
       'phi0959.phi006': 'phi0959.phi006.perseus-lat1.tb.xml', 'phi0975.phi001': 'phi0975.phi001.perseus-lat1.tb.xml'}
OLA_ID = {'phi1351.phi005': 'phi1351.phi004'}
ev = collections.defaultdict(list)   # (work, subset) -> events
per_work = {}
for w, fn in LDT.items():
    gold = lib.read_ldt(f'{D}/ldt/repo/v2.1/Latin/texts/{fn}')
    gp = []; gtree = {}; sentof = {}
    for n, (sid, sd, s) in enumerate(gold):
        tr = lib.Tree(s, 'agdt')
        for k, t in enumerate(s):
            if t.pos != '-': gp.append(t)
            gtree[id(t)] = (tr, k); sentof[id(t)] = n
    ofs = sorted(glob.glob(f'{D}/ola/{OLA_ID.get(w, w)}.*.conllu'))
    of = max(ofs, key=os.path.getsize)
    OS = lib.read_ola(of); otoks = []; otree = {}
    for s in OS:
        tr = lib.Tree(s, 'agdt')
        for k, t in enumerate(s): otoks.append(t); otree[len(otoks) - 1] = (tr, k)
    am = lib.align(gp, otoks)
    inv = {v: g for g, v in am.items()}
    # held-out flags per gold sentence
    skeys = collections.defaultdict(list)
    for t in gp: skeys[sentof[id(t)]].append(lib.norm_form(t.form))
    flag = {}
    for n, ks in skeys.items():
        key = tuple(k for k in ks if k)
        flag[n] = 'test' if key in HF['test'].get(w, ()) else ('dev' if key in HF['dev'].get(w, ()) else 'train')
    gh = set()
    for n, (sid, sd, s) in enumerate(gold):
        tr = gtree[id(s[0])][0]
        for k in lib.agdt_hits(tr): gh.add(id(s[k]))
    oh = set(); flat = 0
    for s in OS:
        for k in lib.agdt_hits(otree[flat][0]): oh.add(flat + k)
        flat += len(s)
    covered = sorted(am.values()); import bisect
    def near(x):
        i = bisect.bisect_left(covered, x)
        return any(0 <= j < len(covered) and abs(covered[j] - x) <= 3 for j in (i - 1, i))
    rows = ev[w]
    for x in oh:
        if x in inv:
            g = gp[inv[x]]; tr, k = gtree[id(g)]
            rows.append(('O', flag[sentof[id(g)]], id(g) in gh, agdt_class(tr, k), w))
            if id(g) in gh and DUMP and flag[sentof[id(g)]] != 'train':
                otr, ok_ = otree[x]; sb = [otr.t[q].form for q in lib.sbj_agdt(otr, ok_)]; lo = min([ok_] + lib.sbj_agdt(otr, ok_))
                DUMP.write('LDTTP\t%s\t%s\tprev=%s\t%s\t%s\thead=%s\n' % (w, flag[sentof[id(g)]], otr.t[lo-1].form if lo else '', sb, g.form, otr.t[otr.head(ok_)].form if otr.head(ok_) is not None else None))
            if id(g) not in gh and DUMP:
                hd = tr.head(k); hd = tr.t[hd] if hd is not None else None
                otr, ok_ = otree[x]; sb = [otr.t[q].form for q in lib.sbj_agdt(otr, ok_)]
                DUMP.write('LDTFP\t%s\t%s\t%s\t%s\tID=%s/%s\tgoldhead=%s/%s\tola_sbj=%s\n' % (w, flag[sentof[id(g)]], ' '.join(t.form for t in tr.t), g.form, gold[sentof[id(g)]][0], g.raw[0], hd.form if hd else None, hd.rel if hd else None, sb))
        elif near(x): rows.append(('U', 'unk', None, '', w))
    for g in gp:
        if id(g) in gh:
            gi = gpos = None
    gpidx = {id(t): n for n, t in enumerate(gp)}
    for g in gp:
        if id(g) in gh:
            n = gpidx[id(g)]
            if n in am:
                x = am[n]; tr, k = otree[x]
                rows.append(('G', flag[sentof[id(g)]], x in oh, agdt_class(tr, k), w))
                if x not in oh and DUMP:
                    DUMP.write('LDTFN\t%s\t%s\t%s\n' % (w, ' '.join(t.form for t in tr.t), tr.t[k].form))
            else: rows.append(('GU', flag[sentof[id(g)]], None, '', w))
    per_work[w] = dict(gold_tokens=len(gp), ola_tokens=len(otoks), aligned=len(am), ola_file=os.path.basename(of),
                       gold_sentences=len(skeys), heldout_test_sentences=sum(1 for v in flag.values() if v == 'test'),
                       heldout_dev_sentences=sum(1 for v in flag.values() if v == 'dev'))

def agg(works, subsets):
    fp = collections.Counter(); fn = collections.Counter(); tp = nO = nG = un = gun = 0
    for w in works:
        for kind, sub, ok, cls, _ in ev[w]:
            if kind == 'U' and 'all' in subsets: un += 1
            if kind == 'GU' and (sub in subsets or 'all' in subsets): gun += 1
            if sub not in subsets and 'all' not in subsets: continue
            if kind == 'O':
                nO += 1; tp += bool(ok)
                if not ok: fp[cls] += 1
            if kind == 'G':
                nG += 1
                if not ok: fn[cls] += 1
    # recall uses G events; precision uses O events; agree counted from O
    gtp = sum(1 for w in works for kind, sub, ok, cls, _ in ev[w] if kind == 'G' and ok and (sub in subsets or 'all' in subsets))
    lo, hi = lib.wilson(tp, nO); rlo, rhi = lib.wilson(gtp, nG)
    return dict(ola_hits_aligned=nO, agree=tp, precision=pct(tp, nO), wilson_lo=round(lo, 4), wilson_hi=round(hi, 4),
                gold_hits_aligned=nG, found=gtp, recall=pct(gtp, nG), recall_lo=round(rlo, 4), recall_hi=round(rhi, 4),
                ola_hits_in_range_unaligned=un, gold_hits_unaligned=gun,
                false_positive_classes=dict(fp.most_common()), missed_classes=dict(fn.most_common()))
R['ldt_prose_all_sentences'] = agg(PROSE, {'all'})
R['ldt_prose_heldout_test'] = agg(PROSE, {'test'})
R['ldt_prose_heldout_dev'] = agg(PROSE, {'dev'})
R['ldt_prose_heldout_test_dev'] = agg(PROSE, {'test', 'dev'})
R['ldt_prose_train_sentences'] = agg(PROSE, {'train'})
R['ldt_verse_all_sentences'] = agg(VERSE, {'all'})
R['ldt_verse_heldout_test_dev'] = agg(VERSE, {'test', 'dev'})
R['ldt_by_work'] = per_work
R['ldt_by_work_prose_agree'] = {w: agg([w], {'all'}) for w in PROSE}
for w in R['ldt_by_work_prose_agree']:
    for k in ('false_positive_classes', 'missed_classes'): R['ldt_by_work_prose_agree'][w].pop(k)

# ---------------- C: CIRCSE Germania (UD gold) ----------------
def read_circse(prefix):
    sents = []; cur = []; sid = None
    for fn in ('train', 'dev', 'test'):
        for line in open(f'{D}/circse/repo/la_circse-ud-{fn}.conllu', encoding='utf8'):
            if line.startswith('# sent_id'): sid = line.split('=', 1)[1].strip(); continue
            if line.startswith('#'): continue
            line = line.rstrip('\n')
            if not line:
                if cur and sid.startswith(prefix): sents.append((sid, cur))
                cur = []; continue
            c = line.split('\t')
            if '-' in c[0] or '.' in c[0]: continue
            t = lib.T(); t.i = int(c[0]); t.form = c[1]; t.lemma = c[2]; t.pos = c[3]; t.head = int(c[6]); t.rel = c[7]
            f = {}
            if c[5] != '_':
                for kv in c[5].split('|'):
                    k, _, v = kv.partition('='); f[k] = v
            t.mfeats = f; t.mood = ''; t.case = f.get('Case', '')
            cur.append(t)
    return sents

def circse_eval(prefix, ola_name, label):
    CS = read_circse(prefix)
    gtoks = []; gtr = {}
    for sid, s in CS:
        tr = lib.Tree(s, 'ud')
        for k, t in enumerate(s): gtoks.append(t); gtr[len(gtoks) - 1] = (tr, k, sid)
    OS = lib.read_ola(f'{D}/ola/{ola_name}.conllu'); otoks = []; otree = {}
    for s in OS:
        tr = lib.Tree(s, 'agdt')
        for k, t in enumerate(s): otoks.append(t); otree[len(otoks) - 1] = (tr, k)
    ol = [t for t in otoks if t.pos != 'u']; omap = [i for i, t in enumerate(otoks) if t.pos != 'u']
    am0 = lib.align(gtoks, ol); am = {g: omap[o] for g, o in am0.items()}   # gold flat -> ola flat (punctuation removed on OLA side)
    inv = {v: g for g, v in am.items()}
    ud_rule = set(); gold_abs = set(); gold_abs_part = set()
    for gi, (tr, k, sid) in gtr.items():
        pass
    base = 0
    for sid, s in CS:
        tr = gtr[base][0]
        for k in lib.ud_hits(tr): ud_rule.add(base + k)
        for k, t in enumerate(s):
            if t.rel == 'advcl:abs': gold_abs.add(base + k)
        base += len(s)
    # participial gold abs: UD-rule shape regardless of the relation label
    def part_shape(gi):
        tr, k, sid = gtr[gi]; t = tr.t[k]
        s = [c for c in tr.ch[k] if tr.t[c].rel.split(':')[0] in ('nsubj', 'csubj')]
        h = tr.head(k)
        if not s and t.rel.startswith('conj') and h is not None: s = [c for c in tr.ch[h] if tr.t[c].rel.split(':')[0] in ('nsubj', 'csubj')]
        return t.mfeats.get('VerbForm') == 'Part' and t.mfeats.get('Case') == 'Abl' and any(tr.t[c].mfeats.get('Case') == 'Abl' for c in s)
    gold_abs_part = {g for g in gold_abs if part_shape(g)}
    oh = set(); flat = 0
    for s in OS:
        for k in lib.agdt_hits(otree[flat][0]): oh.add(flat + k)
        flat += len(s)
    # precision: ola hit -> gold advcl:abs at the aligned token
    O_al = [x for x in oh if x in inv]; O_un = [x for x in oh if x not in inv]
    tp = sum(1 for x in O_al if inv[x] in gold_abs)
    tp_rule = sum(1 for x in O_al if inv[x] in ud_rule)
    fp = collections.Counter()
    for x in O_al:
        g = inv[x]
        if g not in gold_abs:
            tr, k, sid = gtr[g]; fp[ud_class(tr, k)] += 1
            if DUMP: DUMP.write('CIRCSEFP\t%s\t%s\t%s\tID=%s/%s\n' % (label, ' '.join(t.form for t in tr.t), tr.t[k].form, sid, tr.t[k].i))
    # recall
    found_all = [g for g in gold_abs if g in am and am[g] in oh]
    found_part = [g for g in gold_abs_part if g in am and am[g] in oh]
    fn = collections.Counter()
    for g in gold_abs:
        if g in am and am[g] not in oh:
            tr, k, sid = gtr[g]; x = am[g]; ot, ok_ = otree[x]
            fn['ola:' + agdt_class(ot, ok_) + '|gold:' + ud_class(tr, k).split('|')[0]] += 1
            if DUMP: DUMP.write('CIRCSEFN\t%s\t%s\t%s\tID=%s/%s\n' % (label, ' '.join(t.form for t in tr.t), tr.t[k].form, sid, tr.t[k].i))
    out = dict(gold_words=len(gtoks), ola_words=len(ol), aligned_gold_words=len(am),
               gold_advcl_abs=len(gold_abs), gold_advcl_abs_aligned=sum(1 for g in gold_abs if g in am),
               gold_advcl_abs_participial_shape=len(gold_abs_part),
               gold_abs_by_anchor_form=dict(collections.Counter('Part' if gtr[g][0].t[gtr[g][1]].mfeats.get('VerbForm') == 'Part' else 'nonpart:' + gtr[g][0].t[gtr[g][1]].pos for g in gold_abs)),
               ud_rule_on_gold=dict(hits=len(ud_rule), labelled_abs=len(ud_rule & gold_abs), precision=pct(len(ud_rule & gold_abs), len(ud_rule)),
                                    wilson=[round(x, 4) for x in lib.wilson(len(ud_rule & gold_abs), len(ud_rule))],
                                    recall_of_abs=pct(len(ud_rule & gold_abs), len(gold_abs)),
                                    non_abs_rel=dict(collections.Counter(gtr[g][0].t[gtr[g][1]].rel for g in ud_rule - gold_abs))),
               ola_hits=len(oh), ola_hits_aligned=len(O_al), ola_hits_unaligned=len(O_un),
               precision_vs_gold_advcl_abs=summary(tp, len(O_al), len(gold_abs_part))['precision'],
               precision_wilson=[round(x, 4) for x in lib.wilson(tp, len(O_al))],
               agree=tp, agree_with_gold_rule_shape=tp_rule,
               recall_all_gold_abs=dict(found=len(found_all), of=sum(1 for g in gold_abs if g in am), value=pct(len(found_all), sum(1 for g in gold_abs if g in am)),
                                        wilson=[round(x, 4) for x in lib.wilson(len(found_all), sum(1 for g in gold_abs if g in am))]),
               recall_participial_shape=dict(found=len(found_part), of=sum(1 for g in gold_abs_part if g in am), value=pct(len(found_part), sum(1 for g in gold_abs_part if g in am))),
               false_positive_classes=dict(fp.most_common()), missed_classes=dict(fn.most_common()))
    return out
R['circse_germania'] = circse_eval('TacGerma', 'phi1351.phi002.perseus-lat1', 'germania')
json.dump(R, open(OUT, 'w'), indent=1, sort_keys=True)
for k, v in R.items():
    if k.startswith('ldt_prose') or k.startswith('ldt_verse'):
        if isinstance(v, dict) and 'precision' in v:
            print(k, {x: y for x, y in v.items() if not x.endswith('classes')})
c = R['circse_germania']; print({x: y for x, y in c.items() if x not in ('false_positive_classes', 'missed_classes')}); print(c['false_positive_classes']); print(c['missed_classes'])
