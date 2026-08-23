package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/waseem-polus/aycorn/server/internal/mcptools"
)

func TestToolsetRegister_matchesSharedCatalog(t *testing.T) {
	// Given
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "test"}, nil)
	(&toolset{}).register(server)
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP server: %v", err)
	}
	t.Cleanup(func() {
		if err := serverSession.Close(); err != nil {
			t.Errorf("close MCP server session: %v", err)
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() {
		if err := clientSession.Close(); err != nil {
			t.Errorf("close MCP client session: %v", err)
		}
	})

	// When
	result, err := clientSession.ListTools(ctx, nil)

	// Then
	if err != nil {
		t.Fatalf("list registered MCP tools: %v", err)
	}
	want := mcptools.All()
	if len(result.Tools) != len(want) {
		t.Fatalf("registered tool count = %d; want %d", len(result.Tools), len(want))
	}
	registered := make(map[string]string, len(result.Tools))
	for _, tool := range result.Tools {
		registered[tool.Name] = tool.Description
	}
	for _, definition := range want {
		if registered[definition.Name] != definition.Description {
			t.Fatalf("registered tool %q description = %q; want %q", definition.Name, registered[definition.Name], definition.Description)
		}
	}
}
