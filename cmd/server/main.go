package main

import (
	"flag"
	"log"
	"os"

	"github.com/tmonteith/SportsAgentic/pkg/mcp"
)

func main() {
	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = "8080"
	}

	mode := flag.String("mode", "sse", "Transport mode: 'sse' (HTTP / SSE server) or 'stdio' (MCP stdio)")
	port := flag.String("port", defaultPort, "Port to listen on when running in SSE/HTTP mode")
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
		bindAddr := ":" + *port
		log.Printf("Starting College Football Agent SSE Server on %s\n", bindAddr)
		log.Printf(" - SSE Endpoint:     http://localhost:%s/sse\n", *port)
		log.Printf(" - Message Endpoint: http://localhost:%s/message\n", *port)
		if err := server.ServeSSE(bindAddr); err != nil {
			log.Fatalf("SSE server error: %v", err)
		}
	default:
		log.Fatalf("Unknown mode '%s'. Supported modes: 'sse', 'stdio'", *mode)
	}
}
