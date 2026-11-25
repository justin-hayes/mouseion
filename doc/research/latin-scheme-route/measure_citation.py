"""Join the OLA PAULA cts01 Citation layer to the CoNLL-U sentences and tokens
(by the t_N token id in column 10 against #t_N in cts01) for the classical
BY-SA Work files, and measure what the Greek citation code would accept.
usage: measure_citation.py <conllu-dir> <cts-dir> <manifest.tsv> <out.json>
Counts only; no text."""
import sys, re, json, collections
cdir, ctsdir, manifest, out = sys.argv[1:5]
rows = [l.rstrip('\n').split('\t') for l in open(manifest)][1:]
urns = [r[0] for r in rows if r[1] == 'core' and 'CC-BY-SA' in r[3]]
FEAT = re.compile(r'<feat id="[^"]*" xlink:href="#t_(\d+)" value="([^"]*)"')
DIGITS = re.compile(r'^\d+$')
tot = collections.Counter(); per = {}; shape = collections.Counter()
for u in urns:
    cts = {int(m.group(1)): m.group(2) for m in FEAT.finditer(open(f'{ctsdir}/{u}.cts01.xml', encoding='utf-8').read())}
    w = collections.Counter(); sent_vals = []
    prev = None; depths = collections.Counter(); nonnum_vals = set(); leaf_vals = []; last_val = None
    cur = []
    def flush():
        if not cur: return
        vals = [v for v in cur if v is not None and v != '']
        distinct = list(dict.fromkeys(vals))
        w['sentences'] += 1
        w['sent_distinct_%s' % min(len(distinct), 3)] += 1
        tops = {v.split('_')[0] for v in distinct}
        if len(tops) > 1: w['sent_cross_top_level'] += 1
        if not distinct: w['sent_no_citation'] += 1
        sent_vals.append(len(distinct)); cur.clear()
    for line in open(f'{cdir}/sentence-seg.ola.{u}.tok01_sentence-seg01.predicted.conllu', encoding='utf-8'):
        line = line.rstrip('\n')
        if line.startswith('#'): continue
        if not line: flush(); continue
        c = line.split('\t'); tid = int(c[9][2:]); w['tokens'] += 1
        if tid not in cts: w['tok_not_in_cts'] += 1; cur.append(None); continue
        v = cts[tid]; cur.append(v)
        if v == '': w['tok_empty_citation'] += 1; continue
        urn_prefix = v.startswith('urn:cts:')
        if urn_prefix:
            w['tok_urn_prefix'] += 1
            v = v.split('perseus-lat', 1)[1][2:] if 'perseus-lat' in v else v  # drop 'urn:...perseus-latN_'
        comps = v.split('_'); depths[len(comps)] += 1
        kinds = set()
        for x in comps:
            if DIGITS.match(x): continue
            if re.match(r'^\d+[A-Za-z]+$', x): kinds.add('digits_letter_suffix')
            elif re.match(r'^[A-Za-z]+\d*$', x): kinds.add('named_section')
            else: kinds.add('other')
        for k in kinds: w['tok_kind_' + k] += 1
        if kinds: w['works_kind_' + '+'.join(sorted(kinds))] += 0
        shape[tuple('N' if DIGITS.match(x) else 'S' for x in comps)] += 1
        if not all(DIGITS.match(x) for x in comps): w['tok_nonnumeric_component'] += 1; nonnum_vals.add(v)
        if v != last_val:
            leaf_vals.append(v); last_val = v
    flush()
    # monotonicity of numeric citation runs (distinct consecutive values)
    nums = []
    for v in leaf_vals:
        comps = v.split('_')
        if all(DIGITS.match(x) for x in comps): nums.append(tuple(int(x) for x in comps))
    dec = sum(1 for a, b in zip(nums, nums[1:]) if b < a)
    uniq_runs = len(leaf_vals); uniq = len(set(leaf_vals))
    first = next((v for v in leaf_vals), None); lastv = leaf_vals[-1] if leaf_vals else None
    tops = {v.split('_')[0] for v in leaf_vals}
    per[u] = dict(tokens=w['tokens'], sentences=w['sentences'], tok_not_in_cts=w['tok_not_in_cts'], tok_empty=w['tok_empty_citation'],
                  nonnumeric_tokens=w['tok_nonnumeric_component'], modal_depth=depths.most_common(1)[0][0] if depths else 0,
                  citation_runs=uniq_runs, distinct_citations=uniq, run_decreases=dec, top_level_values=len(tops),
                  sent_multi=w['sent_distinct_2'] + w['sent_distinct_3'], sent_cross_top=w['sent_cross_top_level'], sent_no_cit=w['sent_no_citation'])
    tot.update(w)
    tot['works'] += 1
    if w['tok_nonnumeric_component']: tot['works_with_nonnumeric'] += 1
    for k in ('digits_letter_suffix', 'named_section', 'other'):
        if w['tok_kind_' + k]: tot['works_with_' + k] += 1
    if w['tok_urn_prefix']: tot['works_with_urn_prefix'] += 1
    if dec: tot['works_with_run_decreases'] += 1
    if w['tok_not_in_cts']: tot['works_with_tokens_not_in_cts'] += 1
    if w['tok_empty_citation']: tot['works_with_empty'] += 1
    if w['sent_no_citation']: tot['works_with_uncited_sentences'] += 1
    tot['run_decreases'] += dec
    if w['tok_nonnumeric_component'] == 0 and w['tok_not_in_cts'] == 0 and w['tok_empty_citation'] == 0 and dec == 0: tot['works_clean_for_greek_style_citation'] += 1
res = dict(total=dict(tot), shapes={'.'.join(k): v for k, v in shape.most_common(12)}, per_work=per)
json.dump(res, open(out, 'w'), indent=1, sort_keys=True)
print(json.dumps(res['total'], indent=1)); print(res['shapes'])
