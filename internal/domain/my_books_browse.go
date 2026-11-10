package domain

// LanguageCount is one owner-scoped language pill count. Tag is "unknown"
// for books whose language state is unknown.
type LanguageCount struct {
	Tag   string
	Count int
}

type DispositionCount struct {
	Disposition BookDisposition
	Count       int
}

// MyBooksBrowseRow is one visible My Books row. An ordinary row has one Book;
// a grouped Work row has the matching Books of that Work in constituent-book
// order.
type MyBooksBrowseRow struct {
	Key        string
	Grouped    bool
	WorkKey    string
	WorkTitle  string
	WorkAuthor string
	GreekTitle string
	// ConstituentBooks is the Work's reviewed total; OwnedBooks is how many of
	// them the learner owns in the active language, including Hidden Books.
	ConstituentBooks int
	OwnedBooks       int
	Books            []MyBook
	// WorkUpdateBooks carries every owned Book of a grouped Work, including
	// Books outside the current filter, so the Work-level Analysis update
	// notice and bulk action count the whole Work just as its other bulk
	// actions do. Ordinary rows leave it nil.
	WorkUpdateBooks []MyBook
}

type MyBooksBrowseResult struct {
	Items []MyBook
	// Rows is the page as rendered: ordinary rows and grouped Work rows. Items
	// is the same page flattened to Books for callers that only need Book
	// identity.
	Rows       []MyBooksBrowseRow
	ScopeTotal int
	// Total counts every matching Book. BookTotal counts the matching Books
	// that belong to a reviewed Work, so it pairs with WorkTotal for the shape
	// line. RowCount is the rendered row count paging uses.
	Total             int
	BookTotal         int
	WorkTotal         int
	RowCount          int
	HasWorks          bool
	Counts            []LanguageCount
	DispositionCounts []DispositionCount
	AllCount          int
	HiddenCount       int
	ReadCount         int
}

// MyBookWorkGroupKey is the row-grouping key: one Work's Books share a key, and
// an ordinary Book is its own group.
func MyBookWorkGroupKey(book MyBook) string {
	if unit := book.ReadingUnit; unit != nil && unit.WorkKey != "" {
		return "work:" + unit.WorkKey
	}
	return "book:" + book.Book.ID
}

// MyBookWorkOrderLess orders two Books of one Work by reviewed constituent
// number, then Book identity.
func MyBookWorkOrderLess(a, b MyBook) bool {
	an, bn := 0, 0
	if a.ReadingUnit != nil {
		an = a.ReadingUnit.ConstituentBook
	}
	if b.ReadingUnit != nil {
		bn = b.ReadingUnit.ConstituentBook
	}
	if an != bn {
		return an < bn
	}
	return a.Book.ID < b.Book.ID
}
