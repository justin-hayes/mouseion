from __future__ import annotations

import gzip
import hashlib
import importlib.util
import io
import json
import sqlite3
import stat
import sys
from datetime import date
from pathlib import Path
from xml.etree import ElementTree

import pytest


def load_script():
    path = Path(__file__).parents[1] / "scripts" / "derive_dictionary_index.py"
    spec = importlib.util.spec_from_file_location("derive_dictionary_index", path)
    assert spec
    assert spec.loader
    module = importlib.util.module_from_spec(spec)
    # Register before exec: dataclasses resolves `from __future__ import
    # annotations` string annotations through sys.modules[cls.__module__].
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def test_fixture_derives_filtered_entries_and_metadata(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl",
        output,
        "dump-2026-09-14",
    )

    connection = sqlite3.connect(output)
    assert connection.execute(
        "SELECT value FROM metadata WHERE key = 'provider_version'"
    ).fetchone() == ("dump-2026-09-14",)
    assert connection.execute("SELECT value FROM metadata WHERE key = 'license'").fetchone() == (
        "Wiktionary-derived data: CC BY-SA 4.0 / GFDL",
    )
    assert connection.execute(
        "SELECT value FROM metadata WHERE key = 'attribution'"
    ).fetchone() == ("Wiktionary contributors; CC BY-SA 4.0 / GFDL",)
    assert connection.execute(
        "SELECT value FROM metadata WHERE key = 'index_format'"
    ).fetchone() == ("5",)
    row = connection.execute(
        "SELECT language, lemma, upos, senses_json, gender, article, plural FROM entries WHERE lemma = 'haus'"
    ).fetchone()
    assert row[:3] == ("de", "haus", "NOUN")
    assert json.loads(row[3])[0]["Gloss"] == "house"
    assert row[4:] == ("Neut", "das", "Häuser")
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (7,)
    assert connection.execute(
        "SELECT count(*) FROM entries WHERE lemma = 'unbekannt'"
    ).fetchone() == (0,)
    assert connection.execute("SELECT ipa FROM entries WHERE lemma = 'albero'").fetchone() == ("",)
    assert set(connection.execute("SELECT DISTINCT language FROM entries").fetchall()) == {
        ("de",),
        ("it",),
    }
    for (senses_json,) in connection.execute("SELECT senses_json FROM entries"):
        senses = json.loads(senses_json)
        assert senses
        assert all(sense["Gloss"].strip() for sense in senses)
    assert connection.execute(
        "SELECT upos, plural, senses_json FROM entries WHERE lemma IN ('aufstehen', 'gut') ORDER BY lemma"
    ).fetchall() == [
        (
            "VERB",
            "",
            '[{"Gloss":"to get up","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"","Article":"","Plural":"","IPA":""}]',
        ),
        (
            "ADJ",
            "",
            '[{"Gloss":"good","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"","Article":"","Plural":"","IPA":""}]',
        ),
    ]
    italian = connection.execute(
        "SELECT lemma, gender, article, plural FROM entries WHERE language = 'it' ORDER BY lemma"
    ).fetchall()
    assert italian == [
        ("albero", "Masc", "l'", "alberi"),
        ("casa", "Fem", "la", "case"),
        ("libro", "Masc", "il", "libri"),
        ("zaino", "Masc", "lo", "zaini"),
    ]
    connection.close()


def test_gzipped_dump_is_a_supported_input(tmp_path: Path):
    module = load_script()
    source = Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl"
    compressed = tmp_path / "kaikki.json.gz"
    with gzip.open(compressed, "wt", encoding="utf-8") as output:
        output.write(source.read_text(encoding="utf-8"))

    index = tmp_path / "dictionary.sqlite"
    module.derive(compressed, index, "dump-2026-09-14")

    connection = sqlite3.connect(index)
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (7,)
    connection.close()


def test_definite_plural_forms_fall_back_to_noun_gender(tmp_path: Path):
    module = load_script()
    source = tmp_path / "articles.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(
                {
                    "word": word,
                    "lang_code": "de",
                    "pos": "noun",
                    "tags": [gender],
                    "senses": [{"glosses": [word]}],
                    "forms": [{"form": form, "tags": ["definite", "nominative", "plural"]}],
                }
            )
            for word, gender, form in [
                ("Gauner", "masculine", "Gauner"),
                ("Feigheit", "feminine", "Feigheiten"),
            ]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT lemma, article FROM entries ORDER BY lemma").fetchall() == [
        ("feigheit", "die"),
        ("gauner", "der"),
    ]
    connection.close()


def test_ambiguous_gender_does_not_derive_an_article(tmp_path: Path):
    module = load_script()
    source = tmp_path / "ambiguous.jsonl"
    source.write_text(
        json.dumps(
            {
                "word": "λέξη",
                "lang_code": "el",
                "pos": "noun",
                "tags": ["masculine", "feminine"],
                "senses": [{"glosses": ["word"]}],
            },
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT gender, article FROM entries").fetchone() == ("", "")
    connection.close()


def test_valid_nominative_article_wins_across_colliding_source_records(tmp_path: Path):
    module = load_script()
    source = tmp_path / "colliding.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(item, ensure_ascii=False)
            for item in [
                {
                    "word": "ΟΔΌΣ",
                    "lang_code": "el",
                    "pos": "noun",
                    "tags": ["masculine"],
                    "senses": [{"glosses": ["road"]}],
                },
                {
                    "word": "οδός",
                    "lang_code": "el",
                    "pos": "noun",
                    "tags": ["feminine"],
                    "senses": [{"glosses": ["way"]}],
                    "forms": [{"form": "η", "tags": ["definite", "nominative", "singular"]}],
                },
            ]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT gender, article FROM entries").fetchone() == ("", "η")
    connection.close()


def test_captured_kaikki_forms_fall_back_to_noun_gender(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        Path(__file__).parents[1] / "testdata" / "dictionary_article_forms.jsonl",
        output,
        "fixture-v1",
    )

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT lemma, article FROM entries ORDER BY lemma").fetchall() == [
        ("feigheit", "die"),
        ("gauner", "der"),
    ]
    connection.close()


def test_captured_kaikki_ipa_is_normalized(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        Path(__file__).parents[1] / "testdata" / "dictionary_article_forms.jsonl",
        output,
        "fixture-v1",
    )

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT ipa FROM entries WHERE lemma = 'feigheit'").fetchone() == (
        "/ˈfaɪ̯kaɪ̯t/",
    )
    assert connection.execute("SELECT ipa FROM entries WHERE lemma = 'gauner'").fetchone() == (
        "/ˈɡaʊ̯nər/",
    )
    connection.close()


def test_derivation_normalizes_ipa_and_extracts_principal_parts(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        Path(__file__).parents[1] / "testdata" / "dictionary_form_presentation.jsonl",
        output,
        "fixture-v1",
    )

    connection = sqlite3.connect(output)
    rows = {
        lemma: (ipa, principal_parts)
        for lemma, ipa, principal_parts in connection.execute(
            "SELECT lemma, ipa, principal_parts FROM entries"
        )
    }
    gehen_senses = json.loads(
        connection.execute("SELECT senses_json FROM entries WHERE lemma = 'gehen'").fetchone()[0]
    )
    connection.close()
    assert rows == {
        "gehen": ("/ˈɡeːən/", "geht · ging · gegangen"),
        "regnen": ("", ""),
        "wasser": ("/ˈvasɐ/", ""),
        "föhn": ("", ""),
        "andare": ("/anˈda.re/", ""),
    }
    assert gehen_senses[0]["IPA"] == "/ˈɡeːən/"


def test_downloader_uses_mouseion_cache_without_external_dependency(monkeypatch, tmp_path: Path):
    module = load_script()
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path))

    assert module.download_cache_path() == tmp_path / "mouseion" / "raw-wiktextract-data.jsonl.gz"


def test_derived_index_is_readable_by_the_nonroot_container(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl",
        output,
        "dump-2026-09-14",
    )

    assert stat.S_IMODE(output.stat().st_mode) == 0o644


def test_german_fixture_keys_match_runtime_lookup_keys(tmp_path: Path):
    module = load_script()
    fixture = Path(__file__).parents[1] / "testdata" / "german_normalization_parity.jsonl"
    output = tmp_path / "dictionary.sqlite"
    module.derive(fixture, output, "dump-2026-09-14")

    expected = {}
    for line in fixture.read_text(encoding="utf-8").splitlines():
        item = json.loads(line)
        expected[item["word"]] = item["runtime_lookup_key"]
        assert module.normalize("de", item["word"]) == item["runtime_lookup_key"]

    connection = sqlite3.connect(output)
    actual = dict(connection.execute("SELECT lemma, senses_json FROM entries").fetchall())
    connection.close()
    assert set(actual) == set(expected.values())


def test_french_normalization_matches_the_committed_parity_fixture():
    module = load_script()
    fixture = Path(__file__).parents[1] / "testdata" / "french_normalization_parity.jsonl"

    rows = 0
    for line in fixture.read_text(encoding="utf-8").splitlines():
        item = json.loads(line)
        rows += 1
        assert module.normalize("fr", item["raw"]) == item["canonical"]
    assert rows > 0
    # The same profile Go's runtime lookup applies, so a curly-apostrophe
    # lemma finds the ASCII-apostrophe entry.
    assert module.normalize("fr", "aujourd\u2019hui") == "aujourd'hui"


