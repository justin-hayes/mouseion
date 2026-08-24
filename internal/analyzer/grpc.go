package analyzer

import (
	"context"
	"fmt"
	"os"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultNLPAddress = "localhost:50051"

// GRPCAnalyzer calls the long-lived Python NLP service over gRPC.
type GRPCAnalyzer struct {
	conn   *grpc.ClientConn
	client mouseionv1.AnalyzerServiceClient
}

// NewGRPCAnalyzer creates an analyzer for target. When target is empty,
// MOUSEION_NLP_ADDR is used, falling back to localhost:50051.
func NewGRPCAnalyzer(target string, options ...grpc.DialOption) (*GRPCAnalyzer, error) {
	if target == "" {
		target = os.Getenv("MOUSEION_NLP_ADDR")
	}
	if target == "" {
		target = defaultNLPAddress
	}
	if len(options) == 0 {
		options = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	conn, err := grpc.NewClient(target, options...)
	if err != nil {
		return nil, fmt.Errorf("create NLP gRPC client: %w", err)
	}
	return &GRPCAnalyzer{conn: conn, client: mouseionv1.NewAnalyzerServiceClient(conn)}, nil
}

func (a *GRPCAnalyzer) Analyze(ctx context.Context, request AnalyzeRequest) (Result, error) {
	corpus, err := a.client.Analyze(ctx, &mouseionv1.AnalyzeRequest{
		Language:     request.Language,
		DocumentText: request.Document.Text,
		SourceDocument: &mouseionv1.SourceDocument{
			Id:               request.Document.ID,
			SourceIdentifier: request.Document.SourceIdentifier,
			Title:            request.Document.Title,
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("call NLP analyzer: %w", err)
	}
	result, err := FromProto(corpus)
	if err != nil {
		return Result{}, fmt.Errorf("convert NLP analyzer response: %w", err)
	}
	return result, nil
}

func (a *GRPCAnalyzer) GetCapabilities(ctx context.Context) (Capabilities, error) {
	response, err := a.client.GetCapabilities(ctx, &mouseionv1.GetCapabilitiesRequest{})
	if err != nil {
		return Capabilities{}, fmt.Errorf("get NLP capabilities: %w", err)
	}
	result := Capabilities{Languages: make([]LanguageCapability, 0, len(response.GetLanguages()))}
	for _, language := range response.GetLanguages() {
		result.Languages = append(result.Languages, LanguageCapability{
			Language: language.GetLanguage(), DisplayName: language.GetDisplayName(),
			ModelVersion: language.GetModelVersion(), SupportedFeatures: append([]string(nil), language.GetSupportedFeatures()...),
			Ready: language.GetReady(),
		})
	}
	return result, nil
}

// Close releases the analyzer's reusable gRPC connection.
func (a *GRPCAnalyzer) Close() error {
	return a.conn.Close()
}

var _ Analyzer = (*GRPCAnalyzer)(nil)
var _ CapabilityProvider = (*GRPCAnalyzer)(nil)
