package main

import (
	"flag"
	"log"
	"os"

	"github.com/tmonteith/SportsAgentic/pkg/mcp"
)

func main() {
	mode := flag.String("mode", "sse", "Transport mode: 'sse' (HTTP / SSE server) or 'stdio' (MCP stdio)")
	port := flag.String("port", "8080", "Port to listen on when running in SSE/HTTP mode")
	flag.Parse()

	apiKey := os.Getenv("CFBD_API_KEY")
	if apiKey == "" {
		log.Println("Warning: CFBD_API_KEY environment variable not set.")
	}

	server := mcp.NewServer(apiKey)

	switch *mode {
	case "stdio":
		log.Println("Starting College Football MCP Server over stdio...")
		if err := server.ServeStdio(); err != nil {
			log.Fatalf("Stdio server error: %v", err)
		}
	case "sse":
		addr := "localhost:" + *port
		log.Printf("Starting College Football Agent SSE Server on http://%s\n", addr)
		log.Printf(" - SSE Endpoint:     http://%s/sse\n", addr)
		log.Printf(" - Message Endpoint: http://%s/message\n", addr)
		if err := server.ServeSSE(addr); err != nil {
			log.Fatalf("SSE server error: %v", err)
		}
	default:
		log.Fatalf("Unknown mode '%s'. Supported modes: 'sse', 'stdio'", *mode)
	}
}