def test_french_fixture_derives_gender_h_status_plural_and_ipa(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        Path(__file__).parents[1] / "testdata" / "dictionary_french_fixture.jsonl",
        output,
        "dump-2026-09-02",
    )

    connection = sqlite3.connect(output)
    rows = {
        (lemma, upos): (gender, plural, ipa, h_status, principal_parts)
        for lemma, upos, gender, plural, ipa, h_status, principal_parts in connection.execute(
            "SELECT lemma, upos, gender, plural, ipa, h_status, principal_parts FROM entries"
        )
    }
    connection.close()

    # Aspirated-h evidence is recorded, and the noun keeps its regular plural.
    assert rows[("hache", "NOUN")] == ("Fem", "haches", "/aʃ/", "aspirated", "")
    # A missing aspirated-h category is unknown, never mute; the head template
    # marks the noun invariable.
    assert rows[("héros", "NOUN")] == ("Masc", "invariable", "/e.ʁo/", "", "")
    assert rows[("prix", "NOUN")] == ("Masc", "invariable", "/pʁi/", "", "")
    # The optional schwa is preserved verbatim.
    assert rows[("souvenir", "NOUN")][2] == "/su.v(ə).niʁ/"
    # IPA tagged only with a non-European region is omitted.
    assert rows[("char", "NOUN")][2] == ""
    # A transcription carrying France among other regional tags is kept.
    assert rows[("timbre", "NOUN")][2] == "/tɛ̃bʁ/"
    # France and Europe tags are accepted; the first accepted transcription wins.
    assert rows[("café", "NOUN")][2] == "/ka.fe/"
    # No French principal parts, even where a German-shaped head expansion
    # would have matched.
    assert rows[("aller", "VERB")][4] == ""
    assert rows[("aller", "VERB")][2] == ""
    # Gender and aspirated-h evidence are recorded on NOUN rows only.
    assert rows[("rapide", "ADJ")][0] == ""
    assert rows[("héroïque", "ADJ")][3] == ""
    # The canonical lemma folds the typographic apostrophe.
    assert ("aujourd'hui", "ADV") in rows


def test_french_invariable_reads_head_template_expansions():
    module = load_script()

    # Non-mapping templates are skipped, and only a whole-word "invariable"
    # marker counts.
    assert module.french_invariable(
        {"head_templates": ["not a mapping", {"expansion": "prix m (invariable)"}]}
    )
    assert module.french_invariable(
        {"head_templates": [{"expansion": "héros m (invariable, feminine héroïne)"}]}
    )
    assert not module.french_invariable(
        {"head_templates": [{"expansion": "hache f (plural haches)"}]}
    )
    assert not module.french_invariable({"head_templates": [{"expansion": "invariables"}]})


def test_french_derivation_records_form_of_links_and_headwords(tmp_path: Path):
    module = load_script()
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(item, ensure_ascii=False)
            for item in [
                # Form-of-only entry: indexed for lookup, but not a headword.
                {
                    "word": "voiles",
                    "lang_code": "fr",
                    "pos": "noun",
                    "senses": [
                        {
                            "glosses": ["plural of voile"],
                            "tags": ["form-of", "plural"],
                            "form_of": [{"word": "voile"}],
                        },
                        # A repeated link and an empty target do not add rows.
                        {
                            "glosses": ["plural of voile"],
                            "tags": ["form-of", "plural"],
                            "form_of": [{"word": "voile"}],
                        },
                        {
                            "glosses": ["empty form-of target"],
                            "tags": ["form-of"],
                            "form_of": [{"word": "  "}],
                        },
                    ],
                },
                # Non-mapping and gloss-less senses are skipped, and an empty
                # word never becomes an index key.
                {
                    "word": "sensesanormal",
                    "lang_code": "fr",
                    "pos": "noun",
                    "senses": ["not a mapping"],
                },
                {
                    "word": "sansglose",
                    "lang_code": "fr",
                    "pos": "noun",
                    "senses": [{"form_of": [{"word": "rien"}]}],
                },
                {
                    "word": "",
                    "lang_code": "fr",
                    "pos": "noun",
                    "senses": [{"glosses": ["empty"]}],
                },
                {
                    "word": "voile",
                    "lang_code": "fr",
                    "pos": "noun",
                    "senses": [{"glosses": ["sail"]}],
                },
                # A mixed entry has an ordinary sense, so it is a headword even
                # though another sense is a form-of link.
                {
                    "word": "portes",
                    "lang_code": "fr",
                    "pos": "noun",
                    "senses": [
                        {
                            "glosses": ["plural of porte"],
                            "tags": ["form-of", "plural"],
                            "form_of": [{"word": "porte"}],
                        },
                        {"glosses": ["doors"]},
                    ],
                },
                {
                    "word": "porte",
                    "lang_code": "fr",
                    "pos": "noun",
                    "senses": [{"glosses": ["door"]}],
                },
                # Participle tags are recorded so a participial adjective can be
                # treated as the analyzed verb's own form.
                {
                    "word": "mouillé",
                    "lang_code": "fr",
                    "pos": "verb",
                    "senses": [
                        {
                            "glosses": ["past participle of mouiller"],
                            "tags": ["form-of", "participle", "past"],
                            "form_of": [{"word": "mouiller"}],
                        }
                    ],
                },
                {
                    "word": "mouiller",
                    "lang_code": "fr",
                    "pos": "verb",
                    "senses": [{"glosses": ["to wet"]}],
                },
                {
                    "word": "tapes",
                    "lang_code": "fr",
                    "pos": "verb",
                    "senses": [
                        {
                            "glosses": ["second-person singular present of taper"],
                            "tags": ["form-of"],
                            "form_of": [{"word": "taper"}],
                        }
                    ],
                },
                {
                    "word": "taper",
                    "lang_code": "fr",
                    "pos": "verb",
                    "senses": [{"glosses": ["to slap"]}],
                },
            ]
        )
        + "\n\n",
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"

    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute(
        "SELECT value FROM metadata WHERE key = 'index_format'"
    ).fetchone() == ("5",)
    headwords = set(connection.execute("SELECT lemma, upos FROM french_headwords").fetchall())
    assert headwords == {
        ("voile", "NOUN"),
        ("porte", "NOUN"),
        ("portes", "NOUN"),
        ("mouiller", "VERB"),
        ("taper", "VERB"),
    }
    form_rows = connection.execute(
        "SELECT form, upos, target, participle, evidence_id FROM french_forms ORDER BY form"
    ).fetchall()
    connection.close()

    assert [
        (form, upos, target, participle) for form, upos, target, participle, _ in form_rows
    ] == [
        ("mouillé", "VERB", "mouiller", 1),
        ("portes", "NOUN", "porte", 0),
        ("tapes", "VERB", "taper", 0),
        ("voiles", "NOUN", "voile", 0),
    ]
    assert all(evidence_id.startswith("wiktionary-form-of:") for *_, evidence_id in form_rows)


def test_sound_ipa_skips_unusable_sounds():
    module = load_script()

    assert module.sound_ipa("not a mapping", "fr") == ""
    assert module.sound_ipa({"ipa": "   "}, "fr") == ""
    assert module.sound_ipa({"ipa": "/a/", "note": "before a vowel"}, "fr") == ""
    assert module.sound_ipa({"ipa": "/haʃ/", "tags": ["Louisiana"]}, "fr") == ""
    assert module.sound_ipa({"ipa": "/su.v(ə).niʁ/"}, "fr") == "/su.v(ə).niʁ/"
    # Non-French languages still strip optional segments.
    assert module.sound_ipa({"ipa": "/ˈɡeː(h)ən/", "tags": ["standard"]}, "de") == "/ˈɡeːən/"


def test_derivation_records_dump_extraction_and_source_hash(tmp_path: Path):
    module = load_script()
    source = Path(__file__).parents[1] / "testdata" / "dictionary_french_fixture.jsonl"
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        source,
        output,
        "dump=2026-09-02;extraction=2026-10-03",
        module.SourceMetadata(dump_date="2026-09-02", extraction_date="2026-10-03"),
    )

    connection = sqlite3.connect(output)
    provenance = dict(connection.execute("SELECT key, value FROM metadata").fetchall())
    connection.close()
    assert provenance["dump_date"] == "2026-09-02"
    assert provenance["extraction_date"] == "2026-10-03"
    assert provenance["source_sha256"] == hashlib.sha256(source.read_bytes()).hexdigest()


def test_modern_greek_keys_use_nfc_casefold_normalization(tmp_path: Path):
    module = load_script()
    source = tmp_path / "greek.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(
                {
                    "word": word,
                    "lang_code": "el",
                    "pos": "noun",
                    "senses": [{"glosses": [word]}],
                },
                ensure_ascii=False,
            )
            for word in ["ΟΔΟΣ", "οδός", "που\u0301", "που"]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    actual = {lemma for (lemma,) in connection.execute("SELECT lemma FROM entries ORDER BY lemma")}
    connection.close()
    assert actual == {"οδοσ", "οδόσ", "πού", "που"}


