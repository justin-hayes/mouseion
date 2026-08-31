package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func renderCampaignPage(t *testing.T, campaigns []campaignView, prepared []preparedCampaignOption, message, pageError, activeCampaignID string) string {
	t.Helper()
	var output bytes.Buffer
	if err := CampaignsPage(domain.User{ID: "owner-1", Username: "learner"}, "csrf-token", campaigns, prepared, message, pageError, activeCampaignID).Render(context.Background(), &output); err != nil {
		t.Fatalf("render campaigns page: %v", err)
	}
	return output.String()
}

func testCampaign(id string, book domain.BookProgress, deck domain.DeckProgress) campaignView {
	return campaignView{
		Campaign: domain.LearningCampaign{ID: id, SourceMaterialID: "book-" + id, DeckPreparationID: "deck-" + id, BookProgress: book, DeckProgress: deck, Status: domain.DeriveCampaignStatus(book, deck)},
		Book:     domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-" + id, Title: "Book " + id, Language: "de"}},
		Deck:     domain.DeckPreparation{ID: "deck-" + id, DeckName: "Mouseion::de::Book " + id},
	}
}

func testQueuedCampaign(id string, position int) campaignView {
	campaign := testCampaign(id, domain.BookQueued, domain.DeckQueued)
	campaign.QueuePosition = position
	return campaign
}

func TestCampaignsPageRendersQueuePositionsAndPreparedActions(t *testing.T) {
	prepared := []preparedCampaignOption{{
		Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "prepared-book", Title: "Prepared Book"}},
		Deck: domain.DeckPreparation{ID: "prepared-deck", DeckName: "Mouseion::de::Prepared Book", TotalCards: 9},
	}}
	for _, test := range []struct {
		name           string
		campaigns      []campaignView
		queueWords     []string
		queuePositions []string
	}{
		{name: "zero", campaigns: nil},
		{name: "one", campaigns: []campaignView{testQueuedCampaign("one", 1)}, queueWords: []string{"Book one", "Start learning"}, queuePositions: []string{"Queue position:</strong> <span class=\"numeric\">1</span>"}},
		{name: "many", campaigns: []campaignView{testQueuedCampaign("one", 1), testQueuedCampaign("two", 2), testQueuedCampaign("three", 3)}, queueWords: []string{"Book one", "Book two", "Book three"}, queuePositions: []string{"Queue position:</strong> <span class=\"numeric\">1</span>", "Queue position:</strong> <span class=\"numeric\">2</span>", "Queue position:</strong> <span class=\"numeric\">3</span>"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderCampaignPage(t, test.campaigns, prepared, "", "", "")
			if !strings.Contains(html, `<h2 id="campaign-queue-heading">Queue</h2>`) || !strings.Contains(html, "Prepared books and decks ready to add") || !strings.Contains(html, `method="post" action="/campaigns"`) {
				t.Fatalf("queue/prepared hierarchy missing: %s", html)
			}
			for _, word := range test.queueWords {
				if !strings.Contains(html, word) {
					t.Errorf("page missing %q", word)
				}
			}
			for _, position := range test.queuePositions {
				if !strings.Contains(html, position) {
					t.Errorf("page missing queue position %q", position)
				}
			}
			if test.name == "zero" && !strings.Contains(html, "Your learning queue is empty.") {
				t.Error("zero queue does not explain its empty state")
			}
		})
	}
}

func TestCampaignsPageRendersIndependentActiveProgressPermutations(t *testing.T) {
	for _, test := range []struct {
		name      string
		book      domain.BookProgress
		deck      domain.DeckProgress
		remaining []string
		complete  bool
	}{
		{name: "both incomplete", book: domain.BookReading, deck: domain.DeckStudying, remaining: []string{"Mark book finished — campaign remains active", "Mark deck reviewed — campaign remains active"}},
		{name: "book finished", book: domain.BookFinished, deck: domain.DeckStudying, remaining: []string{"Book: Finished", "Deck: Studying", "Complete campaign and add its vocabulary to known"}, complete: true},
		{name: "deck reviewed", book: domain.BookReading, deck: domain.DeckReviewed, remaining: []string{"Book: Reading", "Deck: Reviewed", "Complete campaign and add its vocabulary to known"}, complete: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderCampaignPage(t, []campaignView{testCampaign("active", test.book, test.deck)}, nil, "", "", "")
			if !strings.Contains(html, "Active campaign") || !strings.Contains(html, "Remaining before completion") || !strings.Contains(html, "Book") || !strings.Contains(html, "Deck") {
				t.Fatalf("active hierarchy/progress missing: %s", html)
			}
			for _, expected := range test.remaining {
				if !strings.Contains(html, expected) {
					t.Errorf("active page missing %q", expected)
				}
			}
			if test.complete != strings.Contains(html, "Complete campaign and add its vocabulary to known") {
				t.Errorf("completion confirmation presence=%t html=%s", strings.Contains(html, "Complete campaign and add its vocabulary to known"), html)
			}
		})
	}
}

