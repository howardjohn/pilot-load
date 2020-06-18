FROM golang:1.13-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o pilot-load .

FROM alpine

COPY --from=builder /app/pilot-load /app/pilot-load

ENTRYPOINT ["/app/pilot-load"]
