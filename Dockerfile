# 构建阶段
FROM golang:1.21-alpine AS builder

WORKDIR /workspace

RUN apk add --no-cache git ca-certificates

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY pkg/ ./pkg/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /namespace-guardian ./cmd/controller

# 运行阶段
FROM gcr.io/distroless/base-debian12

WORKDIR /
COPY --from=builder /namespace-guardian /namespace-guardian

USER 65532:65532

ENTRYPOINT ["/namespace-guardian"]

