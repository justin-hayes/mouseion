# Dockerfile for the mouseion Go web server.
# Builds the server binary from source; migrations are embedded in the binary.
FROM golang:1.24 AS build
WORKDIR /src
# Cache module deps first
COPY go.mod go.sum ./
RUN go mod download
# Copy source + templates + generated protobuf + embedded migrations
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY proto/ ./proto/
COPY gen/ ./gen/
COPY migrations/ ./migrations/
# Generate templ views at build time (avoids committing drift surprises in the image)
COPY internal/webapp/*.templ ./internal/webapp/
# Pin templ to the version in go.mod (v0.3.977); newer versions require Go >= 1.25.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.977 && \
    export PATH="$PATH:$(go env GOPATH)/bin" && \
    templ generate && \
    CGO_ENABLED=0 go build -o /out/mouseion-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mouseion-server /mouseion-server
EXPOSE 8080
ENTRYPOINT ["/mouseion-server"]