def test_modern_greek_fixture_preserves_dictionary_morphology(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(
        Path(__file__).parents[1] / "testdata" / "dictionary_greek_fixture.jsonl",
        output,
        "fixture-v1",
    )

    connection = sqlite3.connect(output)
    rows = {
        lemma: (upos, json.loads(senses), gender, article, plural, ipa, principal_parts)
        for lemma, upos, senses, gender, article, plural, ipa, principal_parts in connection.execute(
            "SELECT lemma, upos, senses_json, gender, article, plural, ipa, principal_parts FROM entries"
        )
    }
    connection.close()

    assert rows["άνθρωποσ"] == (
        "NOUN",
        [
            {
                "Gloss": "person",
                "Examples": [],
                "Topics": [],
                "Tags": ["masculine"],
                "Phrase": "",
                "Gender": "Masc",
                "Article": "ο",
                "Plural": "άνθρωποι",
                "IPA": "/ˈanθropos/",
            }
        ],
        "Masc",
        "ο",
        "άνθρωποι",
        "/ˈanθropos/",
        "",
    )
    assert rows["πόλη"][:5] == (
        "NOUN",
        [
            {
                "Gloss": "city",
                "Examples": [],
                "Topics": [],
                "Tags": ["feminine"],
                "Phrase": "",
                "Gender": "Fem",
                "Article": "η",
                "Plural": "πόλεις",
                "IPA": "",
            }
        ],
        "Fem",
        "η",
        "πόλεις",
    )
    assert rows["σπίτι"][:5] == (
        "NOUN",
        [
            {
                "Gloss": "house",
                "Examples": [],
                "Topics": [],
                "Tags": ["neuter"],
                "Phrase": "",
                "Gender": "Neut",
                "Article": "το",
                "Plural": "σπίτια",
                "IPA": "/ˈspi.ti/",
            }
        ],
        "Neut",
        "το",
        "σπίτια",
    )
    assert rows["δρόμοσ"][3] == "ο"
    assert rows["είμαι"][-1] == ""


def test_remaining_analyzer_upos_tags_are_retained(tmp_path: Path):
    module = load_script()
    source = tmp_path / "pos.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(
                {
                    "word": word,
                    "lang_code": "de",
                    "pos": pos,
                    "senses": [{"glosses": [word], "tags": tags}],
                }
            )
            for word, pos, tags in [
                ("obwohl", "conj", []),
                ("dass", "conj", ["subordinating"]),
                ("und", "conj", ["coordinating"]),
                ("dies", "det", []),
                ("zwei", "num", []),
                ("ach", "intj", []),
            ]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert set(connection.execute("SELECT lemma, upos FROM entries").fetchall()) == {
        ("obwohl", "CCONJ"),
        ("obwohl", "SCONJ"),
        ("dass", "SCONJ"),
        ("und", "CCONJ"),
        ("dies", "DET"),
        ("zwei", "NUM"),
        ("ach", "INTJ"),
    }
    connection.close()


def test_meanings_without_mapped_pos_are_retained_without_inventing_pos(tmp_path: Path):
    module = load_script()
    source = tmp_path / "missing-pos.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(
                {"word": word, "lang_code": language, "pos": pos, "senses": [{"glosses": [gloss]}]}
            )
            for word, language, pos, gloss in [
                ("haus", "de", "unmapped-pos", "house"),
                ("casa", "it", "", "house"),
                ("σπίτι", "el", "unmapped-pos", "house"),
            ]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute(
        "SELECT language, lemma, upos FROM entries ORDER BY language"
    ).fetchall() == [
        ("de", "haus", ""),
        ("el", "σπίτι", ""),
        ("it", "casa", ""),
    ]
    assert [
        json.loads(row[0])[0]["Gloss"]
        for row in connection.execute("SELECT senses_json FROM entries ORDER BY language")
    ] == [
        "house",
        "house",
        "house",
    ]
    connection.close()


def test_download_input_fetches_and_caches_the_dump(monkeypatch, tmp_path: Path):
    module = load_script()
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path))
    payload = (
        b'{"word": "haus", "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["house"]}]}\n'
    )
    monkeypatch.setattr(module, "urlopen", lambda url: io.BytesIO(payload))

    path = module.download_input(force=True)

    assert path == tmp_path / "mouseion" / "raw-wiktextract-data.jsonl.gz"
    assert path.read_bytes() == payload


def test_download_input_reuses_the_cached_dump(monkeypatch, tmp_path: Path):
    module = load_script()
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path))
    cached = tmp_path / "mouseion" / "raw-wiktextract-data.jsonl.gz"
    cached.parent.mkdir(parents=True)
    cached.write_bytes(b"cached")

    def unexpected_download(url):
        raise AssertionError("cached input must not be downloaded again")

    monkeypatch.setattr(module, "urlopen", unexpected_download)

    assert module.download_input(force=False) == cached


def test_download_input_rejects_a_non_http_scheme(monkeypatch, tmp_path: Path):
    module = load_script()
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path))
    monkeypatch.setattr(module, "KAIKKI_DOWNLOAD_URL", "file:///etc/passwd")

    with pytest.raises(RuntimeError, match="non-http"):
        module.download_input(force=True)


