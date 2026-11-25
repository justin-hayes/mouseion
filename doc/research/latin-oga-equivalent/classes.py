"""Author classes for the Latin OGA-equivalent research (ticket: OGA equivalent for classical Latin).

Working definition (map #2658): classical = authors through roughly the 2nd
century AD. Classes:
  core        - Republic through the 1st century AD, plus Suetonius/Florus (c. 120)
  borderline  - 2nd-century or undated authors we cannot place with confidence
  out         - 3rd century and later (late, technical-late, commentary), or not a Work
IDs are Perseus/CTS author IDs (phi*, stoa*, ...). Anything not listed is "out".
"""

CORE = {
    "phi0119": "Plautus", "phi0134": "Terence", "phi0448": "Caesar",
    "phi0472": "Catullus", "phi0474": "Cicero", "phi0478": "Q. Cicero",
    "phi0550": "Lucretius", "phi0588": "Nepos", "phi0620": "Propertius",
    "phi0631": "Sallust", "phi0660": "Tibullus", "phi0684": "Varro",
    "phi0690": "Virgil", "phi0692": "Appendix Vergiliana",
    "phi0830": "Calpurnius Siculus", "phi0836": "Celsus",
    "phi0845": "Columella", "phi0860": "Curtius Rufus",
    "phi0893": "Horace", "phi0911": "Laus Pisonis", "phi0914": "Livy",
    "phi0917": "Lucan", "phi0959": "Ovid", "phi0969": "Persius",
    "phi0972": "Petronius", "phi0975": "Phaedrus", "phi0978": "Pliny the Elder",
    "phi1002": "Quintilian", "phi1011": "Scribonius Largus",
    "phi1014": "Seneca the Elder", "phi1017": "Seneca", "phi1020": "Statius",
    "phi1035": "Valerius Flaccus", "phi1038": "Valerius Maximus",
    "phi1056": "Vitruvius", "phi1103": "Priapea", "phi1221": "Augustus",
    "phi1242": "Florus", "phi1276": "Juvenal", "phi1294": "Martial",
    "phi1318": "Pliny the Younger", "phi1345": "Silius Italicus",
    "phi1348": "Suetonius", "phi1351": "Tacitus",
    # classical authors that Perseus files under Stoa identifiers
    "stoa0255": "Seneca (Dialogi)", "stoa0289": "Velleius Paterculus",
    "stoa0200": "Pomponius Mela",
}

BORDERLINE = {
    "phi1212": "Apuleius",   # b. c.125; Apologia/Florida/Metamorphoses are c.158-170 AD
    "phi1254": "Gellius",    # Noctes Atticae, c.170-180 AD
    "phi1251": "Gaius",      # Institutes, c.161 AD
    "phi1263": "Hyginus (Fabulae)",  # attribution and date uncertain
    "stoa0072": "Calpurnius Flaccus",  # declamations, 2nd c.
    "stoa0024": "Lucius Ampelius",     # Liber memorialis, 2nd-4th c.
}

OUT_NAMES = {
    "phi2331": "Historia Augusta (4th c.)", "phi2349": "Servius (c.400)",
    "phi1512": "Porphyrio (3rd c.)", "phi2003": "Apicius (late)",
}


def author_of(urn):
    """'phi0448.phi001.perseus-lat2' -> 'phi0448'."""
    return urn.split(".")[0]


def klass(urn):
    a = author_of(urn)
    if a in CORE:
        return "core"
    if a in BORDERLINE:
        return "borderline"
    return "out"
