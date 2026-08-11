FROM golang:1.26.5-alpine3.23 AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/remi-api ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /bin/remi-api /app/remi-api
EXPOSE 8088
USER 65532:65532
ENTRYPOINT ["/app/remi-api"]
