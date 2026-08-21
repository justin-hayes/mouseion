package frequency

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestPercentilesUseDeterministicBucketMidpoints(t *testing.T) {
	got, err := Percentiles([7]int{4, 3, 2, 1})
	if err != nil {
		t.Fatal(err)
	}
	want := [7]float64{0.2, 0.55, 0.8, 0.95, 1, 1, 1}
	for class := range want {
		if got[class] != want[class] {
			t.Errorf("class %d: got %v, want %v", class, got[class], want[class])
		}
	}
	if _, err = Percentiles([7]int{}); !errors.Is(err, ErrEmptyDataset) {
		t.Fatalf("empty histogram error = %v", err)
	}
}

func TestParseDWDSSnapshotCanonicalizesMapsAndRanks(t *testing.T) {
	input := `"lemma","url","wortklasse","artikeldatum","artikeltyp","frequenzklasse"
"daß","x","Konjunktion","2020","x","2"
"Haus","x","Substantiv","2020","x","6"
"laufen","x","Verb","2020","x","3"
"ohne","x","Präposition","2020","x","n/a"
`
	got, err := ParseDWDS(strings.NewReader(input), "de", "2026-08-21 13:00:46 CEST")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 3 || len(got.MissingFrequency) != 1 {
		t.Fatalf("entries=%d missing=%d", len(got.Entries), len(got.MissingFrequency))
	}
	if got.Entries[0].CanonicalLemma != "haus" || got.Entries[0].UPOS != "NOUN" || got.Entries[0].Rank != 1 || got.Entries[0].FrequencyClass != 6 {
		t.Fatalf("highest ranked entry = %+v", got.Entries[0])
	}
	if got.Entries[2].CanonicalLemma != "dass" || got.Entries[2].UPOS != "CCONJ" {
		t.Fatalf("canonical entry = %+v", got.Entries[2])
	}
}

func TestParseDWDSNormalizedGzipAndDuplicateResolution(t *testing.T) {
	input := "lemma\tupos\thaeufigkeitsklasse\tglobal_freq_percentile\tsource_version\n" +
		"daß\tSCONJ\t1\t0.1\tv1\n" +
		"dass\tSCONJ\t5\t0.9\tv1\n" +
		"Haus\tNOUN\t2\t0.5\tv1\n"
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := ParseDWDS(&compressed, "de", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || len(got.Duplicates) != 1 {
		t.Fatalf("entries=%d duplicates=%+v", len(got.Entries), got.Duplicates)
	}
	if got.Entries[0].CanonicalLemma != "dass" || got.Entries[0].FrequencyClass != 5 {
		t.Fatalf("duplicate winner = %+v", got.Entries[0])
	}
}

func TestParseDWDSRejectsMalformedRowsActionably(t *testing.T) {
	_, err := ParseDWDS(strings.NewReader("lemma,wortklasse,frequenzklasse\nHaus,Substantiv,9\nleer,,2\n"), "de", "v1")
	if err == nil || !strings.Contains(err.Error(), "row 2") || !strings.Contains(err.Error(), "integer 0-6") || !strings.Contains(err.Error(), "row 3") {
		t.Fatalf("error = %v", err)
	}
	_, err = ParseDWDS(strings.NewReader("lemma,wortklasse,frequenzklasse\nHaus,Substantiv,2\n"), "fr", "v1")
	if err == nil || !strings.Contains(err.Error(), "unsupported language") {
		t.Fatalf("unsupported language error = %v", err)
	}
}

type fakeStore struct {
	users      map[string]domain.User
	created    domain.FrequencyDataset
	entries    []domain.FrequencyEntry
	active     string
	percentile float64
	found      bool
	lookup     [3]string
}

func (f *fakeStore) GetUserByID(_ context.Context, id string) (domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, errors.New("missing")
	}
	return u, nil
}
func (f *fakeStore) CreateFrequencyDataset(_ context.Context, v domain.FrequencyDataset, entries []domain.FrequencyEntry) (domain.FrequencyDataset, error) {
	v.ID = "dataset"
	f.created, f.entries = v, entries
	return v, nil
}
func (f *fakeStore) ActivateFrequencyDataset(_ context.Context, id string) error {
	f.active = id
	return nil
}
func (f *fakeStore) DeactivateFrequencyDataset(context.Context, string) error { return nil }
func (f *fakeStore) RemoveFrequencyDataset(context.Context, string) error     { return nil }
func (f *fakeStore) GetActiveFrequencyDataset(context.Context, string) (domain.FrequencyDataset, error) {
	return f.created, nil
}
func (f *fakeStore) ListFrequencyDatasets(context.Context, string) ([]domain.FrequencyDataset, error) {
	return []domain.FrequencyDataset{f.created}, nil
}
func (f *fakeStore) FrequencyPercentile(_ context.Context, language, lemma, upos string) (float64, bool, error) {
	f.lookup = [3]string{language, lemma, upos}
	return f.percentile, f.found, nil
}

func TestServiceEnforcesAdminAndCanonicalizesLookups(t *testing.T) {
	store := &fakeStore{users: map[string]domain.User{"admin": {ID: "admin", IsAdmin: true}, "user": {ID: "user"}}, percentile: 0.97, found: true}
	service := NewService(store)
	input := "lemma,wortklasse,frequenzklasse\nHaus,Substantiv,6\n"
	if _, _, err := service.Create(context.Background(), "user", "de", "v1", strings.NewReader(input)); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("non-admin create error = %v", err)
	}
	if err := service.Activate(context.Background(), "user", "dataset"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("non-admin activate error = %v", err)
	}
	if err := service.Deactivate(context.Background(), "user", "dataset"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("non-admin deactivate error = %v", err)
	}
	if err := service.Remove(context.Background(), "user", "dataset"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("non-admin remove error = %v", err)
	}
	dataset, _, err := service.Replace(context.Background(), "admin", "de", "v1", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if !dataset.Active || store.active != dataset.ID || store.created.License != DWDSLicense || store.created.Attribution != DWDSAttribution {
		t.Fatalf("dataset=%+v store=%+v", dataset, store)
	}
	top, found, err := service.IsTopPercentile(context.Background(), "de", "daß", "sconj", 0.95)
	if err != nil || !top || !found || store.lookup != [3]string{"de", "dass", "SCONJ"} {
		t.Fatalf("top=%v found=%v lookup=%v err=%v", top, found, store.lookup, err)
	}
}
