"""Measure Lewis & Short TEI structure against the ADR 0104 outline rule.
Usage: python3 -I measure_ls.py <lat.ls.perseus-eng2.xml>
Prints aggregates only."""
import sys,re,json,collections,random,xml.etree.ElementTree as ET
r=ET.parse(sys.argv[1]).getroot()
es=list(r.iter('entryFree'))
txt=lambda e:''.join(e.itertext())
C=collections.Counter
def defs(s):
    out=[]
    for c in s:
        if c.tag in('cit','bibl'): break
        if c.tag=='hi' and c.get('rend')=='ital':
            t=txt(c).strip(' ,;:.')
            if t: out.append(t)
    return out
XREF=re.compile(r'^\s*(?:[^,]*,)*[^,]*?(?:=|\bv\.|\bcf\.|\bsee\b|\bsame as\b)',re.I)
st=C()
lv=C()
nlab=C()
per=[]
for e in es:
    ss=e.findall('sense')
    d=[defs(s) for s in ss]
    st['entries']+=1
    st['senses']+=len(ss)
    if not ss: st['entries_no_sense']+=1
    ital_direct=[c for c in e if c.tag=='hi']
    if not ss and ital_direct: st['no_sense_but_entry_level_ital']+=1
    if ss:
        st['senses_with_defs']+=sum(bool(x) for x in d)
        if any(d): st['entries_with_any_def']+=1
        else: st['entries_with_senses_no_def']+=1
        f=ss[0]
        if any(c.tag in('orth','itype','pos','gen') for c in f): st['first_sense_holds_forms_children']+=1
        if len(ss)==1: st['single_sense_entries']+=1
        if d[0]: st['first_sense_has_def']+=1
    # short entries (<= 160 chars) are mostly cross-references
    t=txt(e)
    if len(t)<=160: st['short_entries']+=1
    if re.search(r'\bv\.\s+[A-Za-z]',t) and len(t)<=160: st['short_xref_entries']+=1
    for s in ss:
        lv[s.get('level')]+=1
        n=s.get('n') or ''
        shape='roman' if re.fullmatch(r'[IVX]+',n) else 'upper' if re.fullmatch(r'[A-Z]',n) else 'arabic' if re.fullmatch(r'\d+',n) else 'lower' if re.fullmatch(r'[a-z]',n) else 'paren' if re.fullmatch(r'\([a-z0-9]+\)',n) else 'other:'+n[:6]
        nlab[(s.get('level'),shape)]+=1
    per.append((e.get('key'),len(ss),sum(bool(x) for x in d)))
print(json.dumps(st,indent=1))
print('level counts',sorted(lv.items()))
print('n label shapes by level',sorted(nlab.items(),key=lambda x:-x[1])[:14])
nd=[x for x in per if x[1]]
print('ratio senses with defs/senses',round(st['senses_with_defs']/st['senses'],4))

# --- junk-definition analysis (italic abbreviations/fragments taken as definitions)
JUNK=re.compile(r'^(?:fin|init|med|gen|inf|dat|abl|acc|subj|ind|imper|part|sup|adj\.? ?[mfn]?|[mfn]|P\. a|cf|v|e\. g|i\. e|id|ib|sq|sqq|al|etc)\.?$',re.I)
allsense=[(e,s) for e in es for s in e.findall('sense')]
alld=[(e,s,defs(s)) for e,s in allsense]
junkdef=sum(1 for _,_,d in alld for x in d if JUNK.match(x) or len(x)<=3)
totd=sum(len(d) for _,_,d in alld)
print('definition strings',totd,'junk-or<=3 chars',junkdef)
only_junk=0;ent_only_junk=0
byent=collections.defaultdict(list)
for e,s,d in alld: byent[e.get('id')].extend(d)
for k,d in byent.items():
    if d and all(JUNK.match(x) or len(x)<=3 for x in d): ent_only_junk+=1
print('entries whose only definitions are junk',ent_only_junk)
print('entries by type', dict(C(e.get('type') for e in es)))
