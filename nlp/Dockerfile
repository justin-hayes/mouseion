# Dockerfile for the mouseion Python NLP gRPC service (Stanza).
FROM python:3.13-slim
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
# Fixed UID so the model volumes can be chowned to it by compose (see compose.yaml nlp-init).
RUN groupadd --system --gid 10001 mouseion && \
    useradd --system --uid 10001 --gid mouseion --create-home --home-dir /home/mouseion mouseion && \
    mkdir -p /opt/stanza_resources /opt/huggingface && \
    chown 10001:10001 /opt/stanza_resources /opt/huggingface
ENV PYTHONPATH=/src:/src/nlp/src:/src/gen/python
EXPOSE 50051
USER 10001:10001
# Run the long-lived gRPC analyzer server
ENTRYPOINT ["python", "-m", "mouseion_nlp.server"]
