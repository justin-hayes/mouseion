"""Approximate Beta Code -> Unicode Greek for Perseus LSJ/Middle Liddell keys.

Scope is limited to what the Perseus keys use: letters, ) ( / \\ = + | and * for
capitals. Trailing digits (homograph numbers) are returned separately.
Unicode is NFC-composed. Anything unrecognised is reported, not guessed.
"""
import re, unicodedata

LET = {'a':'α','b':'β','g':'γ','d':'δ','e':'ε','z':'ζ','h':'η','q':'θ','i':'ι','k':'κ',
       'l':'λ','m':'μ','n':'ν','c':'ξ','o':'ο','p':'π','r':'ρ','s':'σ','t':'τ','u':'υ',
       'f':'φ','x':'χ','y':'ψ','w':'ω','v':'ϝ'}
MARK = {')':'\u0313','(':'\u0314','/':'\u0301','\\':'\u0300','=':'\u0342','+':'\u0308','|':'\u0345'}
TRAIL = re.compile(r'(\d+)$')

def beta_to_greek(s):
    """Return (greek, homograph_number_or_None, unknown_chars)."""
    num = None
    m = TRAIL.search(s)
    if m:
        num = int(m.group(1)); s = s[:m.start()]
    out, unknown, cap, pending = [], [], False, ''
    for ch in s:
        if ch == '*':
            cap = True; continue
        if ch in MARK:
            # Marks normally follow their letter. After a capital marker they
            # precede it ("*)aqh"), so they wait for the next letter.
            if out and not cap:
                out[-1] = out[-1] + MARK[ch]
            else:
                pending += MARK[ch]
            continue
        base = LET.get(ch.lower())
        if base is None:
            unknown.append(ch); continue
        if cap or ch.isupper():
            base = base.upper() if base != 'σ' else 'Σ'
            cap = False
        out.append(base + pending); pending = ''
    # NFC composes base+marks; keep a stable order
    return unicodedata.normalize('NFC', ''.join(out)), num, unknown

def canonical(greek):
    """ancient-greek/1 as recorded in ADR 0099: NFC, lowercase, final sigma folded, NFC."""
    x = unicodedata.normalize('NFC', greek).lower()
    x = x.replace('ς', 'σ')
    return unicodedata.normalize('NFC', x)
