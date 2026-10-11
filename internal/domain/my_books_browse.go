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

type MyBooksBrowseResult struct {
	Items             []MyBook
	ScopeTotal        int
	Total             int
	Counts            []LanguageCount
	DispositionCounts []DispositionCount
	AllCount          int
	HiddenCount       int
	ReadCount         int
}
