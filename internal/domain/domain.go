// Package domain contains the core corpus and vocabulary models.
package domain

import "time"

type User struct {
	ID, Username string
	IsAdmin      bool
	CreatedAt    time.Time
}
type LanguageProfile struct {
	ID, OwnerID, Language, DisplayName string
	CreatedAt                          time.Time
}
type SourceMaterial struct {
	ID, OwnerID, Language, SourceIdentifier, Title, MediaType, ContentHash, FullText string
	Content                                                                          []byte
	CreatedAt                                                                        time.Time
}
type OpdsConnection struct {
	ID, OwnerID, Name, URL, Username, Password, Language string
	CreatedAt, UpdatedAt                                 time.Time
}
type Corpus struct {
	ID, OwnerID, SourceMaterialID, ArtifactHash, Status string
	CreatedAt                                           time.Time
}
type NormalizedArtifact struct {
	ContentHash, Language, SchemaVersion, NormalizationProfile, NormalizationVersion, AnalyzerName, AnalyzerVersion string
	CreatedAt                                                                                                       time.Time
}
type SharedLemma struct {
	ContentHash, Language, CanonicalLemma, UPOS string
	Morphology                                  []byte
	Frequency                                   int64
}
type KnownVocabulary struct {
	ID, OwnerID, Language, CanonicalLemma, UPOS string
	CreatedAt                                   time.Time
}
type VocabularyState struct {
	ID, OwnerID, Language, CanonicalLemma, UPOS, State string
	UpdatedAt                                          time.Time
}
type ExampleSentence struct {
	ID, OwnerID, CorpusID, SentenceKey, Text string
	SourceLocation                           []byte
	CreatedAt                                time.Time
}
type CuratedSentence struct {
	ID, OwnerID, ExampleSentenceID, Language, CanonicalLemma, UPOS, Notes string
	CreatedAt                                                             time.Time
}
type Deck struct {
	ID, OwnerID, Language, Name string
	CreatedAt                   time.Time
}
type Card struct {
	ID, OwnerID, DeckID, DedupKey, CanonicalLemma, UPOS, Front, Back string
	CreatedAt                                                        time.Time
}
type ProcessingHistory struct {
	ID, OwnerID, CorpusID, Operation, Status string
	Details                                  []byte
	StartedAt                                time.Time
	CompletedAt                              *time.Time
}
type FrequencyDataset struct {
	ID, Language, Name, Version string
	CreatedAt                   time.Time
}
