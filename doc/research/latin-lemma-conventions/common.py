import csv, re, sys, os, unicodedata, collections, xml.etree.ElementTree as ET
HERE = os.path.dirname(os.path.abspath(__file__))
VERSE = {'phi0119','phi0134','phi0472','phi0550','phi0620','phi0660','phi0690','phi0692','phi0830','phi0893','phi0911','phi0917','phi0959','phi0969','phi0975','phi1020','phi1035','phi1103','phi1276','phi1294','phi1345'}
AUTHOR = {'phi0119':'Plautus','phi0134':'Terence','phi0448':'Caesar','phi0472':'Catullus','phi0474':'Cicero','phi0478':'Q. Cicero','phi0550':'Lucretius','phi0588':'Nepos','phi0620':'Propertius','phi0631':'Sallust','phi0660':'Tibullus','phi0684':'Varro','phi0690':'Virgil','phi0836':'Celsus','phi0845':'Columella','phi0860':'Curtius','phi0893':'Horace','phi0914':'Livy','phi0917':'Lucan','phi0959':'Ovid','phi0969':'Persius','phi0972':'Petronius','phi0975':'Phaedrus','phi1002':'Quintilian','phi1014':'Seneca the Elder','phi1017':'Seneca','phi1020':'Statius','phi1035':'Valerius Flaccus','phi1038':'Valerius Maximus','phi1056':'Vitruvius','phi1103':'Priapea','phi1212':'Apuleius','phi1242':'Florus','phi1254':'Gellius','phi1276':'Juvenal','phi1294':'Martial','phi1318':'Pliny the Younger','phi1345':'Silius','phi1348':'Suetonius','phi1351':'Tacitus','stoa0255':'Seneca (Dialogi)','phi0550':'Lucretius'}
def is_verse(u):
    a, w = u.split('.')[:2]
    return a in VERSE or (a == 'phi1017' and int(w[3:]) <= 10)
def manifest():
    rows = list(csv.DictReader(open(f'{HERE}/manifest.tsv'), delimiter='\t'))
    return {r['urn']: r for r in rows}
def work_sets():
    """Return dict set-name -> list of urns (longest Edition per Work)."""
    m = manifest()
    urns = [l.strip() for l in open(f'{HERE}/urns.txt') if l.strip()]
    best = {}
    for u in urns:
        k = '.'.join(u.split('.')[:2])
        if k not in best or int(m[u]['words']) > int(m[best[k]]['words']): best[k] = u
    cls = {u: m[u]['class'] for u in best.values()}
    core = [u for u in best.values() if cls[u] == 'core']
    bord = [u for u in best.values() if cls[u] == 'borderline']
    W = lambda u: int(m[u]['words'])
    s = {}
    s['prose80'] = sorted(u for u in core if not is_verse(u) and W(u) >= 5000)
    s['prose_all'] = sorted(u for u in core if not is_verse(u))
    s['verse61'] = sorted(u for u in core if is_verse(u) and W(u) >= 5000)
    s['verse_all'] = sorted(u for u in core if is_verse(u))
    s['borderline'] = sorted(bord)
    return s
def read_conllu(path):
    """yield (form, lemma, pos, feats, deprel, sent_index)"""
    si = 0
    for line in open(path, encoding='utf8'):
        if line.startswith('#'): continue
        line = line.rstrip('\n')
        if not line: si += 1; continue
        c = line.split('\t')
        if len(c) < 10: continue
        yield c[1], c[2], c[3], c[5], c[7], si
def nfd_strip(s):
    s = s.replace('у', 'y')  # Cyrillic u used in L&S for Greek upsilon
    s = unicodedata.normalize('NFD', s)
    s = ''.join(c for c in s if not unicodedata.combining(c))
    return unicodedata.normalize('NFC', s.replace('œ', 'oe').replace('Œ', 'Oe').replace('æ', 'ae').replace('Æ', 'Ae'))
