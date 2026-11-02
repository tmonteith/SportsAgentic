package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tmonteith/SportsAgentic/pkg/cfbd"
)

const testTimeout = 5 * time.Second

// newTestServer returns a Server whose CFBD client talks to handler.
func newTestServer(t *testing.T, handler http.HandlerFunc) *Server {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)

	srv := NewServer("")
	srv.cfbdClient = cfbd.NewClientWithBaseURL("", upstream.URL)
	return srv
}

func callScores(t *testing.T, srv *Server, args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()
	var request mcp.CallToolRequest
	request.Params.Arguments = args
	result, err := srv.handleGetScores(context.Background(), request)
	if err != nil {
		t.Fatalf("handleGetScores returned error: %v", err)
	}
	return result
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("got %d content items, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("got content %T, want TextContent", result.Content[0])
	}
	return text.Text
}

func TestGetScoresRejectsQueriesWithoutTeamOrWeek(t *testing.T) {
	tests := map[string]map[string]interface{}{
		"no arguments": {},
		"year only":    {"year": float64(2026)},
		"date only":    {"date": "2026-11-01"},
		"year and date": {
			"year": float64(2026),
			"date": "2026-11-01",
		},
		"week zero":   {"year": float64(2026), "week": float64(0)},
		"blank team":  {"year": float64(2026), "team": "  "},
		"week string": {"year": float64(2026), "week": "10"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				fmt.Fprint(w, "[]")
			})

			result := callScores(t, srv, args)

			if !result.IsError {
				t.Fatalf("got a successful result, want an error: %s", resultText(t, result))
			}
			text := resultText(t, result)
			if !strings.Contains(text, "'team'") || !strings.Contains(text, "'week'") {
				t.Errorf("error doesn't say what to pass: %q", text)
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("made %d CFBD requests, want 0", got)
			}
		})
	}
}

func TestGetScoresPassesFiltersToCFBD(t *testing.T) {
	tests := map[string]struct {
		args map[string]interface{}
		want url.Values
	}{
		"week": {
			args: map[string]interface{}{"year": float64(2026), "week": float64(10)},
			want: url.Values{"year": {"2026"}, "week": {"10"}},
		},
		"team alias": {
			args: map[string]interface{}{"year": float64(2026), "team": "Bama"},
			want: url.Values{"year": {"2026"}, "team": {"Alabama"}},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got url.Values
			srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query()
				fmt.Fprint(w, "[]")
			})

			result := callScores(t, srv, tt.args)

			if result.IsError {
				t.Fatalf("got error: %s", resultText(t, result))
			}
			if got.Encode() != tt.want.Encode() {
				t.Errorf("CFBD query = %q, want %q", got.Encode(), tt.want.Encode())
			}
		})
	}
}

func TestGetScoresFiltersWeekByDate(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":1,"startDate":"2026-10-31T19:30:00.000Z"},{"id":2,"startDate":"2026-11-01T01:00:00.000Z"}]`)
	})

	result := callScores(t, srv, map[string]interface{}{
		"year": float64(2026),
		"week": float64(10),
		"date": "2026-11-01",
	})

	if result.IsError {
		t.Fatalf("got error: %s", resultText(t, result))
	}
	var games []cfbd.Game
	if err := json.Unmarshal([]byte(resultText(t, result)), &games); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if len(games) != 1 || games[0].ID != 2 {
		t.Errorf("got games %+v, want only id 2", games)
	}
}

// sseTestServer serves srv over SSE and reports how each connection's handler ended.
type sseTestServer struct {
	url        string
	sseClosed  chan struct{}
	messageEnd chan interface{}
}

func newSSETestServer(t *testing.T, srv *Server) *sseTestServer {
	t.Helper()
	ts := server.NewTestServer(srv.mcpServer)
	t.Cleanup(ts.Close)

	s := &sseTestServer{
		url:        ts.URL,
		sseClosed:  make(chan struct{}),
		messageEnd: make(chan interface{}, 1),
	}
	inner := ts.Config.Handler
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			defer close(s.sseClosed)
			inner.ServeHTTP(w, r)
			return
		}
		defer func() { s.messageEnd <- recover() }()
		inner.ServeHTTP(w, r)
	})
	return s
}

// openSession opens the SSE stream and returns the session's message endpoint.
func (s *sseTestServer) openSession(t *testing.T, ctx context.Context) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url+"/sse", nil)
	if err != nil {
		t.Fatalf("building SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("opening SSE stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE endpoint event: %v", err)
		}
		if strings.HasPrefix(line, "data:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
}

func toolCallBody(t *testing.T, args map[string]interface{}) string {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "get_cfb_scores",
			"arguments": args,
		},
	})
	if err != nil {
		t.Fatalf("encoding tool call: %v", err)
	}
	return string(body)
}

func waitFor(t *testing.T, what string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(testTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestUnfilteredCallOverSSEReturnsError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("unfiltered call reached CFBD")
	})
	ts := newSSETestServer(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	endpoint := ts.openSession(t, ctx)

	// The agent calls tools without an initialize handshake.
	body := toolCallBody(t, map[string]interface{}{"year": 2026})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building tool call: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("posting tool call: %v", err)
	}
	defer resp.Body.Close()

	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decoding tool response: %v", err)
	}
	if !rpc.Result.IsError {
		t.Fatalf("got a successful result, want an error: %+v", rpc)
	}
	if len(rpc.Result.Content) != 1 || !strings.Contains(rpc.Result.Content[0].Text, "'week'") {
		t.Errorf("error doesn't say what to pass: %+v", rpc.Result.Content)
	}
}

// The agent drops the SSE stream when its timeout fires, while CFBD is still answering.
// mcp-go v0.8.3 then wrote the result to the finished stream's writer and panicked.
func TestClientDisconnectMidCallDoesNotPanic(t *testing.T) {
	cfbdStarted := make(chan struct{})
	cfbdRelease := make(chan struct{})
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		close(cfbdStarted)
		<-cfbdRelease
		fmt.Fprint(w, "[]")
	})
	ts := newSSETestServer(t, srv)

	sseCtx, closeSSE := context.WithCancel(context.Background())
	defer closeSSE()
	endpoint := ts.openSession(t, sseCtx)

	postCtx, closePost := context.WithCancel(context.Background())
	defer closePost()
	body := toolCallBody(t, map[string]interface{}{"team": "Alabama"})
	req, err := http.NewRequestWithContext(postCtx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building tool call: %v", err)
	}
	posted := make(chan struct{})
	go func() {
		defer close(posted)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()

	waitFor(t, "the call to reach CFBD", cfbdStarted)
	closePost()
	closeSSE()
	waitFor(t, "the tool call to be abandoned", posted)
	waitFor(t, "the SSE handler to return", ts.sseClosed)

	close(cfbdRelease)

	select {
	case recovered := <-ts.messageEnd:
		if recovered != nil {
			t.Fatalf("message handler panicked: %v", recovered)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the message handler to finish")
	}
}
