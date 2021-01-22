FROM golang:1.15.7-alpine AS base
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

FROM base AS build
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/pilot-load .

FROM alpine

COPY --from=build /out/pilot-load /app/pilot-load

ENTRYPOINT ["/app/pilot-load"]
