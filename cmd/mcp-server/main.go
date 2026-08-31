// Command mcp-server exposes audited read-only exchange tools over MCP.
package main

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	store "github.com/woyaofei303/exchange-lab/internal/platform/postgres"
	"github.com/woyaofei303/exchange-lab/internal/readtools"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type symbolInput struct {
	Symbol string `json:"symbol" jsonschema:"exact contract symbol; only BTCUSDT-PERP is allowed"`
}
type historyInput struct {
	Symbol string `json:"symbol,omitempty"`
	Limit  int    `json:"limit" jsonschema:"maximum 50 records"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(context.Background()); err != nil {
		logger.Error("MCP server stopped")
		os.Exit(1)
	}
}
func run(ctx context.Context) error {
	userID, err := strconv.ParseInt(os.Getenv("EXCHANGE_LAB_MCP_USER_ID"), 10, 64)
	if err != nil || userID <= 0 {
		return errors.New("MCP principal is required")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://exchange:exchange_dev_only@127.0.0.1:5432/exchange?sslmode=disable"
	}
	pool, err := store.OpenPool(ctx, dsn, store.PoolConfig{MinConns: 0, MaxConns: 2, ConnectTimeout: 5 * time.Second})
	if err != nil {
		return err
	}
	defer pool.Close()
	service, err := readtools.New(userID, store.NewToolSource(pool, 3*time.Second), store.NewToolAuditRepository(pool, 3*time.Second), time.Now)
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "exchange-lab-readonly", Version: "v1.0.0"}, &mcp.ServerOptions{Instructions: "Read-only simulated perpetual-contract evidence. No order, cancel, margin, leverage, balance, transfer, or database mutation tools exist."})
	mcp.AddTool(server, &mcp.Tool{Name: "get_position", Description: "Get the authenticated simulated user's exact BTCUSDT-PERP position."}, func(ctx context.Context, _ *mcp.CallToolRequest, input symbolInput) (*mcp.CallToolResult, any, error) {
		value, err := service.GetPosition(ctx, input.Symbol)
		return nil, value, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_margin_status", Description: "Get server-computed margin and liquidation-distance evidence."}, func(ctx context.Context, _ *mcp.CallToolRequest, input symbolInput) (*mcp.CallToolResult, any, error) {
		value, err := service.GetMarginStatus(ctx, input.Symbol)
		return nil, value, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_mark_price", Description: "Get the independent index and mark price; never substitutes last trade."}, func(ctx context.Context, _ *mcp.CallToolRequest, input symbolInput) (*mcp.CallToolResult, any, error) {
		value, err := service.GetMarkPrice(ctx, input.Symbol)
		return nil, value, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_funding", Description: "Get bounded funding-rate history."}, func(ctx context.Context, _ *mcp.CallToolRequest, input historyInput) (*mcp.CallToolResult, any, error) {
		value, err := service.GetFunding(ctx, input.Symbol, input.Limit)
		return nil, value, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_liquidation_history", Description: "Get at most 50 liquidation records for the fixed authenticated user."}, func(ctx context.Context, _ *mcp.CallToolRequest, input historyInput) (*mcp.CallToolResult, any, error) {
		value, err := service.GetLiquidationHistory(ctx, input.Limit)
		return nil, value, err
	})
	return server.Run(ctx, &mcp.StdioTransport{})
}
