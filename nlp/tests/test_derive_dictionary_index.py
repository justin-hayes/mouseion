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
    ).fetchone() == ("3",)
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
