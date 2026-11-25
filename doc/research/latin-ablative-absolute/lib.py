"""Readers and the ablative-absolute rule. Aggregates only leave this code."""
import re, collections, xml.etree.ElementTree as ET

class T:
    __slots__=('i','form','lemma','pos','mood','case','rel','head','sent','mfeats','raw')
def strip_rel(r):
    """AGDT label -> (base, conjunct flag, ap flag, exd flag)."""
    m = re.match(r'^([A-Za-z]+?)((?:_(?:CO|AP|ExD\d*))*)(?:_.*)?$', r)
    base = r.split('_')[0]
    return base, '_CO' in r, '_AP' in r, '_ExD' in r

def read_ola(path):
    sents=[]; cur=[]
    for line in open(path,encoding='utf8'):
        if line.startswith('#'): continue
        line=line.rstrip('\n')
        if not line:
            if cur: sents.append(cur); cur=[]
            continue
        c=line.split('\t')
        t=T(); t.i=int(c[0]); t.form=c[1]; t.lemma=c[2]; t.pos=c[3]
        f={}
        if c[5]!='_':
            for kv in c[5].split('|'):
                k,_,v=kv.partition('='); f[k]=v
        t.mfeats=f; t.mood=f.get('Mood',''); t.case=f.get('Case','')
        t.head=int(c[6]); t.rel=c[7]; t.raw=c
        cur.append(t)
    if cur: sents.append(cur)
    return sents

def read_ldt(path):
    sents=[]
    root=ET.parse(path).getroot()
    for s in root.iter('sentence'):
        cur=[]
        for w in s.findall('word'):
            t=T(); t.i=int(w.get('id')); t.form=w.get('form') or ''; t.lemma=w.get('lemma') or ''
            pt=((w.get('postag') or '')+'---------')[:9]
            t.pos='-' if w.get('insertion_id') else pt[0]
            if pt[0]=='t': t.pos='v'
            t.mood=pt[4] if pt[4]!='-' else ''; t.case=pt[7] if pt[7]!='-' else ''
            t.mfeats={}
            t.head=int(w.get('head') or 0); t.rel=w.get('relation') or ''
            t.raw=(w.get('id'),w.get('insertion_id'),s.get('id'),s.get('subdoc'))
            cur.append(t)
        if cur: sents.append((s.get('id'),s.get('subdoc'),cur))
    return sents

# ---------- shared normalisation (as latin-oga-equivalent/common.py) ----------
import difflib, unicodedata, math
def norm_form(s):
    s = unicodedata.normalize("NFD", s)
    s = "".join(c for c in s if unicodedata.category(c) != "Mn").lower()
    s = s.replace("j", "i").replace("v", "u")
    return re.sub(r"[^a-z]", "", s)

def align(a, b):
    """index map a->b by global alignment on normalised forms (only tokens with a key)."""
    ka=[norm_form(t.form) for t in a]; kb=[norm_form(t.form) for t in b]
    ia=[i for i,k in enumerate(ka) if k]; ib=[i for i,k in enumerate(kb) if k]
    sm=difflib.SequenceMatcher(None,[ka[i] for i in ia],[kb[i] for i in ib],autojunk=False)
    m={}
    for blk in sm.get_matching_blocks():
        for k in range(blk.size): m[ia[blk.a+k]]=ib[blk.b+k]
    return m

def wilson(k,n,z=1.959964):
    if n==0: return (0.0,0.0)
    p=k/n; d=1+z*z/n; c=p+z*z/(2*n)
    r=z*math.sqrt(p*(1-p)/n+z*z/(4*n*n))
    return ((c-r)/d,(c+r)/d)

# ---------- generic tree (0-based index within sentence) ----------
class Tree:
    """toks: list of T (1-based .i, .head 0=root). kind 'agdt' or 'ud'."""
    def __init__(self, toks, kind):
        self.t=toks; self.kind=kind
        self.byid={t.i:k for k,t in enumerate(toks)}
        self.ch=collections.defaultdict(list)
        self.root=[]
        for k,t in enumerate(toks):
            h=self.byid.get(t.head) if t.head else None
            if h is None or h==k: self.root.append(k)
            else: self.ch[h].append(k)
    def head(self,k):
        h=self.t[k].head
        return self.byid.get(h) if h else None
    def base(self,k):
        r=self.t[k].rel
        return r.split('_')[0] if self.kind=='agdt' else r.split(':')[0]
    def is_coord(self,k):
        return self.kind=='agdt' and self.base(k)=='COORD'

def children_through_coord(tr,k):
    out=[]; seen={k}
    def walk(p):
        for c in tr.ch[p]:
            if c in seen: continue
            seen.add(c)
            if tr.is_coord(c): walk(c)
            else: out.append(c)
    walk(k); return out

def sbj_agdt(tr,k):
    """SBJ children of participle k, direct, through COORD below, and via COORD ancestors."""
    subs=[c for c in children_through_coord(tr,k) if tr.base(c)=='SBJ']
    seen={k}; h=tr.head(k)
    while h is not None and tr.is_coord(h) and h not in seen:
        seen.add(h)
        subs+= [c for c in children_through_coord(tr,h) if tr.base(c)=='SBJ' and c!=k]
        h=tr.head(h)
    return subs

def agdt_hits(tr):
    """Rule AA-1 over an AGDT-scheme tree. Returns list of (anchor index, extent-size)."""
    out=[]
    for k,t in enumerate(tr.t):
        if t.pos!='v' or t.mood!='p' or t.case!='b': continue
        if tr.base(k)!='ADV': continue
        subs=[s for s in sbj_agdt(tr,k) if tr.t[s].case=='b' and tr.t[s].pos not in ('-','u')]
        if subs: out.append(k)
    return out

def ud_hits(tr):
    """Same rule restated over UD: Abl participle as advcl* with an Abl nsubj."""
    out=[]
    for k,t in enumerate(tr.t):
        f=t.mfeats
        if f.get('VerbForm')!='Part' or f.get('Case')!='Abl': continue
        if tr.base(k) not in ('advcl',): continue
        def subs(p):
            r=[]
            for c in tr.ch[p]:
                if tr.t[c].rel.split(':')[0] in ('nsubj','csubj') : r.append(c)
            return r
        s=subs(k)
        # conj-shared subject
        h=tr.head(k)
        if not s and tr.t[k].rel.startswith('conj') and h is not None: s=subs(h)
        if any(tr.t[c].mfeats.get('Case')=='Abl' for c in s): out.append(k)
    return out
