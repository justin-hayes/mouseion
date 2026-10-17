"""THROWAWAY: materialise only the explicitly confirmed live-review proposals."""

import copy
import json
from pathlib import Path

root = Path(__file__).resolve().parent
ledger = json.loads((root / "review-status.json").read_text())
gold = copy.deepcopy(json.loads((root / "review-candidates.json").read_text()))
assert ledger["status"] == "live_review_complete"
assert ledger["is_complete_gold_set"]
assert not ledger["remaining_full_link_review_source_sentence_ids"]
assert set(ledger["fully_confirmed_source_sentence_ids"]) == {
    s["source_sentence_id"] for s in gold["samples"]
}
for sample in gold["samples"]:
    sid = sample["source_sentence_id"]
    events = [
        (i, event) for i, event in enumerate(ledger["events"])
        if sid in event.get("confirmed_targets", {})
    ]
    assert len(events) == 1
    index, event = events[0]
    assert set(event["confirmed_targets"][sid]) == {
        link["surface"] for link in sample["candidate_word_links"]
    }
    sample["human_review"] = {
        "reviewer": ledger["reviewer"], "ledger_event_index": index,
        "method": "Live confirmation of displayed English span(s) and all three target highlights; no corrections requested",
        "date": "2026-10-17", "status": "confirmed",
    }
    sample["gold_spans"] = sample.pop("candidate_spans")
    sample["gold_word_links"] = sample.pop("candidate_word_links")
    for link in sample["gold_word_links"]:
        link["status"] = "human_confirmed"
gold.update({
    "schema": "anabasis-alignment-gold/1", "is_gold": True,
    "status": "human_confirmed", "reviewer": ledger["reviewer"],
    "review_ledger": "review-status.json", "candidate_commit": ledger["candidate_commit"],
    "review_method": "Agent-proposed spans/links, individually confirmed by Justin in live batches; not independent annotation or expert certification",
    "evaluation_status_at_creation": "No alignment evaluation performed before completion of live human checkpoint",
})
gold["licence"] = gold["licence"].replace(
    "Extraction and unreviewed proposed correspondences by OpenAI assistant for Justin, 2026-10-17;",
    "Extraction and proposed correspondences by OpenAI assistant, confirmed in live review by Justin Hayes, 2026-10-17;",
)
(root / "gold.json").write_text(json.dumps(gold, ensure_ascii=False, indent=2) + "\n")
print("Gold materialised: 8 live-confirmed sentences, 24 target occurrences.")
