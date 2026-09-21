# Dockerfile for the mouseion Python NLP gRPC service (Stanza).
FROM python:3.11-slim
WORKDIR /src
ENV STANZA_RESOURCES_DIR=/opt/stanza_resources
ENV HF_HOME=/opt/huggingface
# Install the package + its runtime deps (grpcio, protobuf, stanza, torch)
COPY nlp/ ./nlp/
COPY internal/canonicalization/german_post1996.json ./internal/canonicalization/german_post1996.json
COPY proto/ ./proto/
COPY gen/python/ ./gen/python/
# Copy the shared protobuf bindings so `from mouseion.v1 import ...` resolves.
RUN python -m pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu && \
    python -m pip install --no-cache-dir -e ./nlp && \
    cp -r ./gen/python/mouseion /src/
ENV PYTHONPATH=/src:/src/nlp/src:/src/gen/python
EXPOSE 50051
# Run the long-lived gRPC analyzer server
ENTRYPOINT ["python", "-m", "mouseion_nlp.server"]
