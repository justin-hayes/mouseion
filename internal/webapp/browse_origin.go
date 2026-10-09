package webapp

import (
	"encoding/hex"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Browse origin query keys. An origin is the Reading Browse excursion a
// Concordance or Study page came from. It is carried beside, never inside, the
// lookup interpretation and captured priority, and it grants no authority: every
// request that consumes it revalidates owner, language, commitment, and evidence.
const (
	originKeyBook     = "from_book"
	originKeySnapshot = "from_snap"
	originKeyPrefix   = "from_q"
	originKeyAll      = "from_all"
	originKeyPage     = "from_page"
	originKeyRow      = "from_row"
	originKeyRevision = "from_rev"

	// maxOriginValueLength bounds every carried value so a hand-built link cannot
	// inflate each page and form that propagates it.
	maxOriginValueLength = 512
)

var originKeys = []string{originKeyBook, originKeySnapshot, originKeyPrefix, originKeyAll, originKeyPage, originKeyRow, originKeyRevision}

// browseOrigin names the exact Current reading commitment (Book and snapshot),
// the applied Browse controls, the row the learner left from, and the live
// Browse evidence revision at that time. The snapshot proves commitment only; the
// revision is the freshness guard. The study language travels as the ordinary
// language parameter.
type browseOrigin struct {
	BookID, SnapshotID string
	Prefix             string
	IncludeAll         bool
	Page               int
	Row                string
	Revision           string
}

// Active reports whether this origin identifies a commitment. A direct
// Concordance visit has none, and none is ever fabricated.
func (o browseOrigin) Active() bool { return o.BookID != "" && o.SnapshotID != "" }

// hasOriginKeys reports whether a request carries any origin parameter.
func hasOriginKeys(values url.Values) bool {
	return slices.ContainsFunc(originKeys, values.Has)
}

// parseBrowseOrigin reads a syntactically valid origin. Any malformed or
// partial origin is dropped whole rather than repaired or partly honored.
func parseBrowseOrigin(values url.Values) browseOrigin {
	for _, key := range originKeys {
		if len(values.Get(key)) > maxOriginValueLength {
			return browseOrigin{}
		}
	}
	origin := browseOrigin{
		BookID: strings.TrimSpace(values.Get(originKeyBook)), SnapshotID: strings.TrimSpace(values.Get(originKeySnapshot)),
		Prefix: values.Get(originKeyPrefix), Row: values.Get(originKeyRow), Revision: values.Get(originKeyRevision), Page: 1,
	}
	switch values.Get(originKeyAll) {
	case "":
	case "1":
		origin.IncludeAll = true
	default:
		return browseOrigin{}
	}
	if raw := values.Get(originKeyPage); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return browseOrigin{}
		}
		origin.Page = page
	}
	if !origin.Active() {
		return browseOrigin{}
	}
	return origin
}

// Values returns the origin parameters, or none for an inactive origin.
func (o browseOrigin) Values() url.Values {
	values := url.Values{}
	if !o.Active() {
		return values
	}
	values.Set(originKeyBook, o.BookID)
	values.Set(originKeySnapshot, o.SnapshotID)
	if o.Prefix != "" {
		values.Set(originKeyPrefix, o.Prefix)
	}
	if o.IncludeAll {
		values.Set(originKeyAll, "1")
	}
	if o.Page > 1 {
		values.Set(originKeyPage, strconv.Itoa(o.Page))
	}
	if o.Row != "" {
		values.Set(originKeyRow, o.Row)
	}
	if o.Revision != "" {
		values.Set(originKeyRevision, o.Revision)
	}
	return values
}

// Apply appends the origin to a local URL, keeping any fragment. An inactive
// origin leaves the URL unchanged.
func (o browseOrigin) Apply(raw string) string {
	if !o.Active() {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	maps.Copy(query, o.Values())
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// ReturnURL is Back to book vocabulary. Reading revalidates the commitment,
// language, and evidence revision before showing anything.
func (o browseOrigin) ReturnURL(language string) string {
	values := url.Values{}
	if language != "" {
		values.Set("language", language)
	}
	if o.Prefix != "" {
		values.Set("q", o.Prefix)
	}
	if o.IncludeAll {
		values.Set("all", "1")
	}
	values.Set("reading", o.BookID)
	values.Set("snapshot", o.SnapshotID)
	if o.Revision != "" {
		values.Set("rev", o.Revision)
	}
	if o.Row != "" {
		values.Set("row", o.Row)
	}
	values.Set("page", strconv.Itoa(max(o.Page, 1)))
	return "/reading?" + values.Encode()
}

// browseRowKey identifies a Browse row by its effective identity.
func browseRowKey(upos, lemma string) string { return upos + ":" + lemma }

// browseRowElementID is the stable anchor of a Browse row.
func browseRowElementID(key string) string {
	return "browse-row-" + hex.EncodeToString([]byte(key))
}
