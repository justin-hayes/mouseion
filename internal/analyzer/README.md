# Language analyzer boundary

`Analyzer` accepts one complete source document per call and returns
project-owned sentences, tokens, provenance, normalization metadata, and source
locations. Core packages use these types; generated Protobuf types are confined
to `ToProto` and `FromProto`, which bridge analysis results to the versioned
normalized-corpus artifact.

Every token supplies the ADR 0005 identity inputs: the result language, a
canonical lemma derived under the declared normalization profile, and a coarse
Universal POS (`UPOS`) tag. Surface text and the backend's raw lemma remain
unchanged. This keeps candidate selection, persistence, enrichment, and export
independent of both Stanza and the transport schema.

## Implementing another backend

A Stanza, spaCy, Trankit, or UDPipe adapter implements the single `Analyze`
method. It translates `AnalyzeRequest` into one batch backend operation, maps
the backend response into `Result`, and performs canonicalization using the
selected normalization profile. Backend clients and model-specific values stay
inside that adapter; callers never make per-token remote calls.

Backend tests should invoke `analyzertest.RunContract` with an implementation
factory. `analyzertest.Fake` is available to core package tests that need a
deterministic analyzer without Python, a model download, a server, or a
database.
