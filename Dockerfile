FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.23.3
RUN addgroup -S exchange && adduser -S -G exchange -u 10001 exchange
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/api /usr/local/bin/api
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
