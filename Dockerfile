# syntax=docker/dockerfile:1

FROM golang:1.22 AS builder
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/blobbus ./cmd/blobbus

FROM scratch
COPY --from=builder /out/blobbus /blobbus
EXPOSE 8080
ENTRYPOINT ["/blobbus"]
