//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestScopedWorkflowGermanItalianFromAcquisitionToDownload(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "scoped-workflow-integration-secret")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "workflow-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "workflow-bob", "bob-password", false)
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "it", "Italian"); err != nil {
		t.Fatal(err)
	}

	germanBytes := scopedWorkflowEPUB("workflow-german", "German Reader", "Haus ist ein sehr schönes Buch.", "Dies ist ein zweites Kapitel mit Inhalt.")
	italianBytes := scopedWorkflowEPUB("workflow-italian", "Lettore italiano", "Libro è un esempio molto bello.", "Questo è un secondo capitolo utile.")
	revisedItalianBytes := scopedWorkflowEPUB("workflow-italian", "Lettore italiano revised", "Libro è un esempio completamente nuovo.", "Questo è un capitolo aggiornato.")
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		switch r.URL.Path {
		case "/opds", "/opds/language":
			_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Workflow catalog</title><entry><id>1</id><title>German</title><link rel="subsection" type="application/atom+xml" href="/opds/language/1"/></entry><entry><id>2</id><title>Italian</title><link rel="subsection" type="application/atom+xml" href="/opds/language/2"/></entry></feed>`)
		case "/opds/language/1":
			_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>workflow-german</id><title>German Reader</title><link rel="%s" type="%s" href="/workflow-german.epub"/></entry></feed>`, opds.AcquisitionRel, opds.EPUBMediaType)
		case "/opds/language/2":
			_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Italian</title><entry><id>workflow-italian</id><title>Lettore italiano</title><link rel="%s" type="%s" href="/workflow-italian.epub"/></entry></feed>`, opds.AcquisitionRel, opds.EPUBMediaType)
		case "/workflow-german.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(germanBytes)
		case "/workflow-italian.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(italianBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer catalog.Close()
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Workflow catalog", URL: catalog.URL + "/opds"})
	if err != nil {
		t.Fatal(err)
	}

	fake := &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		lemma, surface := "haus", "Haus"
		if req.Language == "it" {
			lemma, surface = "libro", "Libro"
		}
		end := uint64(len([]rune(req.Document.Text)))
		return analyzer.Result{
			SchemaVersion:        "1.0.0",
			Language:             req.Language,
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "workflow-fake", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
			Sentences: []analyzer.Sentence{{
				Text:     req.Document.Text,
				Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: end},
				Tokens:   []analyzer.Token{{Surface: surface, CanonicalLemma: lemma, UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: uint64(len([]rune(surface)))}}},
			}},
		}, nil
	}}
	workers := river.NewWorkers()
	analysisClient, err := analysis.NewClient(store.Pool(), fake, selection.NewService(store), workers)
	if err != nil {
		t.Fatal(err)
	}
	prepareddeck.AddBatchWorker(workers, store, cardexport.NewService(store), analysisClient, nil, prepareddeck.BatchConfig{}, false)
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: cardexport.NewService(store)})
	if err = analysisClient.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer analysisClient.Stop(context.Background())

	analysisService := analysis.NewService(store.Pool(), analysisClient)
	preparedService := prepareddeck.NewService(store, analysisClient)
	h := New(Services{
		Auth:             authService,
		WebAuth:          webauth.New(authService, false, time.Hour),
		Store:            store,
		OPDS:             opds.NewService(store, epub.NewService(store), catalog.Client()),
		Analysis:         analysisService,
		AnalysisInsights: analysisinsights.NewService(store),
		PreparedDeck:     preparedService,
		Capabilities: staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
			{Language: "de", DisplayName: "German", Ready: true},
			{Language: "it", DisplayName: "Italian", Ready: true},
		}}},
		SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, alice.Username, "alice-password")

	languagePages := make(map[string]string, 2)
	for _, language := range []string{"de", "it"} {
		page := perform(t, h, "GET", "/opds/language?connection="+connection.ID+"&language="+language, nil, cookies)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Add to My Books") {
			t.Fatalf("%s catalog page=%d %s", language, page.Code, page.Body.String())
		}
		languagePages[language] = page.Body.String()
	}
	acquire := func(language string) *httptest.ResponseRecorder {
		t.Helper()
		token := hiddenInputValue(t, languagePages[language], "acquisition")
		form := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {language}, "acquisition": {token}}
		r := httptest.NewRequest("POST", "/opds/acquire", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("HX-Request", "true")
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if response := acquire("de"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Added to My Books") {
		t.Fatalf("German acquisition=%d %s", response.Code, response.Body.String())
	}
	if response := acquire("it"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Added to My Books") {
		t.Fatalf("Italian acquisition=%d %s", response.Code, response.Body.String())
	}
	if response := acquire("de"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Already in My Books") {
		t.Fatalf("duplicate acquisition=%d %s", response.Code, response.Body.String())
	}
	books, err := store.ListSourceMaterials(ctx, alice.ID)
	if err != nil || len(books) != 2 {
		t.Fatalf("multi-add books=%d err=%v", len(books), err)
	}
	var analysisJobCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, alice.ID).Scan(&analysisJobCount); err != nil || analysisJobCount != 0 {
		t.Fatalf("acquisition created analysis jobs=%d err=%v", analysisJobCount, err)
	}
	findBook := func(identifier string) domain.SourceMaterialSummary {
		t.Helper()
		for _, book := range books {
			if book.Source.SourceIdentifier == identifier {
				return book
			}
		}
		t.Fatalf("book %q not found", identifier)
		return domain.SourceMaterialSummary{}
	}
	german := findBook("workflow-german")
	italian := findBook("workflow-italian")

	germanReview := perform(t, h, "GET", "/books/"+german.Source.ID+"/scope", nil, cookies)
	if germanReview.Code != http.StatusOK || !strings.Contains(germanReview.Body.String(), "Analysis is a separate action") {
		t.Fatalf("German review=%d %s", germanReview.Code, germanReview.Body.String())
	}
	germanSnapshot, germanUnits, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, german.Source.ID)
	if err != nil || len(germanUnits.Units) != 2 {
		t.Fatalf("German snapshot=%q units=%d err=%v", germanSnapshot, len(germanUnits.Units), err)
	}
	confirmScope := func(book domain.SourceMaterialSummary, snapshot string, units ...domain.ExtractedUnit) string {
		t.Helper()
		selected := make([]string, 0, len(units))
		for _, unit := range units {
			selected = append(selected, unit.ID)
		}
		response := perform(t, h, "POST", "/books/"+book.Source.ID+"/scope", url.Values{"csrf_token": {csrf}, "snapshot_id": {snapshot}, "unit_id": selected}, cookies)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("confirm %s=%d location=%q body=%s", book.Source.SourceIdentifier, response.Code, response.Header().Get("Location"), response.Body.String())
		}
		updated, getErr := store.ListSourceMaterials(ctx, alice.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		for _, current := range updated {
			if current.Source.ID == book.Source.ID {
				if current.ConfirmedScopeID == "" {
					t.Fatalf("scope for %s was not confirmed", book.Source.SourceIdentifier)
				}
				return current.ConfirmedScopeID
			}
		}
		t.Fatalf("confirmed book %s disappeared", book.Source.SourceIdentifier)
		return ""
	}
	germanScopeID := confirmScope(german, germanSnapshot, germanUnits.Units[0])
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, alice.ID).Scan(&analysisJobCount); err != nil || analysisJobCount != 0 {
		t.Fatalf("scope confirmation created analysis jobs=%d err=%v", analysisJobCount, err)
	}

	beforeMetadata, err := store.GetSourceMaterial(ctx, alice.ID, german.Source.ID)
	if err != nil {
		t.Fatal(err)
	}
	metadata := beforeMetadata
	metadata.Title = "German Reader renamed"
	if _, err = store.PutSourceMaterial(ctx, metadata); err != nil {
		t.Fatal(err)
	}
	afterMetadata, err := store.GetSourceMaterial(ctx, alice.ID, german.Source.ID)
	if err != nil || afterMetadata.Title != metadata.Title || afterMetadata.ContentRevisionID != beforeMetadata.ContentRevisionID || afterMetadata.ContentDigest != beforeMetadata.ContentDigest {
		t.Fatalf("metadata edit changed content identity: before=%+v after=%+v err=%v", beforeMetadata, afterMetadata, err)
	}

	if response := perform(t, h, "POST", "/books/"+german.Source.ID+"/analyze", nil, cookies); response.Code != http.StatusForbidden {
		t.Fatalf("analysis without csrf=%d", response.Code)
	}
	if response := perform(t, h, "POST", "/books/"+german.Source.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies); response.Code != http.StatusSeeOther {
		t.Fatalf("explicit analysis=%d %s", response.Code, response.Body.String())
	}
	duplicates := make(chan analysis.Handle, 8)
	errorsCh := make(chan error, 8)
	var submissions sync.WaitGroup
	for i := 0; i < 8; i++ {
		submissions.Add(1)
		go func() {
			defer submissions.Done()
			handle, submitErr := analysisService.SubmitScopedAnalysis(ctx, alice.ID, german.Source.ID, germanScopeID)
			if submitErr != nil {
				errorsCh <- submitErr
				return
			}
			duplicates <- handle
		}()
	}
	submissions.Wait()
	close(duplicates)
	close(errorsCh)
	var germanHandle analysis.Handle
	for submitErr := range errorsCh {
		t.Fatalf("concurrent analysis submission: %v", submitErr)
	}
	for handle := range duplicates {
		if germanHandle.ID == 0 {
			germanHandle = handle
		} else if handle.ID != germanHandle.ID || handle.RunID != germanHandle.RunID {
			t.Fatalf("duplicate analysis handle=%+v first=%+v", handle, germanHandle)
		}
	}
	if germanHandle.ID == 0 {
		t.Fatal("concurrent duplicate submissions returned no handle")
	}
	status, err := analysisService.Wait(ctx, alice.ID, germanHandle.ID)
	if err != nil || status.State != rivertype.JobStateCompleted || status.ScopeID != germanScopeID || status.RunID == "" {
		t.Fatalf("German analysis status=%+v err=%v", status, err)
	}
	var riverJobCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='analyze_corpus' AND args->>'run_id'=$1`, status.RunID).Scan(&riverJobCount); err != nil || riverJobCount != 1 {
		t.Fatalf("duplicate River analysis jobs=%d err=%v", riverJobCount, err)
	}
	bobCookies, bobCSRF := loginCookies(t, h, bob.Username, "bob-password")
	jobPage := perform(t, h, "GET", fmt.Sprintf("/jobs/%d", germanHandle.ID), nil, cookies)
	if jobPage.Code != http.StatusOK || !strings.Contains(jobPage.Body.String(), "Completed") || !strings.Contains(jobPage.Body.String(), "View analysis result") || !strings.Contains(jobPage.Body.String(), `href="/books/`+german.Source.ID+`"`) {
		t.Fatalf("completed analysis page=%d %s", jobPage.Code, jobPage.Body.String())
	}
	resultPage := perform(t, h, "GET", fmt.Sprintf("/books/%s/analyses/%s", german.Source.ID, status.RunID), nil, cookies)
	if resultPage.Code != http.StatusSeeOther || resultPage.Header().Get("Location") != "/books/"+german.Source.ID {
		t.Fatalf("exact German redirect=%d location=%q %s", resultPage.Code, resultPage.Header().Get("Location"), resultPage.Body.String())
	}
	if bobResult := perform(t, h, "GET", fmt.Sprintf("/books/%s/analyses/%s", german.Source.ID, status.RunID), nil, bobCookies); bobResult.Code != http.StatusNotFound {
		t.Fatalf("cross-owner exact result=%d %s", bobResult.Code, bobResult.Body.String())
	}
	if mismatched := perform(t, h, "GET", fmt.Sprintf("/books/%s/analyses/%s", italian.Source.ID, status.RunID), nil, cookies); mismatched.Code != http.StatusNotFound {
		t.Fatalf("mismatched-book exact result=%d %s", mismatched.Code, mismatched.Body.String())
	}
	if missing := perform(t, h, "GET", fmt.Sprintf("/books/%s/analyses/%s", german.Source.ID, uuid.NewString()), nil, cookies); missing.Code != http.StatusNotFound {
		t.Fatalf("missing exact result=%d %s", missing.Code, missing.Body.String())
	}
	if err = store.Pool().QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, alice.ID, status.RunID).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET state='running' WHERE owner_id=$1 AND id=$2`, alice.ID, status.RunID); err != nil {
		t.Fatal(err)
	}
	if nonCompleted := perform(t, h, "GET", fmt.Sprintf("/books/%s/analyses/%s", german.Source.ID, status.RunID), nil, cookies); nonCompleted.Code != http.StatusNotFound {
		t.Fatalf("non-completed exact result=%d %s", nonCompleted.Code, nonCompleted.Body.String())
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET state='completed' WHERE owner_id=$1 AND id=$2`, alice.ID, status.RunID); err != nil {
		t.Fatal(err)
	}
	currentBookPage := perform(t, h, "GET", "/books/"+german.Source.ID, nil, cookies)
	if currentBookPage.Code != http.StatusOK || !strings.Contains(currentBookPage.Body.String(), `action="/books/`+german.Source.ID+`/deck/preparations"`) {
		t.Fatalf("current book preparation surface=%d %s", currentBookPage.Code, currentBookPage.Body.String())
	}
	bookPreparation := perform(t, h, "POST", fmt.Sprintf("/books/%s/deck/preparations", german.Source.ID), url.Values{"csrf_token": {csrf}}, cookies)
	if bookPreparation.Code != http.StatusSeeOther || !strings.HasPrefix(bookPreparation.Header().Get("Location"), "/deck-preparations/") {
		t.Fatalf("book preparation=%d location=%q %s", bookPreparation.Code, bookPreparation.Header().Get("Location"), bookPreparation.Body.String())
	}
	if crossOwnerBookPreparation := perform(t, h, "POST", fmt.Sprintf("/books/%s/deck/preparations", german.Source.ID), url.Values{"csrf_token": {bobCSRF}}, bobCookies); crossOwnerBookPreparation.Code != http.StatusNotFound {
		t.Fatalf("cross-owner book preparation=%d %s", crossOwnerBookPreparation.Code, crossOwnerBookPreparation.Body.String())
	}
	resultPreparation := perform(t, h, "POST", fmt.Sprintf("/books/%s/analyses/%s/deck/preparations", german.Source.ID, status.RunID), url.Values{"csrf_token": {csrf}}, cookies)
	if resultPreparation.Code != http.StatusSeeOther || !strings.HasPrefix(resultPreparation.Header().Get("Location"), "/deck-preparations/") {
		t.Fatalf("exact result preparation=%d location=%q %s", resultPreparation.Code, resultPreparation.Header().Get("Location"), resultPreparation.Body.String())
	}
	if crossOwnerPreparation := perform(t, h, "POST", fmt.Sprintf("/books/%s/analyses/%s/deck/preparations", german.Source.ID, status.RunID), url.Values{"csrf_token": {bobCSRF}}, bobCookies); crossOwnerPreparation.Code != http.StatusNotFound {
		t.Fatalf("cross-owner exact preparation=%d %s", crossOwnerPreparation.Code, crossOwnerPreparation.Body.String())
	}
	bookPage := perform(t, h, "GET", "/books/"+german.Source.ID, nil, cookies)
	if bookPage.Code != http.StatusOK || !strings.Contains(bookPage.Body.String(), "Vocabulary coverage") || strings.Contains(bookPage.Body.String(), "Analyzed scope") || strings.Contains(bookPage.Body.String(), germanScopeID) || !strings.Contains(bookPage.Body.String(), "German Reader renamed") {
		t.Fatalf("German insights=%d %s", bookPage.Code, bookPage.Body.String())
	}

	if response := perform(t, h, "POST", fmt.Sprintf("/jobs/%d/deck/preparations", germanHandle.ID), nil, cookies); response.Code != http.StatusForbidden {
		t.Fatalf("preparation without csrf=%d", response.Code)
	}
	if response := perform(t, h, "POST", "/jobs/999999/deck/preparations", url.Values{"csrf_token": {csrf}}, cookies); response.Code != http.StatusBadRequest {
		t.Fatalf("forged analysis ID preparation=%d %s", response.Code, response.Body.String())
	}
	if response := perform(t, h, "POST", fmt.Sprintf("/jobs/%d/deck/preparations", germanHandle.ID), url.Values{"csrf_token": {bobCSRF}}, bobCookies); response.Code == http.StatusSeeOther {
		t.Fatalf("cross-owner preparation was accepted")
	}
	preparationResponse := perform(t, h, "POST", fmt.Sprintf("/jobs/%d/deck/preparations", germanHandle.ID), url.Values{"csrf_token": {csrf}}, cookies)
	if preparationResponse.Code != http.StatusSeeOther {
		t.Fatalf("preparation=%d %s", preparationResponse.Code, preparationResponse.Body.String())
	}
	preparationID := strings.TrimPrefix(preparationResponse.Header().Get("Location"), "/deck-preparations/")
	preparationID = strings.TrimSuffix(preparationID, "/status")
	preparation, err := waitForPreparation(ctx, preparedService, alice.ID, preparationID)
	if err != nil || preparation.State != domain.DeckPreparationReady || preparation.AnalysisRunID != status.RunID {
		t.Fatalf("preparation=%+v err=%v", preparation, err)
	}
	for i := 0; i < 2; i++ {
		download := perform(t, h, "GET", "/deck-preparations/"+preparationID+"/download", nil, cookies)
		if download.Code != http.StatusOK || len(download.Body.Bytes()) == 0 || download.Header().Get("X-Mouseion-Analysis-Run-ID") != status.RunID || !strings.Contains(download.Header().Get("Content-Disposition"), "German Reader renamed.apkg") {
			t.Fatalf("download %d=%d headers=%v bytes=%d", i, download.Code, download.Header(), len(download.Body.Bytes()))
		}
	}
	if response := perform(t, h, "GET", "/deck-preparations/"+preparationID+"/download", nil, bobCookies); response.Code != http.StatusNotFound {
		t.Fatalf("cross-owner download=%d", response.Code)
	}

	priorReview := perform(t, h, "GET", "/books/"+german.Source.ID+"/scope", nil, cookies)
	if priorReview.Code != http.StatusOK || !strings.Contains(priorReview.Body.String(), "Check all") || strings.Contains(priorReview.Body.String(), "Scope comparison") {
		t.Fatalf("new scope review=%d %s", priorReview.Code, priorReview.Body.String())
	}
	secondScopeID := confirmScope(german, germanSnapshot, germanUnits.Units...)
	if secondScopeID == germanScopeID {
		t.Fatal("changed selection reused historical scope")
	}
	if response := perform(t, h, "POST", "/books/"+german.Source.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies); response.Code != http.StatusSeeOther {
		t.Fatalf("second analysis=%d %s", response.Code, response.Body.String())
	}
	var secondJobID int64
	if err = store.Pool().QueryRow(ctx, `SELECT river_job_id FROM analysis_jobs WHERE owner_id=$1 AND reviewed_scope_id=$2 ORDER BY created_at DESC LIMIT 1`, alice.ID, secondScopeID).Scan(&secondJobID); err != nil {
		t.Fatal(err)
	}
	secondStatus, err := analysisService.Wait(ctx, alice.ID, secondJobID)
	if err != nil || secondStatus.State != rivertype.JobStateCompleted {
		t.Fatalf("second analysis=%+v err=%v", secondStatus, err)
	}
	var corpusCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM corpora WHERE owner_id=$1 AND source_material_id=$2 AND reviewed_scope_id IS NOT NULL`, alice.ID, german.Source.ID).Scan(&corpusCount); err != nil || corpusCount != 2 {
		t.Fatalf("historical corpus count=%d err=%v", corpusCount, err)
	}
	if _, err = analysisService.Result(ctx, alice.ID, germanHandle.ID); err != nil {
		t.Fatalf("historical first analysis unreadable: %v", err)
	}

	italianSnapshot, italianUnits, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, italian.Source.ID)
	if err != nil {
		t.Fatal(err)
	}
	italianScopeID := confirmScope(italian, italianSnapshot, italianUnits.Units[0])
	if _, err = epub.NewService(store).Import(ctx, alice.ID, "it", revisedItalianBytes); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetEPUBReviewedScope(ctx, alice.ID, italian.Source.ID, italianScopeID); err != nil {
		t.Fatalf("historical Italian scope became unreadable: %v", err)
	}
	if _, err = analysisService.SubmitScopedAnalysis(ctx, alice.ID, italian.Source.ID, italianScopeID); err == nil {
		t.Fatal("stale Italian scope was accepted after changed content")
	}
	newItalianSnapshot, newItalianUnits, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, italian.Source.ID)
	if err != nil || newItalianSnapshot == italianSnapshot {
		t.Fatalf("Italian content revision did not create a new snapshot: old=%q new=%q err=%v", italianSnapshot, newItalianSnapshot, err)
	}
	italian = findCurrentBook(t, ctx, store, alice.ID, "workflow-italian")
	newItalianScopeID := confirmScope(italian, newItalianSnapshot, newItalianUnits.Units[0])
	if newItalianScopeID == italianScopeID {
		t.Fatal("changed Italian content reused historical scope")
	}

	legacy, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "workflow-legacy", Title: "Legacy EPUB", MediaType: "application/epub+zip", ContentHash: "legacy-workflow", Content: []byte("legacy bytes"), FullText: "Legacy full text."})
	if err != nil {
		t.Fatal(err)
	}
	legacyHandle, err := analysisService.SubmitAnalysis(ctx, alice.ID, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacyStatus, err := analysisService.Wait(ctx, alice.ID, legacyHandle.ID)
	if err != nil || legacyStatus.State != rivertype.JobStateCompleted {
		t.Fatalf("legacy analysis=%+v err=%v", legacyStatus, err)
	}
	legacyPage := perform(t, h, "GET", fmt.Sprintf("/jobs/%d", legacyHandle.ID), nil, cookies)
	if legacyPage.Code != http.StatusOK || !strings.Contains(legacyPage.Body.String(), "Legacy/full-text results cannot unlock a new deck preparation") {
		t.Fatalf("legacy history page=%d %s", legacyPage.Code, legacyPage.Body.String())
	}
}

func waitForPreparation(ctx context.Context, service *prepareddeck.Service, owner, id string) (domain.DeckPreparation, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		preparation, err := service.Get(ctx, owner, id)
		if err != nil {
			return domain.DeckPreparation{}, err
		}
		if preparation.State == domain.DeckPreparationReady || preparation.State == domain.DeckPreparationFailed || preparation.State == domain.DeckPreparationCancelled {
			return preparation, nil
		}
		select {
		case <-ctx.Done():
			return domain.DeckPreparation{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func findCurrentBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, identifier string) domain.SourceMaterialSummary {
	t.Helper()
	books, err := store.ListSourceMaterials(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range books {
		if book.Source.SourceIdentifier == identifier {
			return book
		}
	}
	t.Fatalf("book %q not found", identifier)
	return domain.SourceMaterialSummary{}
}

func scopedWorkflowEPUB(identifier, title, firstText, secondText string) []byte {
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	files := map[string]string{
		"mimetype":                "application/epub+zip",
		"META-INF/container.xml":  `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`,
		"OEBPS/content.opf":       fmt.Sprintf(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">%s</dc:identifier><dc:title>%s</dc:title></metadata><manifest><item id="chapter-one" href="chapter-one.xhtml" media-type="application/xhtml+xml"/><item id="chapter-two" href="chapter-two.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter-one"/><itemref idref="chapter-two"/></spine></package>`, identifier, title),
		"OEBPS/chapter-one.xhtml": fmt.Sprintf(`<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Chapter One</h1><p>%s</p></body></html>`, firstText),
		"OEBPS/chapter-two.xhtml": fmt.Sprintf(`<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Chapter Two</h1><p>%s</p></body></html>`, secondText),
	}
	for name, content := range files {
		file, err := archive.Create(name)
		if err != nil {
			return nil
		}
		if _, err = file.Write([]byte(content)); err != nil {
			return nil
		}
	}
	if err := archive.Close(); err != nil {
		return nil
	}
	return output.Bytes()
}
