# Dockerfile for the mouseion Python NLP gRPC service (Stanza).
FROM python:3.11-slim
WORKDIR /src
# Install the package + its runtime deps (grpcio, protobuf, stanza, torch)
COPY nlp/ ./nlp/
COPY proto/ ./proto/
COPY gen/python/ ./gen/python/
# Copy the shared protobuf bindings into the installable package path
RUN python -m pip install --no-cache-dir -e ./nlp && \
    cp -r ./gen/python/mouseion /src/ 2>/dev/null || true
ENV PYTHONPATH=/src:/src/nlp/src:/src/gen/python
EXPOSE 50051
# Run the long-lived gRPC analyzer server
ENTRYPOINT ["python", "-m", "mouseion_nlp.server"]
