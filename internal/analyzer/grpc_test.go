package analyzer

import (
	"context"
	"net"
	"testing"
	"time"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type analyzerService struct {
	mouseionv1.UnimplementedAnalyzerServiceServer
	want *mouseionv1.AnalyzeRequest
}

func (s *analyzerService) GetCapabilities(context.Context, *mouseionv1.GetCapabilitiesRequest) (*mouseionv1.GetCapabilitiesResponse, error) {
	return &mouseionv1.GetCapabilitiesResponse{Languages: []*mouseionv1.LanguageCapability{{
		Language: "de", DisplayName: "German", ModelVersion: "1.10.1",
		SupportedFeatures: []string{"tokenize", "pos", "lemma"}, Ready: true,
	}, {
		Language: "it", DisplayName: "Italian", ModelVersion: "1.9.2",
		SupportedFeatures: []string{"tokenize", "pos", "lemma"}, Ready: false,
	}}}, nil
}

func (s *analyzerService) Analyze(_ context.Context, request *mouseionv1.AnalyzeRequest) (*mouseionv1.NormalizedCorpus, error) {
	if !proto.Equal(request, s.want) {
		return nil, &requestMismatch{got: request}
	}
	return &mouseionv1.NormalizedCorpus{
		SchemaVersion: "1.0.0",
		Language:      request.GetLanguage(),
		Sentences: []*mouseionv1.Sentence{{
			Text:   "Goethe",
			Tokens: []*mouseionv1.Token{{Surface: "Goethe", RawLemma: "Goethe", CanonicalLemma: "goethe", Pos: "PROPN"}},
		}},
		SourceDocuments: []*mouseionv1.SourceDocument{request.GetSourceDocument()},
		Analysis: &mouseionv1.AnalysisProvenance{
			AnalyzedAt: "2026-08-21T12:00:00Z", AnalyzerName: "stub", AnalyzerVersion: "1",
		},
		NormalizationProfile: &mouseionv1.NormalizationProfile{Name: "unicode-casefold", Version: "1.0.0"},
	}, nil
}

type requestMismatch struct{ got *mouseionv1.AnalyzeRequest }

func (e *requestMismatch) Error() string { return "unexpected request: " + e.got.String() }

func TestGRPCAnalyzerRoundTrip(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	want := &mouseionv1.AnalyzeRequest{
		Language: "de", DocumentText: "Goethe",
		SourceDocument: &mouseionv1.SourceDocument{Id: "document-1", SourceIdentifier: "opds:1", Title: "Faust"},
	}
	mouseionv1.RegisterAnalyzerServiceServer(server, &analyzerService{want: want})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	analyzer, err := NewGRPCAnalyzer("passthrough:///bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = analyzer.Close() })

	result, err := analyzer.Analyze(context.Background(), AnalyzeRequest{
		Language: "de",
		Document: SourceDocument{ID: "document-1", SourceIdentifier: "opds:1", Title: "Faust", Text: "Goethe"},
	})
	require.NoError(t, err)
	assert.Equal(t, "de", result.Language)
	assert.Equal(t, time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC), result.Analysis.AnalyzedAt)
	assert.Equal(t, "goethe", result.Sentences[0].Tokens[0].CanonicalLemma)
	capabilities, err := analyzer.GetCapabilities(context.Background())
	require.NoError(t, err)
	assert.Len(t, capabilities.Languages, 2)
	assert.Equal(t, "German", capabilities.Languages[0].DisplayName)
	assert.Equal(t, "1.10.1", capabilities.Languages[0].ModelVersion)
	assert.True(t, capabilities.Languages[0].Ready)
	assert.Equal(t, "Italian", capabilities.Languages[1].DisplayName)
	assert.Equal(t, "1.9.2", capabilities.Languages[1].ModelVersion)
	assert.False(t, capabilities.Languages[1].Ready)
}

func TestGRPCAnalyzerReportsRPCFailure(t *testing.T) {
	analyzer, err := NewGRPCAnalyzer("passthrough:///unavailable",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return nil, context.DeadlineExceeded }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = analyzer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = analyzer.Analyze(ctx, AnalyzeRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "call NLP analyzer", "expected RPC error, got %v", err)
}
