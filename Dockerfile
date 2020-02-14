FROM golang:1.13-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o pilot-load .

FROM alpine
COPY --from=builder /app/pilot-load /app/pilot-load
ADD https://storage.googleapis.com/kubernetes-release/release/v1.17.0/bin/linux/amd64/kubectl /usr/bin/kubectl
RUN chmod +x /usr/bin/kubectl
EXPOSE 9901
ENTRYPOINT ["/app/pilot-load"]
