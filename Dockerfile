FROM golang:1.16.5-alpine AS base
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

FROM howardjohn/shell

COPY --from=build /out/pilot-load /usr/bin/pilot-load

ENTRYPOINT ["/usr/bin/pilot-load"]
