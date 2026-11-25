"""Aggregate a kaikki.org Latin JSONL (post-processed). Usage: python3 -I measure_kaikki.py la.jsonl"""
import sys,json,re,collections,unicodedata,random
C=collections.Counter
n=0;pos=C();lem=0;form=0;rows=[];st=C();tagc=C();sensec=C()
FORM=('form_of','alt_of')
heads=0;macron_head=0;ndiac=0
for line in open(sys.argv[1],encoding='utf8'):
    d=json.loads(line); n+=1
    ss=d.get('senses',[])
    isform=bool(ss) and all(any(k in s for k in ('form_of','alt_of','alternative_of')) or any(t in ('form-of','alt-of','alternative') for t in s.get('tags',[])) for s in ss)
    pos[d.get('pos')]+=1
    if isform: form+=1; continue
    lem+=1
    st['lemma_records']+=1
    g=[s for s in ss if s.get('glosses')]
    st['lemma_with_gloss']+=bool(g)
    sensec[min(len(g),8)]+=1
    for s in ss:
        for t in s.get('tags',[]): tagc[t]+=1
    st['senses']+=len(ss)
    st['senses_w_gloss']+=len(g)
    st['senses_nested_glosses(len>1)']+=sum(len(s['glosses'])>1 for s in g)
    st['has_forms']+=bool(d.get('forms'))
    st['has_head_templates']+=bool(d.get('head_templates'))
    st['has_examples']+=any(s.get('examples') for s in ss)
    st['senses_with_examples']+=sum(bool(s.get('examples')) for s in ss)
    if d.get('head_templates'):
        ex=d['head_templates'][0].get('expansion','')
        heads+=1
        if '̄' in unicodedata.normalize('NFD',ex): macron_head+=1
    w=d.get('word','')
    st['word_has_macron']+=('̄' in unicodedata.normalize('NFD',w))
    st['word_capitalised']+=w[:1].isupper()
    st['word_has_space']+=(' ' in w)
print('records',n,'form/alt-of records',form,'lemma-like',lem)
print(dict(st));print('head expansion with macron',macron_head,'of',heads)
print('lemma pos',pos.most_common(10))
print('gloss-bearing senses per lemma record',sorted(sensec.items()))
print('top sense tags',tagc.most_common(25))
