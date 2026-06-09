FROM golang:1.22-alpine AS builder

# git: go mod tidy needs it for VCS metadata
# protoc + plugins: generate .pb.go from worker.proto
RUN apk add --no-cache git protoc && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.1 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY . .

# Generate protobuf Go code from proto/worker.proto
RUN protoc \
    --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/worker.proto

RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /roster ./cmd/roster

# ─── Runtime image ────────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /roster /usr/local/bin/roster

EXPOSE 8080 50051

ENTRYPOINT ["roster"]
CMD ["hub", "--dir", "/project", "--ui", ":8080"]
