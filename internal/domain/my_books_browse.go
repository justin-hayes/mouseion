package domain

type DispositionCount struct {
	Disposition BookDisposition
	Count       int
}

// MyBooksBackfillReport is the Inbox retirement backfill's result. Removed
// counts ended memberships; LanguageLessWithHistory names the Books among them
// that have reading history but no chosen language. Their membership ended
// like any removed Book's and their completions are kept, so adding the Book
// again with a language restores it.
type MyBooksBackfillReport struct {
	Removed                 int
	LanguageLessWithHistory []Book
}

type MyBooksBrowseResult struct {
	// Items is the page: every Book is one ordinary row.
	Items      []MyBook
	ScopeTotal int
	// Total counts every matching Book; paging uses it as the row count.
	Total             int
	DispositionCounts []DispositionCount
	AllCount          int
	ReadCount         int
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
