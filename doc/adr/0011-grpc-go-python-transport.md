# ADR 0011: gRPC as the Go↔Python transport for the NLP service

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

ADR 0001 §3 established that the boundary between the Go core and the Python NLP producer is a **typed, versioned Protobuf schema**, and that the same `.proto` defines *"the RPC messages and the persisted corpus file format."* It deliberately used the word **RPC**, but did not pin the transport. `product.md` Open Question 9 left it open: *"Exact Protobuf schema; HTTP vs. gRPC transport."* Issue #5 (the schema) settled the schema but not the transport.

Issue #13's first pass implemented the Go→Python boundary as a **subprocess bridge** — Go shells out to `python`, passing JSON on stdin and reading a base64 protobuf on stdout. This works, but it diverges from the architecture's RPC framing: it spawns a fresh Python process per analysis job rather than talking to a long-lived NLP service, and it is not a clean network boundary.

The decision to record: **use gRPC as the transport between the Go core and the Python NLP service**, per the author's intent that the boundary be a genuine RPC call.

## Decision

**Adopt gRPC as the transport between the Go core and the Python NLP service.** The Go core calls the Python service over gRPC; the service runs Stanza and returns the `NormalizedCorpus`. The Protobuf schema in `proto/mouseion/v1/` is extended with a gRPC service definition, and generated gRPC stubs (Go + Python) replace the subprocess bridge.

### 1. A long-lived Python NLP service

The Python Stanza producer (issue #7) becomes a **gRPC server** exposing an `Analyze` RPC. It loads the Stanza pipeline once and stays warm, serving analysis requests over gRPC — no per-job process spawn. This matches the "coarse, ingest-time NLP producer" model of ADR 0001 §2 while making the boundary a real service call.

### 2. The service contract

A gRPC service is added to the existing `NormalizedCorpus` schema (same `.proto`), e.g.:

```proto
service AnalyzerService {
  rpc Analyze(AnalyzeRequest) returns (NormalizedCorpus);
}
```

Reusing the existing `NormalizedCorpus` message keeps one schema for both the RPC and the persisted artifact (ADR 0001 §3). Generated Go and Python gRPC stubs are committed.

### 3. gRPC is consistent with the existing stack

- Protobuf is already the schema (ADR 0001 §3); gRPC uses Protobuf natively.
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` are already in the toolchain.
- The Python side uses `grpcio` (Stanza already pulls a mature Python ecosystem).
- gRPC gives typed request/response, streaming (future), deadlines/timeouts, and is well-suited to a long-lived service in the home lab.

## The River → gRPC analysis workflow

With gRPC adopted, the end-to-end analysis flow becomes:

1. **Web client** submits an analysis request for an owned source material (issue #25).
2. **Go core** enqueues a **River job** (ADR 0010) — the job args carry the owner, source material ID, language, and content hash. Enqueueing is transactional and idempotent (River unique + content-hash).
3. **River worker** (Go) picks up the job and makes an **async gRPC call** to the Python NLP service (`Analyze`), passing the document text + language.
4. **Python service** runs Stanza, returns a `NormalizedCorpus` (or error).
5. **River worker** persists the result — normalized artifact, shared lemmas, corpus, and processing history — atomically, scoped to the owner (reusing the persistence from issue #13).
6. The **web client** polls the job status/handle and, on completion, retrieves the result.

This is precisely the "Go puts a task on the River queue → async gRPC call to Python" flow the author described. River remains the durable job orchestration layer; gRPC is the transport to the NLP service; ownership and authz are enforced in the worker exactly as in issue #13.

## Alternatives considered

- **Subprocess bridge (issue #13's first pass).** Rejected as the primary transport: spawns a Python process per job, diverges from the RPC framing, and is not a clean service boundary. (May remain as a test-only fixture.)
- **Plain HTTP (REST/JSON) service.** Rejected in favor of gRPC: we already have Protobuf; gRPC gives typed contracts without hand-rolled JSON serialization and is a better fit for a long-lived service. HTTP remains an option for the web-facing API (issue #24), but not for the internal Go↔Python boundary.
- **gRPC vs. keeping the boundary in-process.** Rejected: Stanza has no viable Go binding (ADR 0001 §2), so a separate Python process/service is required.

## Consequences

- The Python producer is packaged and run as a **gRPC server** (`grpcio`); a way to launch it in the home lab is added.
- The Go side adds a **gRPC client** that implements the `Analyzer` interface (issue #6), replacing the subprocess bridge. A `Analyzer` implementation (`GRPCAnalyzer`) calls the Python service.
- `proto/mouseion/v1/normalized_corpus.proto` gains the `AnalyzerService` service definition; generated Go + Python gRPC stubs are committed (`make gen` updated).
- The River worker (issue #13) calls the gRPC analyzer instead of the subprocess.
- A second runtime (the Python gRPC service) must run alongside the Go core in the home lab; it is invoked only for analysis, consistent with ADR 0001 §2 (Python not required by the serving web process).
- Open Question 9's transport is **resolved**; the subprocess bridge in issue #13 is reworked to gRPC.

## Open questions

- Whether the Python service runs as a standalone process/container or is managed alongside the Go app in the home-lab compose stack (likely a sidecar).
- Whether to add streaming (e.g. progress updates) to the `Analyze` RPC later; v1 uses a single request/response.

---

## Related

- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md) — §3 the typed RPC boundary this specifies the transport for.
- [ADR 0010: Adopt River as the background-job queue](0010-river-job-queue.md) — River orchestrates the analysis job that makes the gRPC call.
- [ADR 0002 / 0009] — ownership preserved through job args and worker authz.
- [Product specification](../product.md) — resolves Open Question 9's transport; updates the Decision Register.
