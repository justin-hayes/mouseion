from __future__ import annotations

import importlib.util
import gzip
import json
import sqlite3
import stat
from pathlib import Path


def load_script():
    path = Path(__file__).parents[1] / "scripts" / "derive_dictionary_index.py"
    spec = importlib.util.spec_from_file_location("derive_dictionary_index", path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_fixture_derives_filtered_entries_and_metadata(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl", output, "dump-2026-09-14")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT value FROM metadata WHERE key = 'provider_version'").fetchone() == ("dump-2026-09-14",)
    assert connection.execute("SELECT value FROM metadata WHERE key = 'license'").fetchone() == ("Wiktionary-derived data: CC BY-SA 3.0 / GFDL",)
    assert connection.execute("SELECT value FROM metadata WHERE key = 'attribution'").fetchone() == ("Wiktionary contributors; CC BY-SA 3.0 / GFDL",)
    row = connection.execute("SELECT language, lemma, upos, senses_json, gender, article, plural FROM entries WHERE lemma = 'haus'").fetchone()
    assert row[:3] == ("de", "haus", "NOUN")
    assert json.loads(row[3])[0]["Gloss"] == "house"
    assert row[4:] == ("Neut", "das", "Häuser")
    assert connection.execute("SELECT count(*) FROM entries").fetchone() == (7,)
    assert connection.execute("SELECT count(*) FROM entries WHERE lemma = 'unbekannt'").fetchone() == (0,)
    assert connection.execute("SELECT ipa FROM entries WHERE lemma = 'albero'").fetchone() == ("",)
    assert set(connection.execute("SELECT DISTINCT language FROM entries").fetchall()) == {("de",), ("it",)}
    for (senses_json,) in connection.execute("SELECT senses_json FROM entries"):
        senses = json.loads(senses_json)
        assert senses
        assert all(sense["Gloss"].strip() for sense in senses)
    assert connection.execute("SELECT upos, plural, senses_json FROM entries WHERE lemma IN ('aufstehen', 'gut') ORDER BY lemma").fetchall() == [
        ("VERB", "", '[{"Gloss":"to get up","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"","Article":"","Plural":"","IPA":""}]'),
        ("ADJ", "", '[{"Gloss":"good","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"","Article":"","Plural":"","IPA":""}]'),
    ]
    italian = connection.execute("SELECT lemma, gender, article, plural FROM entries WHERE language = 'it' ORDER BY lemma").fetchall()
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
            for word, gender, form in [("Gauner", "masculine", "Gauner"), ("Feigheit", "feminine", "Feigheiten")]
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
    module.derive(Path(__file__).parents[1] / "testdata" / "dictionary_article_forms.jsonl", output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT lemma, article FROM entries ORDER BY lemma").fetchall() == [
        ("feigheit", "die"),
        ("gauner", "der"),
    ]
    connection.close()


def test_captured_kaikki_ipa_is_normalized(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(Path(__file__).parents[1] / "testdata" / "dictionary_article_forms.jsonl", output, "fixture-v1")

    connection = sqlite3.connect(output)
    assert connection.execute("SELECT ipa FROM entries WHERE lemma = 'feigheit'").fetchone() == ("/ˈfaɪ̯kaɪ̯t/",)
    assert connection.execute("SELECT ipa FROM entries WHERE lemma = 'gauner'").fetchone() == ("/ˈɡaʊ̯nər/",)
    connection.close()


def test_derivation_normalizes_ipa_and_extracts_principal_parts(tmp_path: Path):
    module = load_script()
    output = tmp_path / "dictionary.sqlite"
    module.derive(Path(__file__).parents[1] / "testdata" / "dictionary_form_presentation.jsonl", output, "fixture-v1")

    connection = sqlite3.connect(output)
    rows = {
        lemma: (ipa, principal_parts)
        for lemma, ipa, principal_parts in connection.execute("SELECT lemma, ipa, principal_parts FROM entries")
    }
    gehen_senses = json.loads(connection.execute("SELECT senses_json FROM entries WHERE lemma = 'gehen'").fetchone()[0])
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
    module.derive(Path(__file__).parents[1] / "testdata" / "dictionary_fixture.jsonl", output, "dump-2026-09-14")

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
            json.dumps({"word": word, "lang_code": "de", "pos": pos, "senses": [{"glosses": [word], "tags": tags}]})
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