func TestCampaignsPageRendersCompletionOutcomeWithAndWithoutCount(t *testing.T) {
	count := 132
	withCount := testCampaign("counted", domain.BookFinished, domain.DeckStudying)
	withCount.GraduatableLemmaCount = &count
	withoutCount := testCampaign("uncounted", domain.BookFinished, domain.DeckStudying)

	withCountHTML := renderCampaignPage(t, []campaignView{withCount}, nil, "", "", "")
	for _, expected := range []string{
		"Complete campaign and add 132 lemmas to known vocabulary",
		"Book</dt><dd>Book counted — Finished",
		"Prepared deck</dt><dd>Mouseion::de::Book counted — Reviewed",
		"Generated provenance remains attached",
		"Coverage and future queue projections will be recalculated",
		"not a mastery claim or a spaced-repetition grade",
		"Completion cannot currently be undone",
		"name=\"csrf_token\"",
	} {
		if !strings.Contains(withCountHTML, expected) {
			t.Errorf("counted confirmation missing %q: %s", expected, withCountHTML)
		}
	}
	if withoutCountHTML := renderCampaignPage(t, []campaignView{withoutCount}, nil, "", "", ""); !strings.Contains(withoutCountHTML, "Complete campaign and add its vocabulary to known") {
		t.Errorf("count-free confirmation did not use fallback: %s", withoutCountHTML)
	}
}

func TestCampaignsPageRendersAbandonmentAndRecoveryStates(t *testing.T) {
	active := testCampaign("active", domain.BookReading, domain.DeckStudying)
	html := renderCampaignPage(t, []campaignView{active}, nil, "", "", "")
	if !strings.Contains(html, `name="expected_book_progress"`) || !strings.Contains(html, `name="expected_deck_progress"`) {
		t.Errorf("active abandonment form is missing expected progress state: %s", html)
	}
	queuedHTML := renderCampaignPage(t, []campaignView{testCampaign("queued", domain.BookQueued, domain.DeckQueued)}, nil, "", "", "")
	for _, expected := range []string{`action="/campaigns/queued/abandon"`, "This queued campaign has no active reservation to release", "Confirm abandonment", `value="queued"`} {
		if !strings.Contains(queuedHTML, expected) {
			t.Errorf("queued abandonment UI missing %q: %s", expected, queuedHTML)
		}
	}
	for _, expected := range []string{"Abandon campaign", "without deleting it", "book, prepared deck, generated provenance, and campaign history remain", "active reservation is released", "eligible for future decks again", `action="/campaigns/active/abandon"`, `name="expected_campaign_status"`} {
		if !strings.Contains(html, expected) {
			t.Errorf("abandonment confirmation missing %q: %s", expected, html)
		}
	}

	for _, test := range []struct {
		name     string
		message  string
		campaign string
		want     []string
	}{
		{name: "blocked activation", message: "Finish or abandon the active campaign before starting another.", campaign: "active", want: []string{"Action needed", "Finish or abandon the active campaign", `href="#campaign-active"`, "Return to the active campaign"}},
		{name: "stale submission", message: "This campaign changed since this page was loaded. Its current state is unchanged by this request; review Reading Journey before trying again.", want: []string{"Action needed", "changed since this page was loaded", "current state is unchanged"}},
		{name: "mutation failure", message: "Campaign progress could not be updated. No progress was changed; review Reading Journey and try again.", want: []string{"Action needed", "No progress was changed", "review Reading Journey and try again"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderCampaignPage(t, []campaignView{active}, nil, "", test.message, test.campaign)
			for _, expected := range test.want {
				if !strings.Contains(html, expected) {
					t.Errorf("recovery page missing %q: %s", expected, html)
				}
			}
		})
	}
}

func TestCampaignsPageKeepsLearningHierarchyInDocumentOrder(t *testing.T) {
	active := testCampaign("active", domain.BookReading, domain.DeckStudying)
	queued := testQueuedCampaign("queued", 1)
	completed := testCampaign("completed", domain.BookFinished, domain.DeckReviewed)
	abandoned := testCampaign("abandoned", domain.BookAbandoned, domain.DeckAbandoned)
	prepared := []preparedCampaignOption{{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{Title: "Prepared"}}, Deck: domain.DeckPreparation{ID: "prepared", DeckName: "Prepared deck"}}}
	html := renderCampaignPage(t, []campaignView{active, queued, completed, abandoned}, prepared, "", "", "")
	positions := []struct {
		name string
		text string
	}{
		{name: "active", text: `id="active-campaign-heading"`},
		{name: "remaining", text: "Remaining before completion"},
		{name: "consequence", text: "When both conditions are finished"},
		{name: "prepared actions", text: "Prepared books and decks ready to add"},
		{name: "queue", text: `id="campaign-queue-heading"`},
		{name: "history", text: `id="campaign-history-heading"`},
	}
	previous := -1
	for _, position := range positions {
		current := strings.Index(html, position.text)
		if current <= previous {
			t.Fatalf("%s section is out of order: position=%d previous=%d", position.name, current, previous)
		}
		previous = current
	}
	for _, expected := range []string{"Queued behind the single active campaign", "Assigned vocabulary was graduated to known vocabulary", "Ungraduated vocabulary is eligible again"} {
		if !strings.Contains(html, expected) {
			t.Errorf("history/queue state missing %q", expected)
		}
	}
}
