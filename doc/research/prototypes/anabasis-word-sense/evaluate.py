"""THROWAWAY: score only a complete, explicitly human-reviewed native-sense set.

No native ID crosswalk is inferred between lexica. Abstentions stay in the
denominator for correct yield, not conditional chosen-sense accuracy.
"""

import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent


def main():
    gold_path = HERE / "sense-gold.json"
    if not gold_path.exists():
        raise SystemExit(
            "Not scored: sense-gold.json does not exist; live human review is required."
        )
    raw_gold = gold_path.read_bytes()
    gold = json.loads(raw_gold)
    assert gold["status"] == "human_confirmed"
    assert gold["reviewer"] == "Justin Hayes"
    assert gold["review_ledger"]
    raw_predictions = (HERE / "predictions.json").read_bytes()
    predictions = json.loads(raw_predictions)
    assert (
        hashlib.sha256((HERE / "inputs.json").read_bytes()).hexdigest()
        == predictions["inputs_sha256"]
    )
    assert gold["inputs_sha256"] == predictions["inputs_sha256"]
    labels = {(r["target_id"], r["lexicon"]): r for r in gold["labels"]}
    data = json.loads((HERE / "inputs.json").read_bytes())
    assert set(labels) == {
        (t["id"], lex) for t in data["targets"] for lex in ("kaikki", "lsj")
    }
    targets = {target["id"]: target for target in data["targets"]}
    for (target_id, lexicon), label in labels.items():
        assert label["status"] == "human_confirmed"
        native_ids = {
            sense["id"]
            for entry in data[lexicon]
            if entry["key"] == targets[target_id]["canonical_lemma"]
            for sense in entry["senses"]
        }
        assert set(label["acceptable_sense_ids"]) <= native_ids
    reports = []
    for lexicon in ("kaikki", "lsj"):
        for strategy in (
            "morphology-order",
            "projection",
            "unique-projection",
            "no-contextual-gloss",
        ):
            rows = [
                r
                for r in predictions["predictions"]
                if (r["lexicon"], r["strategy"]) == (lexicon, strategy)
            ]
            chosen = [r for r in rows if r["chosen"] is not None]
            correct = [
                r
                for r in chosen
                if r["chosen"]["id"]
                in labels[r["target_id"], lexicon]["acceptable_sense_ids"]
            ]
            wrong = [r for r in chosen if r not in correct]
            reports.append(
                {
                    "lexicon": lexicon,
                    "strategy": strategy,
                    "total": len(rows),
                    "chosen": len(chosen),
                    "correct": len(correct),
                    "wrong": len(wrong),
                    "chosen_sense_accuracy": len(correct) / len(chosen)
                    if chosen
                    else None,
                    "abstentions": len(rows) - len(chosen),
                    "abstention_rate": 1 - len(chosen) / len(rows),
                    "correct_yield": len(correct) / len(rows),
                    "failure_examples": wrong,
                    "abstention_examples": [r for r in rows if r["chosen"] is None],
                }
            )
    result = {
        "gold_sha256": hashlib.sha256(raw_gold).hexdigest(),
        "predictions_sha256": hashlib.sha256(raw_predictions).hexdigest(),
        "limits": "24 purposive targets, proposal-confirmation labels, no held-out validation or calibrated threshold; oracle alignments",
        "reports": reports,
    }
    (HERE / "results.json").write_text(
        json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    )
    for r in reports:
        print(
            r["lexicon"],
            r["strategy"],
            f"{r['correct']}/{r['chosen']} chosen correct; {r['abstentions']}/{r['total']} abstain",
        )


if __name__ == "__main__":
    main()