def test_non_dict_senses_are_skipped(tmp_path: Path):
    module = load_script()
    source = tmp_path / "senses.jsonl"
    source.write_text(
        json.dumps(
            {
                "word": "haus",
                "lang_code": "de",
                "pos": "noun",
                "senses": ["not a mapping"],
            }
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"

    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (0,)
    connection.close()


def test_main_derives_the_index_from_arguments(tmp_path: Path):
    module = load_script()
    source = Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl"
    output = tmp_path / "dictionary.sqlite"

    assert module.main(["--input", str(source), "--output", str(output)]) == 0

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (7,)
    assert connection.execute(
        "SELECT value FROM metadata WHERE key = 'provider_version'"
    ).fetchone() == (
        "dump=unknown;extraction=" + date.today().isoformat() + ";wiktextract=unknown",
    )
    connection.close()


def test_entries_without_a_usable_word_are_skipped(tmp_path: Path):
    module = load_script()
    source = tmp_path / "words.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(item)
            for item in [
                {"word": "", "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["x"]}]},
                {"word": 5, "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["x"]}]},
            ]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"

    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (0,)
    assert connection.execute("SELECT count(*) FROM greek_entries").fetchone() == (0,)
    connection.close()


def test_ancient_greek_entries_keep_source_entries_separate(tmp_path: Path):
    module = load_script()
    source = tmp_path / "greek.jsonl"
    decomposed = "δε\u0301ω"  # NFC is δέω, so the two δέω entries must not merge
    source.write_text(
        "\n".join(
            json.dumps(item, ensure_ascii=False)
            for item in [
                {
                    "word": "δέω",
                    "lang_code": "grc",
                    "pos": "verb",
                    "etymology_number": 1,
                    "senses": [{"glosses": ["to bind"]}],
                    "forms": [{"form": "δῶ", "tags": ["future"]}],
                },
                {
                    "word": decomposed,
                    "lang_code": "grc",
                    "pos": "verb",
                    "etymology_number": 2,
                    "senses": [{"glosses": ["to lack"]}],
                    "forms": [{"form": "δεη\u0301σω", "tags": ["future"]}],
                },
                {
                    "word": "ἄνθρωπος",
                    "lang_code": "grc",
                    "pos": "noun",
                    "tags": ["masculine"],
                    "senses": [{"glosses": ["human being", "person"]}],
                },
                {
                    "word": "Haus",
                    "lang_code": "de",
                    "pos": "noun",
                    "senses": [{"glosses": ["house"]}],
                },
            ]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"

    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    greek = connection.execute(
        "SELECT headword, upos, etymology_number, senses_json, forms_json "
        "FROM greek_entries ORDER BY headword, etymology_number"
    ).fetchall()
    modern = connection.execute("SELECT language, lemma, upos FROM entries").fetchall()
    connection.close()

    # Modern-language rows are untouched; the grc entries are not in them.
    assert modern == [("de", "haus", "NOUN")]
    assert [(row[0], row[1], row[2]) for row in greek] == [
        ("δέω", "VERB", "1"),
        ("δέω", "VERB", "2"),
        ("ἄνθρωπος", "NOUN", ""),
    ]
    # The headword and forms are NFC, and the homographs keep their own senses.
    assert json.loads(greek[0][3])[0]["Gloss"] == "to bind"
    assert json.loads(greek[1][3])[0]["Gloss"] == "to lack"
    assert json.loads(greek[2][3])[0]["Gloss"] == "human being"
    assert json.loads(greek[0][4]) == ["δῶ"]
    assert json.loads(greek[1][4]) == ["δεήσω"]


SYNTHETIC_LSJ = """<?xml version="1.0" encoding="UTF-8"?>
<TEI.2>
<text><body>
<entryFree id="n1" key="lo/gos" type="main">
  <orth>lo/gos</orth>
  <sense id="n1.0" n="A" level="1"><tr>word</tr>, <cit><quote>e)n a)rxh=|</quote>
    <bibl n="urn:cts:greekLit:tlg0032.tlg006.perseus-grc1:1:1:1" default="NO">
      <author>X.</author><title>An.</title><biblScope>1.1.1</biblScope></bibl></cit>
    <sense id="n1.0.1" n="2" level="3"><tr>account</tr></sense></sense>
  <sense id="n1.1" n="B" level="1"><tr>speech</tr>, <cit><quote>y</quote>
    <bibl n="urn:cts:greekLit:tlg0012.tlg001.perseus-grc1:1:1" default="NO">
      <author>Il.</author><biblScope>1.1</biblScope></bibl></cit></sense>
</entryFree>
<entryFree id="n2" key="de/w" type="main"><sense id="n2.0" n="A" level="1"><tr>bind</tr></sense></entryFree>
<entryFree id="n3" key="de/w1" type="main"><sense id="n3.0" n="A" level="1"><tr>lack</tr></sense></entryFree>
<entryFree id="n4" key="lo/gos2" type="main">
  <orth>lo/gos a)nh/r</orth>
  <sense id="n4.0" n="A" level="1"><tr>a man of words</tr></sense>
</entryFree>
<entryFree id="n5" key="de/w2" type="main">
  <orth>de/w</orth>
  <sense id="n5.0" n="A" level="1"><xr><lbl>v.</lbl><ref>dh/w</ref></xr></sense>
</entryFree>
<entryFree id="n6" key="a)/n1" type="main">
  <orth>a)/n</orth> (A),
  <sense id="n6.0" n="A" level="1"><tr>if</tr></sense>
</entryFree>
<entryFree id="n7" key="a)/n2" type="main">
  <orth>a)/n</orth> (B),
  <sense id="n7.0" n="A" level="1"><xr><lbl>=</lbl><ref>e)a/n</ref></xr></sense>
</entryFree>
</body></text>
</TEI.2>
"""


def test_lsj_entries_preserve_identity_hierarchy_and_xenophon_citations(tmp_path: Path):
    module = load_script()
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        json.dumps(
            {"word": "Haus", "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["house"]}]}
        ),
        encoding="utf-8",
    )
    lsj_dir = tmp_path / "lsj"
    lsj_dir.mkdir()
    (lsj_dir / "grc.lsj.perseus-eng1.xml").write_text(SYNTHETIC_LSJ, encoding="utf-8")
    output = tmp_path / "dictionary.sqlite"

    module.derive(
        source, output, "fixture-v1", lsj=module.LsjSource(directory=lsj_dir, pin="pin123")
    )

    connection = sqlite3.connect(output)
    assert connection.execute(
        "SELECT value FROM metadata WHERE key = 'index_format'"
    ).fetchone() == ("8",)
    assert connection.execute("SELECT value FROM metadata WHERE key = 'lsj_pin'").fetchone() == (
        "pin123",
    )
    rows = connection.execute(
        "SELECT entry_id, headword, senses_json, subordinate FROM lsj_entries ORDER BY entry_id"
    ).fetchall()
    connection.close()

    # Native entry ids are preserved and the homographs keep separate rows under
    # the same NFC headword (their trailing Beta Code digit is dropped).
    assert [(row[0], row[1]) for row in rows] == [
        ("n1", "λόγοσ"),
        ("n2", "δέω"),
        ("n3", "δέω"),
        ("n4", "λόγοσ"),
        ("n5", "δέω"),
        ("n6", "ἄν"),
        ("n7", "ἄν"),
    ]
    # A phrase sub-entry (n4) and a "v. ..." redirect stub (n5) are subordinate.
    # The real homographs, the bare entries, and the (A)/(B) pair (n6, n7) are
    # not, so the pair still reaches the lookup as two competing entries.
    assert {row[0]: row[3] for row in rows} == {
        "n1": 0,
        "n2": 0,
        "n3": 0,
        "n4": 1,
        "n5": 1,
        "n6": 0,
        "n7": 0,
    }

    senses = json.loads(rows[0][2])
    assert [sense["id"] for sense in senses] == ["n1.0", "n1.1"]
    assert senses[0]["n"] == "A"
    assert senses[0]["glosses"] == ["word"]
    assert senses[0]["definitions"] == ["word"]
    # Only the Xenophon citation is kept; the Homer one is dropped.
    assert senses[0]["citations"] == ["X. An. 1.1.1"]
    assert senses[1]["citations"] == []
    # The nested sense keeps its native id, level and gloss.
    assert senses[0]["senses"][0]["id"] == "n1.0.1"
    assert senses[0]["senses"][0]["level"] == 3
    assert senses[0]["senses"][0]["glosses"] == ["account"]


def test_lsj_vowel_length_marks_are_dropped_and_admitted():
    module = load_script()

    # A "^" or "_" quantity mark is dropped, and the key is admitted rather than
    # rejected as an unknown character.
    for key, expected in (
        ("o(/ti^", "ὅτι"),
        ("de/ka^", "δέκα"),
        ("pa/nu^", "πάνυ"),
        ("e)/nqa^", "ἔνθα"),
        ("*(re/a_", "Ῥέα"),
        ("*)/ogka_", "Ὄγκα"),
    ):
        headword, admitted = module.lsj_headword(key)
        assert admitted, key
        assert headword == expected, key


def test_lsj_subordinate_classifies_phrases_stubs_and_homographs():
    module = load_script()

    def entry(xml: str):
        # A literal synthetic fixture, not untrusted input.
        return ElementTree.fromstring(xml)  # noqa: S314

    phrase = entry(
        '<entryFree key="ou)2" type="main"><orth>ou) ga/r</orth>'
        '<sense id="x"><tr>for not</tr></sense></entryFree>'
    )
    assert module.lsj_subordinate(phrase), "a multi-word phrase sub-entry is subordinate"

    redirect = entry(
        '<entryFree key="h)/dh2" type="main"><orth>h)/dh</orth>'
        '<sense id="x"><xr><lbl>v.</lbl><ref>*ei)/dw</ref></xr></sense></entryFree>'
    )
    assert module.lsj_subordinate(redirect), "a cross-reference stub is subordinate"

    # A stub LSJ marks (A)/(B)/(C) is a real homograph, not a redirect.
    marked = entry(
        '<entryFree key="a)/n2" type="main"><orth>a)/n</orth> (B), '
        '<sense id="x"><xr><lbl>=</lbl><ref>e)a/n</ref></xr></sense></entryFree>'
    )
    assert not module.lsj_subordinate(marked)

    # A numbered entry with its own gloss is a real homograph.
    homograph = entry(
        '<entryFree key="de/w1" type="main"><orth>de/w</orth>'
        '<sense id="x"><tr>lack</tr></sense></entryFree>'
    )
    assert not module.lsj_subordinate(homograph)

    # The bare entry is never subordinate, even when it only redirects.
    bare = entry(
        '<entryFree key="ma=llon" type="main"><orth>ma=llon</orth>'
        '<sense id="x"><xr><lbl>v.</lbl><ref>ma/la</ref></xr></sense></entryFree>'
    )
    assert not module.lsj_subordinate(bare)


def test_lsj_glosses_skip_etymology_cognates_and_punctuation():
    module = load_script()
    # A literal synthetic fixture, not untrusted input.
    sense = ElementTree.fromstring(  # noqa: S314
        '<sense id="n4.0" n="4" level="2">of things, <tr>to be produced</tr>, '
        '<tr>;</tr> <foreign lang="greek">ta\\ h(me/ras</foreign> Lat. stuff '
        "<tr>dues,</tr> and <tr> it is done, </tr> "
        "(Cf. Skt. <tr>jánati</tr> 'procreate', <tr>jánas</tr> ( = ge/nos ), "
        "Lat. <tr>gigno, gnatus.</tr>) then <tr>come to pass</tr>; "
        "= Lat. <tr>decuria</tr> (Cf. <tr>vasnás</tr> 'price')</sense>"
    )

    derived = module.lsj_sense(sense)

    # Cognates (after a language abbreviation, or inside a parenthesis that
    # names one) are not English glosses; punctuation is trimmed and a
    # punctuation-only gloss is dropped. Source order is kept.
    assert derived["glosses"] == ["to be produced", "dues", "it is done", "come to pass"]


def test_lsj_glosses_skip_the_remaining_cognate_languages():
    module = load_script()
    # A literal synthetic fixture shaped like the pinned LSJ's etymology notes
    # ("(Cf. Skt. ..., Lat. ...)", "(Egypt. ...)"), not untrusted input.
    sense = ElementTree.fromstring(  # noqa: S314
        '<sense id="n6.0" n="1" level="2">(Cf. Skt. <tr>jánati</tr>, Avest. <tr>ayar</tr>) '
        "Hebr. <tr>šālôm</tr> but <tr>peace</tr>, Accad. <tr>kitinnu</tr>, "
        "Egypt. <tr>kemai</tr>, (Egypt. pwr b <tr>p;</tr>), Assyr. <tr>agarru</tr>, "
        "Bab. <tr>buśtu</tr>, Sumer. <tr>balag</tr>, Aram. <tr>gerōphīth</tr>, "
        "Heb. <tr>shibboleth</tr>, Copt. <tr>stēm</tr>, Skr. <tr>hanus</tr>, "
        "Lett. <tr>glums</tr>, OPruss. <tr>seyr</tr>, OSlav. <tr>kryti</tr>, "
        "Serb. <tr>jedan put</tr>, Celt. <tr>benn</tr>, Ital. <tr>sagena</tr>, "
        "Ir. <tr>ulcha</tr>, Armen. <tr>kask</tr>, Phryg. <tr>hircus</tr>, "
        "<tr>come to pass</tr></sense>"
    )

    derived = module.lsj_sense(sense)

    # Foreign forms are not English glosses, whether they directly follow the
    # abbreviation or sit inside the parenthesis it names. The gloss after the
    # abbreviation list is kept in source order.
    assert derived["glosses"] == ["peace", "come to pass"]


def test_lsj_glosses_keep_prose_and_grammar_abbreviations_as_glosses():
    module = load_script()
    # "Egypt" as an ordinary place name, and abbreviations that introduce an
    # English gloss, never suppress it. "Lit." means literally here.
    sense = ElementTree.fromstring(  # noqa: S314
        '<sense id="n6.1" n="2" level="3">the entrance to Egypt <tr>with a store of</tr>, '
        "Adv. <tr>without pain</tr>, Subst. <tr>sinner</tr>, Comp. <tr>more natural</tr>, "
        "Lit. <tr>the people's property</tr></sense>"
    )

    derived = module.lsj_sense(sense)

    assert derived["glosses"] == [
        "with a store of",
        "without pain",
        "sinner",
        "more natural",
        "the people's property",
    ]


def test_lsj_definitions_are_the_glosses_before_the_first_citation():
    module = load_script()
    # A literal synthetic fixture, not untrusted input.
    sense = ElementTree.fromstring(  # noqa: S314
        '<sense id="n5.0" n="1" level="3">of persons, <tr>to be born</tr>, '
        '<foreign lang="greek">neo/s</foreign> new <tr>born</tr>, '
        '<bibl n="urn:cts:greekLit:tlg0012.tlg001.perseus-grc1:1:1">Il. 1.1</bibl>; '
        "<tr>living</tr> under Tmolus, "
        '<cit><quote lang="greek">x</quote><bibl n="urn:x">Hdt.</bibl></cit> '
        "<tr>at our birth</tr></sense>"
    )
    uncited = ElementTree.fromstring(  # noqa: S314
        '<sense id="n5.1" n="2" level="3"><tr>fall due</tr></sense>'
    )

    derived = module.lsj_sense(sense)

    # LSJ states a sense's definitions before citing it; later glosses
    # translate cited usages. All glosses are kept in source order.
    assert derived["definitions"] == ["to be born", "born"]
    assert derived["glosses"] == ["to be born", "born", "living", "at our birth"]
    assert module.lsj_sense(uncited)["definitions"] == ["fall due"]
    no_gloss = ElementTree.fromstring('<sense id="n5.2" n="3" level="3">x</sense>')  # noqa: S314
    assert "definitions" not in module.lsj_sense(no_gloss)
    # A forms paragraph cites its forms; ":—" then starts the meaning.
    forms = ElementTree.fromstring(  # noqa: S314
        '<sense id="n5.3" n="A" level="1">plpf. <cit><quote lang="greek">e)ge/neto</quote>'
        '<bibl n="urn:x">Il.</bibl></cit>, al.:—<tr>come into being</tr>: hence, '
        '<bibl n="urn:y">Pl.</bibl> <tr>arise</tr></sense>'
    )
    assert module.lsj_sense(forms)["definitions"] == ["come into being"]


def test_without_lsj_dir_the_index_stays_at_the_french_format(tmp_path: Path):
    module = load_script()
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        json.dumps(
            {"word": "Haus", "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["house"]}]}
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"

    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute(
        "SELECT value FROM metadata WHERE key = 'index_format'"
    ).fetchone() == ("5",)
    assert connection.execute("SELECT count(*) FROM lsj_entries").fetchone() == (0,)
    assert connection.execute("SELECT count(*) FROM metadata WHERE key = 'lsj_pin'").fetchone() == (
        0,
    )
    connection.close()


def test_idiom_lookup_indexes_only_two_to_four_ordinary_word_entries(tmp_path: Path):
    module = load_script()
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        "\n".join(
            json.dumps(item, ensure_ascii=False)
            for item in [
                {
                    "word": "in gamba",
                    "lang_code": "it",
                    "pos": "adj",
                    "senses": [{"glosses": ["capable, on the ball"]}],
                },
                {
                    "word": "macchina da scrivere",
                    "lang_code": "it",
                    "pos": "noun",
                    "senses": [{"glosses": ["typewriter"]}],
                },
                # Five words and one word are outside the idiom bound.
                {
                    "word": "troppo lunga davvero per essere",
                    "lang_code": "it",
                    "pos": "noun",
                    "senses": [{"glosses": ["too long"]}],
                },
                {
                    "word": "voce",
                    "lang_code": "it",
                    "pos": "noun",
                    "senses": [{"glosses": ["word"]}],
                },
                # All-alt-of and all-form-of entries are not idioms.
                {
                    "word": "altra forma",
                    "lang_code": "it",
                    "pos": "noun",
                    "senses": [
                        {
                            "glosses": ["alternative form of forma"],
                            "tags": ["alt-of"],
                            "alt_of": [{"word": "forma"}],
                        }
                    ],
                },
                {
                    "word": "vecchia forma",
                    "lang_code": "it",
                    "pos": "noun",
                    "senses": [
                        {
                            "glosses": ["form of vecchio"],
                            "tags": ["form-of"],
                            "form_of": [{"word": "vecchio"}],
                        }
                    ],
                },
                # A mixed entry keeps its ordinary sense.
                {
                    "word": "mista espressione",
                    "lang_code": "it",
                    "pos": "noun",
                    "senses": [
                        {
                            "glosses": ["alternative form of espressione"],
                            "tags": ["alt-of"],
                            "alt_of": [{"word": "espressione"}],
                        },
                        {"glosses": ["mixed expression"]},
                    ],
                },
            ]
        )
        + "\n",
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"

    module.derive(source, output, "fixture-v1")

    connection = sqlite3.connect(output)
    entries = connection.execute(
        "SELECT headword, upos, words_json, senses_json, source, version "
        "FROM idiom_entries ORDER BY headword"
    ).fetchall()
    words = connection.execute(
        "SELECT language, word, headword FROM idiom_words ORDER BY word"
    ).fetchall()
    connection.close()

    assert [
        (headword, upos, json.loads(words_json), [sense["Gloss"] for sense in json.loads(senses)])
        for headword, upos, words_json, senses, _source, _version in entries
    ] == [
        ("in gamba", "ADJ", ["in", "gamba"], ["capable, on the ball"]),
        ("macchina da scrivere", "NOUN", ["macchina", "da", "scrivere"], ["typewriter"]),
        ("mista espressione", "NOUN", ["mista", "espressione"], ["mixed expression"]),
    ]
    assert all(
        source == "wiktionary" and version == "fixture-v1" for *_, source, version in entries
    )
    assert words == [
        ("it", "da", "macchina da scrivere"),
        ("it", "espressione", "mista espressione"),
        ("it", "gamba", "in gamba"),
        ("it", "in", "in gamba"),
        ("it", "macchina", "macchina da scrivere"),
        ("it", "mista", "mista espressione"),
        ("it", "scrivere", "macchina da scrivere"),
    ]


def derive_idiom_rows(
    module, tmp_path: Path, items: list[dict], reviewed_idioms=None
) -> tuple[list[tuple], list[tuple]]:
    """Derive an index from raw items and return its idiom entry and word rows.

    Without explicit reviewed idioms, an empty list keeps the shipped one out.
    """
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        "\n".join(json.dumps(item, ensure_ascii=False) for item in items) + "\n",
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    if reviewed_idioms is None:
        reviewed_idioms = module.ReviewedIdioms()
    module.derive(source, output, "fixture-v1", reviewed_idioms=reviewed_idioms)
    connection = sqlite3.connect(output)
    entries = connection.execute(
        "SELECT language, headword, upos, senses_json, source FROM idiom_entries "
        "ORDER BY headword, source"
    ).fetchall()
    words = connection.execute(
        "SELECT word, headword, source FROM idiom_words ORDER BY headword, word, source"
    ).fetchall()
    connection.close()
    return [
        (language, headword, upos, [sense["Gloss"] for sense in json.loads(senses)], source)
        for language, headword, upos, senses, source in entries
    ], words


def test_idiom_lookup_excludes_proper_names_but_keeps_phrases(tmp_path: Path):
    module = load_script()

    entries, words = derive_idiom_rows(
        module,
        tmp_path,
        [
            {
                "word": "La Calamita",
                "lang_code": "it",
                "pos": "name",
                "senses": [{"glosses": ["a surname"]}],
            },
            # A lowercase-initial name is still a name.
            {
                "word": "mare Adriatico",
                "lang_code": "it",
                "pos": "name",
                "senses": [{"glosses": ["Adriatic Sea"]}],
            },
            # Phrase, prepositional phrase and proverb entries have no mapped
            # UPOS, but they are the idioms the index exists for.
            {
                "word": "in bocca al lupo",
                "lang_code": "it",
                "pos": "phrase",
                "senses": [{"glosses": ["good luck"]}],
            },
            {
                "word": "a vanvera",
                "lang_code": "it",
                "pos": "prep_phrase",
                "senses": [{"glosses": ["haphazardly"]}],
            },
            {
                "word": "l'appetito vien mangiando",
                "lang_code": "it",
                "pos": "proverb",
                "senses": [{"glosses": ["appetite comes with eating"]}],
            },
        ],
    )

    assert entries == [
        ("it", "a vanvera", "", ["haphazardly"], "wiktionary"),
        ("it", "in bocca al lupo", "", ["good luck"], "wiktionary"),
        ("it", "l'appetito vien mangiando", "", ["appetite comes with eating"], "wiktionary"),
    ]
    assert {headword for _word, headword, _source in words} == {
        "a vanvera",
        "in bocca al lupo",
        "l'appetito vien mangiando",
    }


def test_idiom_lookup_excludes_entries_with_only_the_literal_use_marker(tmp_path: Path):
    module = load_script()

    entries, words = derive_idiom_rows(
        module,
        tmp_path,
        [
            {
                "word": "allor che",
                "lang_code": "it",
                "pos": "conj",
                "senses": [
                    {"glosses": ["Used other than figuratively or idiomatically: see allor, che."]}
                ],
            },
            # Beside a real sense, the marker stays: it tells the provider
            # which use is literal.
            {
                "word": "al proprio posto",
                "lang_code": "it",
                "pos": "prep_phrase",
                "senses": [
                    {
                        "glosses": [
                            "Used other than figuratively or idiomatically: see al, proprio, posto."
                        ]
                    },
                    {"glosses": ["in one's lane; in one's place"]},
                ],
            },
        ],
    )

    assert entries == [
        (
            "it",
            "al proprio posto",
            "",
            [
                "Used other than figuratively or idiomatically: see al, proprio, posto.",
                "in one's lane; in one's place",
            ],
            "wiktionary",
        )
    ]
    assert {headword for _word, headword, _source in words} == {"al proprio posto"}


def test_reviewed_idiom_is_indexed_even_when_wiktionary_lists_it_as_a_name(tmp_path: Path):
    module = load_script()
    reviewed_path = tmp_path / "reviewed_idioms.json"
    reviewed_path.write_text(
        json.dumps(
            {"languages": {"it": [{"headword": "Far West", "words": ["Far", "West"]}]}},
        ),
        encoding="utf-8",
    )

    entries, words = derive_idiom_rows(
        module,
        tmp_path,
        [
            {
                "word": "Far West",
                "lang_code": "it",
                "pos": "name",
                "senses": [{"glosses": ["a state of lawlessness"]}],
            },
        ],
        reviewed_idioms=module.load_reviewed_idioms(reviewed_path),
    )

    assert entries == [("it", "Far West", "", [], "reviewed-idioms")]
    assert words == [
        ("far", "Far West", "reviewed-idioms"),
        ("west", "Far West", "reviewed-idioms"),
    ]


def test_single_word_names_still_reach_the_lemma_entries(tmp_path: Path):
    module = load_script()

    derive_idiom_rows(
        module,
        tmp_path,
        [
            {
                "word": "Roma",
                "lang_code": "it",
                "pos": "name",
                "senses": [{"glosses": ["Rome"]}],
            },
        ],
    )

    connection = sqlite3.connect(tmp_path / "dictionary.sqlite")
    rows = connection.execute(
        "SELECT language, lemma, upos FROM entries WHERE lemma = 'roma'"
    ).fetchall()
    connection.close()
    assert rows == [("it", "roma", "")]


def test_reviewed_idioms_use_their_own_source_and_version(tmp_path: Path):
    module = load_script()
    source = tmp_path / "kaikki.jsonl"
    source.write_text("", encoding="utf-8")
    reviewed_path = tmp_path / "reviewed_idioms.json"
    reviewed_path.write_text(
        json.dumps(
            {
                "version": "reviewed-test/1",
                "languages": {
                    "el": [
                        {
                            "headword": "κάνω τα στραβά μάτια",
                            "words": ["κάνω", "τα", "στραβά", "μάτια"],
                        }
                    ]
                },
            },
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"

    module.derive(
        source,
        output,
        "fixture-v1",
        reviewed_idioms=module.load_reviewed_idioms(reviewed_path),
    )

    connection = sqlite3.connect(output)
    entries = connection.execute(
        "SELECT language, headword, upos, words_json, senses_json, source, version "
        "FROM idiom_entries"
    ).fetchall()
    words = connection.execute(
        "SELECT language, word, headword, upos, source FROM idiom_words ORDER BY word"
    ).fetchall()
    connection.close()

    assert entries == [
        (
            "el",
            "κάνω τα στραβά μάτια",
            "",
            '["κάνω","τα","στραβά","μάτια"]',
            "[]",
            "reviewed-idioms",
            "reviewed-test/1",
        )
    ]
    assert words == [
        ("el", "κάνω", "κάνω τα στραβά μάτια", "", "reviewed-idioms"),
        ("el", "μάτια", "κάνω τα στραβά μάτια", "", "reviewed-idioms"),
        ("el", "στραβά", "κάνω τα στραβά μάτια", "", "reviewed-idioms"),
        ("el", "τα", "κάνω τα στραβά μάτια", "", "reviewed-idioms"),
    ]


def test_committed_reviewed_idiom_list_loads_every_launch_language():
    module = load_script()

    reviewed = module.load_reviewed_idioms(module.default_reviewed_idioms_path())

    assert "it" in reviewed.languages
    assert "de" in reviewed.languages
    assert "el" in reviewed.languages


@pytest.mark.parametrize(
    "payload",
    [
        [],
        {"languages": []},
        {"languages": {"it": {}}},
        {"languages": {"it": ["not an object"]}},
        {"languages": {"it": [{}]}},
        {"languages": {"it": [{"headword": "x", "words": []}]}},
        {"languages": {"it": [{"headword": "x", "words": [""]}]}},
        {"languages": {"it": [{"headword": "in gamba", "words": ["in", "gambe"]}]}},
        {"languages": {"it": [{"headword": "x", "words": ["a"]}]}},
        {"languages": {"it": [{"headword": "x", "words": ["a", "b", "c", "d", "e"]}]}},
        {"languages": {"xx": []}},
    ],
)
def test_reviewed_idioms_reject_malformed_data(tmp_path: Path, payload):
    module = load_script()
    path = tmp_path / "reviewed_idioms.json"
    path.write_text(json.dumps(payload), encoding="utf-8")

    with pytest.raises(ValueError, match="reviewed idiom"):
        module.load_reviewed_idioms(path)


def test_reviewed_idioms_default_version_and_empty_languages(tmp_path: Path):
    module = load_script()
    path = tmp_path / "reviewed_idioms.json"
    path.write_text(json.dumps({"languages": {}}), encoding="utf-8")

    reviewed = module.load_reviewed_idioms(path)

    assert reviewed.version == module.DEFAULT_REVIEWED_IDIOM_VERSION
    assert reviewed.languages == {}


SYNTHETIC_LS = """<?xml version="1.0" encoding="UTF-8"?>
<TEI.2>
<text><body>
<entryFree id="n10" key="dico2" type="main"><orth extent="full" lang="la">dīco</orth>, <itype>are</itype>,
  <sense id="n10.0" n="I" level="1"><hi rend="ital">To dedicate</hi>, <hi rend="ital">consecrate</hi>:
    <cit><quote lang="la">aedem,</quote> <trans><tr>a temple</tr>,</trans>
    <bibl n="urn:cts:latinLit:phi0474.phi001:1:2"><author>Cic.</author> Att. 1, 2</bibl></cit>;
    <bibl><author>id.</author> Fam. 2, 3</bibl>; <bibl>4, 5</bibl>.</sense>
  <sense id="n10.1" n="A" level="2"><hi rend="ital">Comp.</hi> <hi rend="ital">Hence</hi> <xr><lbl>v.</lbl>
    <ref type="sym" targOrder="U">dicto</ref></xr>; <xr><lbl>cf.</lbl> <ref type="sym" targOrder="U">infra</ref></xr>.</sense>
</entryFree>
<entryFree id="n9" key="dico1" type="main"><orth extent="full" lang="la">dīco</orth>, <itype>dixi</itype>,
  <sense id="n9.0" n="I" level="1"><hi rend="ital">To say</hi>, <hi rend="ital">speak</hi>: <bibl><author>Ov.</author> M. 1, 1</bibl>.</sense>
  <sense id="n9.2" n="A" level="2"><hi rend="ital">Sup.</hi> <hi rend="ital">Esp.</hi> <hi rend="ital">to name</hi>.</sense>
  <sense id="n9.1" n="A" level="2"><hi rend="ital">To tell</hi>.</sense>
  <sense id="n9.10" n="A" level="2"><hi rend="ital">To declare</hi>.</sense>
</entryFree>
<entryFree id="n11" key="dicto" type="main"><orth extent="full" lang="la">dictō</orth>, <itype>are</itype>, <hi rend="ital">no sense element</hi>.</entryFree>
<entryFree id="n12" key="Roma" type="main"><orth extent="full" lang="la">Rōma</orth>, <sense id="n12.0" n="I" level="1"><hi rend="ital">Rome</hi>.</sense></entryFree>
<entryFree id="n13" key="stub" type="gloss"><orth>stub</orth><sense id="n13.0" n="I" level="1"><hi rend="ital">skip</hi></sense></entryFree>
</body></text>
</TEI.2>
"""


def ls_index(tmp_path: Path, module, xml: str = SYNTHETIC_LS) -> list:
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        json.dumps(
            {"word": "Haus", "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["house"]}]}
        ),
        encoding="utf-8",
    )
    ls_dir = tmp_path / "ls"
    ls_dir.mkdir()
    (ls_dir / "lat.ls.perseus-eng1.xml").write_text(xml, encoding="utf-8")
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1", ls=module.LsSource(directory=ls_dir, pin="lspin"))
    connection = sqlite3.connect(output)
    try:
        assert connection.execute("SELECT value FROM metadata WHERE key = 'ls_pin'").fetchone() == (
            "lspin",
        )
        assert connection.execute(
            "SELECT value FROM metadata WHERE key = 'index_format'"
        ).fetchone() == ("8",)
        return connection.execute(
            "SELECT entry_id, headword, orth, senses_json, homograph FROM ls_entries"
        ).fetchall()
    finally:
        connection.close()


def test_ls_entries_keep_homographs_in_id_order_and_skip_non_main_entries(tmp_path: Path):
    rows = ls_index(tmp_path, load_script())

    # Numeric id order (n9 before n10), bare keys with homograph digits dropped,
    # the macron spelling kept as orth, and the gloss stub left out.
    assert [(row[0], row[1], row[2]) for row in rows] == [
        ("n9", "dico", "dīco"),
        ("n10", "dico", "dīco"),
        ("n11", "dicto", "dictō"),
        ("n12", "Roma", "Rōma"),
    ]


def test_ls_entries_keep_the_source_homograph_number(tmp_path: Path):
    rows = {row[0]: row for row in ls_index(tmp_path, load_script())}

    assert (rows["n9"][4], rows["n10"][4], rows["n11"][4]) == (1, 2, 0)


def test_ls_homograph_number_is_the_keys_trailing_digits():
    module = load_script()

    assert module.ls_homograph_number("Pennus2") == 2
    assert module.ls_homograph_number("dico") == 0
    assert module.ls_homograph_number("dico12") == 12


def test_ls_entry_without_senses_is_kept_empty(tmp_path: Path):
    rows = {row[0]: row for row in ls_index(tmp_path, load_script())}

    assert json.loads(rows["n11"][3]) == []


def test_ls_senses_follow_id_order_not_repeated_n_labels(tmp_path: Path):
    rows = {row[0]: row for row in ls_index(tmp_path, load_script())}
    senses = json.loads(rows["n9"][3])

    assert [sense["id"] for sense in senses] == ["n9.0", "n9.1", "n9.2", "n9.10"]
    assert [sense["n"] for sense in senses] == ["I", "A", "A", "A"]
    assert [sense["level"] for sense in senses] == [1, 2, 2, 2]


def test_ls_outline_definitions_leave_out_abbreviations_and_citations(tmp_path: Path):
    rows = {row[0]: row for row in ls_index(tmp_path, load_script())}
    nine = {sense["id"]: sense for sense in json.loads(rows["n9"][3])}
    ten = {sense["id"]: sense for sense in json.loads(rows["n10"][3])}

    assert nine["n9.0"]["definitions"] == ["To say", "speak"]
    assert nine["n9.2"]["definitions"] == ["to name"]  # "Sup." and "Esp." are abbreviations
    assert ten["n10.0"]["definitions"] == ["To dedicate", "consecrate"]
    assert ten["n10.0"]["glosses"] == ["To dedicate", "consecrate", "a temple"]
    assert ten["n10.1"]["definitions"] == ["Hence"]  # "Comp." is an abbreviation


def test_ls_citations_carry_their_author_reference(tmp_path: Path):
    rows = {row[0]: row for row in ls_index(tmp_path, load_script())}
    senses = json.loads(rows["n10"][3])

    assert senses[0]["citations"] == [
        {"author": "Cic.", "text": "Cic. Att. 1, 2"},
        {"author": "Cic.", "text": "id. Fam. 2, 3"},  # "id." repeats the previous author
        {"text": "4, 5"},  # no author element, so no author reference
    ]


def test_ls_cross_references_resolve_to_a_unique_target_or_stay_unresolved(tmp_path: Path):
    rows = {row[0]: row for row in ls_index(tmp_path, load_script())}
    senses = json.loads(rows["n10"][3])

    assert senses[1]["crossrefs"] == [
        {"text": "dicto", "entry_id": "n11", "headword": "dicto"},
        {"text": "infra"},
    ]


def test_ls_cross_reference_to_a_homograph_headword_is_ambiguous(tmp_path: Path):
    module = load_script()
    xml = SYNTHETIC_LS.replace('<ref type="sym" targOrder="U">dicto</ref>', "<ref>dīco</ref>")
    rows = {row[0]: row for row in ls_index(tmp_path, module, xml)}

    assert json.loads(rows["n10"][3])[1]["crossrefs"][0] == {"text": "dīco"}


def test_ls_cross_reference_with_a_homograph_number_picks_that_entry(tmp_path: Path):
    module = load_script()
    xml = SYNTHETIC_LS.replace('<ref type="sym" targOrder="U">dicto</ref>', "<ref>dico1</ref>")
    rows = {row[0]: row for row in ls_index(tmp_path, module, xml)}

    assert json.loads(rows["n10"][3])[1]["crossrefs"][0] == {
        "text": "dico1",
        "entry_id": "n9",
        "headword": "dico",
    }


def test_without_ls_dir_there_is_no_ls_table_content(tmp_path: Path):
    module = load_script()
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        json.dumps(
            {"word": "Haus", "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["house"]}]}
        ),
        encoding="utf-8",
    )
    output = tmp_path / "dictionary.sqlite"
    module.derive(source, output, "fixture-v1")
    connection = sqlite3.connect(output)
    try:
        assert connection.execute("SELECT count(*) FROM ls_entries").fetchone() == (0,)
        assert connection.execute(
            "SELECT count(*) FROM metadata WHERE key = 'ls_pin'"
        ).fetchone() == (0,)
    finally:
        connection.close()


def test_ls_indexes_only_the_first_encoding_and_keeps_greek_origin_entries(tmp_path: Path):
    module = load_script()
    xml = """<TEI.2><text><body>
<entryFree id="n1" key="abacus" type="greek"><orth>abăcus</orth><sense id="n1.0" n="I" level="1"><hi rend="ital">a table</hi></sense></entryFree>
</body></text></TEI.2>"""
    source = tmp_path / "kaikki.jsonl"
    source.write_text(
        json.dumps(
            {"word": "Haus", "lang_code": "de", "pos": "noun", "senses": [{"glosses": ["h"]}]}
        ),
        encoding="utf-8",
    )
    ls_dir = tmp_path / "ls"
    ls_dir.mkdir()
    (ls_dir / "lat.ls.perseus-eng1.xml").write_text(xml, encoding="utf-8")
    (ls_dir / "lat.ls.perseus-eng2.xml").write_text(xml, encoding="utf-8")

    rows = module.collect_ls_entries(ls_dir)

    assert [(row[0], row[1]) for row in rows] == [("n1", "abacus")]


def test_ls_entry_level_citations_redirects_and_urn_authors_are_kept(tmp_path: Path):
    module = load_script()
    xml = """<TEI.2><text><body>
<entryFree id="n1" key="a2" type="main"><orth>a</orth>, v. a2 and
 <bibl n="urn:cts:latinLit:phi0474.phi001:1:2">Att. 1, 2</bibl>.</entryFree>
<entryFree id="n2" key="a2" type="main"><orth>a</orth>, v. <hi rend="ital">no</hi></entryFree>
</body></text></TEI.2>"""
    rows = {row[0]: row for row in ls_index(tmp_path, module, xml)}
    sense = json.loads(rows["n1"][3])[0]

    assert sense["id"] == "n1"
    assert sense["level"] == 0
    assert sense["citations"] == [{"author": "phi0474", "text": "Att. 1, 2"}]
    assert sense["crossrefs"] == [{"text": "a2"}]  # two entries share the key


def test_ls_definition_window_restarts_after_the_meaning_marker():
    module = load_script()
    sense = ElementTree.fromstring(  # noqa: S314
        '<sense id="n1.0" n="I" level="1"><hi rend="ital">first</hi> <bibl><author>Cic.</author> A</bibl>'
        ' :— <hi rend="ital">second</hi> <hi rend="ital">fin.</hi></sense>'
    )
    content = module.LsContent()
    module.ls_collect(sense, content)

    assert content.definitions == ["first", "second"]


# ---- Lewis & Short: entries that lost their text (#2793) --------------------

SYNTHETIC_LS_LOST_TEXT = """<TEI.2><text><body>
<entryFree id="n1" key="lego1" type="main"><orth>lēgo</orth>, <itype>āvi</itype>,
 <sense id="n1.0" n="I" level="1"><hi rend="ital">To send</hi>.</sense>
 <sense id="n1.1" n="1" level="3"> <orth>lēgātus</orth>, <itype>i</itype>, <gen>m.</gen> </sense>
 <sense id="n1.2" n="A" level="2"><hi rend="ital">An ambassador</hi>, <bibl><author>Cic.</author> A 1</bibl>.</sense>
 <sense id="n1.3" n="a" level="4"><hi rend="ital">a deputy</hi>.</sense>
 <sense id="n1.4" n="2" level="3"> <orth>lēgātum</orth>, <itype>i</itype>, <gen>n.</gen>, <hi rend="ital">a bequest</hi>.</sense>
</entryFree>
<entryFree id="n2" key="legatus" type="main"><orth>lēgātus</orth>, <itype>i</itype>, v. 1. lego
 <sense id="n2.0" n="I" level="1"><hi rend="ital">fin.</hi> 1.</sense></entryFree>
<entryFree id="n3" key="legatum" type="main"><orth>lēgātum</orth>, <itype>i</itype>, v. 1. lego
 <sense id="n3.0" n="I" level="1"><hi rend="ital">fin.</hi> 2.</sense></entryFree>
<entryFree id="n4" key="pueritia" type="main"><orth>pŭerĭtia</orth>, <sense id="n4.0" n="I" level="1"><hi rend="ital">boyhood</hi>.</sense></entryFree>
<entryFree id="n5" key="puritia" type="main"><orth>pūrĭtia</orth>, v. pueritia.</entryFree>
<entryFree id="n6" key="percello" type="main"><orth>percello</orth>, <sense id="n6.0" n="I" level="1"><hi rend="ital">to strike down</hi>.</sense></entryFree>
<entryFree id="n7" key="perculsus" type="main"><orth>perculsus</orth>, <itype>a, um</itype>, <mood>Part.</mood>, from percello.</entryFree>
<entryFree id="n8" key="prior" type="main"><orth>prior</orth>, <sense id="n8.0" n="I" level="1"><hi rend="ital">former</hi>.</sense></entryFree>
<entryFree id="n9" key="prius" type="main"><orth>prĭus</orth>, v. <itype>prior</itype>, <pos>adv.</pos></entryFree>
<entryFree id="n10" key="clueo" type="main"><orth>clueo</orth>, <sense id="n10.0" n="I" level="1"><hi rend="ital">to be called</hi>.</sense></entryFree>
<entryFree id="n11" key="cluo" type="main"><orth>clŭo</orth> = clueo, q. v.</entryFree>
<entryFree id="n12" key="foo" type="main"><orth>foo</orth>, v. the foll. art.</entryFree>
<entryFree id="n13" key="bar" type="main"><orth>bar</orth>, <sense id="n13.0" n="I" level="1"><hi rend="ital">a bar</hi>.</sense></entryFree>
<entryFree id="n14" key="loop1" type="main"><orth>loop</orth>, v. loop2.</entryFree>
<entryFree id="n15" key="loop2" type="main"><orth>loop</orth>, v. loop1.</entryFree>
<entryFree id="n16" key="turtur" type="main"><orth>turtur</orth>, <sense id="n16.0" n="I" level="1"><hi rend="ital">fem.</hi> x <bibl><author>Plin.</author> 30</bibl>, <hi rend="ital">a turtle-dove</hi>: <bibl><author>Varr.</author> R. 3</bibl>.</sense></entryFree>
<entryFree id="n17" key="Roma" type="main"><orth>Rōma</orth>, <itype>ae</itype>, <gen>f.</gen>
 <sense id="n17.0" n="I" level="1"><hi rend="ital">the city</hi>.</sense>
 <sense id="n17.1" n="A" level="2"> <orth>Rōmānus</orth>, <itype>a, um</itype>, <pos>adj.</pos>, <hi rend="ital">Roman</hi>.</sense>
 <sense id="n17.2" n="a" level="4"><hi rend="ital">the Romans</hi>.</sense>
 <sense id="n17.3" n="B" level="2"><hi rend="ital">Hence</hi>.</sense></entryFree>
<entryFree id="n18" key="Arma" type="main"><orth>arma</orth>, <sense id="n18.0" n="I" level="1"><hi rend="ital">arms</hi>; also
 <orth>Rōma</orth>, <itype>x</itype>, <hi rend="ital">dup</hi>.</sense></entryFree>
<entryFree id="n20" key="absonus" type="main"><orth>absonus</orth>, <sense id="n20.0" n="I" level="1"><hi rend="ital">discordant</hi>.</sense>
 <sense id="n20.1" n="II" level="1"><hi rend="ital">Adv.</hi> <hi rend="ital">out of tune</hi>.</sense></entryFree>
<entryFree id="n21" key="absone" type="main"><orth>absŏnē</orth>, v. absonus <sense id="n21.0" n="I" level="1"><hi rend="ital">fin.</hi></sense></entryFree>
<entryFree id="n22" key="Abdal" type="main"><orth>Abdal</orth>, <sense id="n22.0" n="I" level="1"><hi rend="ital">v.</hi> bar</sense></entryFree>
<entryFree id="n23" key="absolutorius" type="main"><orth>absolutorius</orth>, <sense id="n23.0" n="I" level="1"><hi rend="ital">acquitting</hi>.</sense>
 <sense id="n23.1" n="II" level="1"><hi rend="ital">Subst.</hi> <orth>absŏlūtōrĭum</orth>, <itype>ii</itype>, <gen>n.</gen>, <hi rend="ital">a means of deliverance</hi>.</sense></entryFree>
<entryFree id="n19" key="Comp" type="main"><orth>comp</orth>, <sense id="n19.0" n="I" level="1"> <orth>comp</orth>, <itype>a</itype>, <hi rend="ital">self</hi>.</sense></entryFree>
</body></text></TEI.2>"""


def lost_text_rows(tmp_path: Path) -> dict:
    rows = ls_index(tmp_path, load_script(), SYNTHETIC_LS_LOST_TEXT)
    return {row[0]: row for row in rows}


def glosses_of(row) -> list[str]:
    return [item for sense in json.loads(row[3]) for item in sense.get("glosses", [])]


def test_ls_redirect_inherits_the_text_of_the_entry_it_names(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    senses = json.loads(rows["n5"][3])
    assert glosses_of(rows["n5"]) == ["boyhood"]
    assert senses[0]["crossrefs"] == [
        {"text": "pueritia", "entry_id": "n4", "headword": "pueritia"}
    ]
    assert "citations" not in senses[1]


def test_ls_fin_pointer_copies_the_numbered_group_it_points_at(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    # "v. 1. lego fin. 1.": the span of the last sense numbered 1, written one
    # level above its bare heading, and not the bequest that follows it.
    assert glosses_of(rows["n2"]) == ["An ambassador", "a deputy"]
    assert glosses_of(rows["n3"]) == ["a bequest"]
    assert [sense["id"] for sense in json.loads(rows["n2"][3])] == ["n2", "n1.2", "n1.3"]


def test_ls_participle_label_itype_redirect_and_qv_name_their_entries(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert glosses_of(rows["n7"]) == ["to strike down"]
    assert glosses_of(rows["n9"]) == ["former"]
    assert glosses_of(rows["n11"]) == ["to be called"]


def test_ls_the_foll_names_the_next_entry(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert glosses_of(rows["n12"]) == ["a bar"]


def test_ls_redirects_that_resolve_to_no_text_stay_as_written(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert glosses_of(rows["n14"]) == []
    assert glosses_of(rows["n15"]) == []
    assert json.loads(rows["n14"][3])[0]["crossrefs"][0]["entry_id"] == "n15"


def test_ls_bare_reference_prefers_the_exact_case_among_casefolded_homographs():
    module = load_script()
    entries = [
        {"id": "n1", "key": "Asper", "headword": "Asper", "senses": []},
        {"id": "n2", "key": "asper1", "headword": "asper", "senses": []},
        {"id": "n3", "key": "x", "headword": "x", "senses": [{"crossrefs": [{"text": "asper"}]}]},
    ]

    module.ls_resolve(entries)

    assert entries[2]["senses"][0]["crossrefs"] == [
        {"text": "asper", "entry_id": "n2", "headword": "asper"}
    ]


def test_ls_italic_after_the_first_citation_is_a_gloss_when_none_precedes_it(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    sense = json.loads(rows["n16"][3])[0]
    assert sense["glosses"] == ["a turtle-dove"]
    assert "definitions" not in sense


def test_ls_entry_written_inside_a_sense_becomes_its_own_entry(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    row = rows["n17.1"]
    assert (row[1], row[2], row[4]) == ("Romanus", "Rōmānus", 0)
    # Its own sense and the deeper sense under it, not the next sibling "B".
    assert [sense["id"] for sense in json.loads(row[3])] == ["n17.1", "n17.2"]
    assert glosses_of(row) == ["Roman", "the Romans"]
    # The parent still holds all of its own senses.
    assert len(json.loads(rows["n17"][3])) == 4


def test_ls_inner_headword_is_not_added_when_an_entry_with_text_has_it(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert "n18.0" not in rows  # "Rōma" is already the entry n17
    assert "n19.0" not in rows  # the entry's own headword repeated
    assert "n1.1" not in rows  # lēgātus already has its redirect entry n2, now with text


def test_ls_bare_fin_pointer_copies_only_the_last_top_level_section(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert glosses_of(rows["n21"]) == ["out of tune"]


def test_ls_italic_v_names_the_entry_after_it(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert json.loads(rows["n22"][3])[0]["crossrefs"] == [
        {"text": "bar", "entry_id": "n13", "headword": "bar"}
    ]
    assert glosses_of(rows["n22"]) == ["a bar"]


def test_ls_label_before_an_inner_headword_does_not_hide_it(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert rows["n23.1"][1] == "absolutorium"


def test_ls_pointer_edge_cases_leave_the_entry_as_written():
    module = load_script()
    target = [
        {"id": "t.0", "n": "I", "level": 1, "glosses": ["only"]},
        {"id": "t.1", "n": "A", "level": 2, "glosses": ["part"]},
    ]

    assert module.ls_pointed_senses(target, "9") == []  # no sense numbered 9
    assert [s["id"] for s in module.ls_pointed_senses(target, "")] == ["t.0", "t.1"]  # one section
    entries = [{"id": "a"}, {"id": "b"}, {"id": "c"}]
    assert module.ls_neighbour(entries, 2, "the foll") is None
    assert module.ls_neighbour(entries, 0, "the preced") is None
    assert module.ls_neighbour(entries, 1, "the preced") == {"id": "a"}
    assert module.ls_neighbour(entries, 1, "elsewhere") is None


def test_ls_the_original_text_entries_are_unchanged_by_the_lost_text_fixes(tmp_path: Path):
    rows = lost_text_rows(tmp_path)

    assert glosses_of(rows["n4"]) == ["boyhood"]
    assert json.loads(rows["n4"][3])[0].keys() >= {"id", "n", "level", "glosses"}
