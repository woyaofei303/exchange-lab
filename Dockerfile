FROM golang:1.26.6-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/market-simulator ./cmd/market-simulator && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/market-worker ./cmd/market-worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/candle-worker ./cmd/candle-worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/event-worker ./cmd/event-worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/price-worker ./cmd/price-worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/risk-worker ./cmd/risk-worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/funding-worker ./cmd/funding-worker && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mcp-server ./cmd/mcp-server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/web-gateway ./cmd/web-gateway

FROM alpine:3.23.3
RUN addgroup -S exchange && adduser -S -G exchange -u 10001 exchange
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/market-simulator /usr/local/bin/market-simulator
COPY --from=build /out/market-worker /usr/local/bin/market-worker
COPY --from=build /out/candle-worker /usr/local/bin/candle-worker
COPY --from=build /out/event-worker /usr/local/bin/event-worker
COPY --from=build /out/price-worker /usr/local/bin/price-worker
COPY --from=build /out/risk-worker /usr/local/bin/risk-worker
COPY --from=build /out/funding-worker /usr/local/bin/funding-worker
COPY --from=build /out/mcp-server /usr/local/bin/mcp-server
COPY --from=build /out/web-gateway /usr/local/bin/web-gateway
COPY scripts/container-entrypoint.sh /usr/local/bin/container-entrypoint
COPY web/dist /srv/web
USER 10001:10001
EXPOSE 3000 8080 8090 8091 8092 8093 8094 8095
ENTRYPOINT ["/usr/local/bin/api"]
