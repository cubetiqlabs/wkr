package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type logEntry struct {
	RequestID     string   `json:"request_id"`
	StatusCode    int      `json:"status_code"`
	Duration      int64    `json:"duration"`
	Method        string   `json:"method"`
	Path          string   `json:"path"`
	NodeID        string   `json:"node_id"`
	Region        string   `json:"region"`
	ClientIP      string   `json:"client_ip"`
	UserAgent     string   `json:"user_agent"`
	Error         string   `json:"error"`
	Logs          []string `json:"logs"`
	StackTrace    string   `json:"stack_trace"`
	RequestBytes  int64    `json:"request_bytes"`
	ResponseBytes int64    `json:"response_bytes"`
	CreatedAt     time.Time `json:"created_at"`
}

func cmdLogs() {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	limit := fs.Int("limit", 20, "Number of log entries")
	follow := fs.Bool("f", false, "Follow logs in real-time")
	verbose := fs.Bool("v", false, "Verbose output (show IP, user-agent, node)")
	fs.Parse(os.Args[2:])

	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	var name string
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	} else {
		cfg := loadConfig()
		name = cfg.Name
	}

	if *follow {
		followLogs(creds, name, *verbose)
		return
	}

	u := fmt.Sprintf("%s/api/v1/workers/by-name/%s/logs?limit=%d", creds.APIURL, name, *limit)
	resp, err := apiRequest("GET", u, creds.Token, nil)
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal(resp.Error)
	}

	var logs []logEntry
	json.Unmarshal(resp.Data, &logs)

	if len(logs) == 0 {
		fmt.Println("No invocation logs yet.")
		return
	}

	fmt.Printf("Logs for %s:\n\n", name)
	for _, l := range logs {
		printLogEntry(l, *verbose)
	}
}

func followLogs(creds *Credentials, name string, verbose bool) {
	fmt.Printf("Following logs for %s (Ctrl+C to stop)...\n\n", name)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	backoff := time.Second
	for {
		select {
		case <-interrupt:
			fmt.Println("\nStopped.")
			return
		default:
		}

		connStart := time.Now()
		err := wsStream(creds, name, verbose, interrupt)

		// Reset backoff if connection lasted > 30s (was healthy)
		if time.Since(connStart) > 30*time.Second {
			backoff = time.Second
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "  [disconnected: %s, reconnecting in %s...]\n", err, backoff)
		} else {
			fmt.Fprintf(os.Stderr, "  [stream ended, reconnecting in %s...]\n", backoff)
		}

		select {
		case <-interrupt:
			fmt.Println("\nStopped.")
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func wsStream(creds *Credentials, name string, verbose bool, interrupt <-chan os.Signal) error {
	wsURL := buildWSURL(creds.APIURL, name, creds.Token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return err
	}
	defer conn.Close()

	// Client-side ping every 15s to keep connection alive
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var l logEntry
			if json.Unmarshal(msg, &l) == nil {
				printLogEntry(l, verbose)
			}
		}
	}()

	select {
	case <-done:
		return nil
	case <-interrupt:
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		return nil
	}
}

func buildWSURL(apiURL, name, token string) string {
	u, _ := url.Parse(apiURL)
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = fmt.Sprintf("/api/v1/ws/logs/%s", name)
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

func printLogEntry(l logEntry, verbose bool) {
	dur := time.Duration(l.Duration)
	ts := l.CreatedAt.Local().Format("15:04:05")

	statusIcon := "✓"
	if l.Error != "" {
		statusIcon = "✗"
	}

	fmt.Printf("%s %s %d %s %s %s [%s]\n",
		ts, statusIcon, l.StatusCode, l.Method, l.Path,
		dur.Round(time.Millisecond), shortID(l.RequestID),
	)

	if verbose {
		fmt.Printf("  node=%s region=%s ip=%s in=%s out=%s\n",
			l.NodeID, l.Region, l.ClientIP,
			fmtBytes(l.RequestBytes), fmtBytes(l.ResponseBytes),
		)
		if l.UserAgent != "" {
			fmt.Printf("  ua=%s\n", truncate(l.UserAgent, 80))
		}
	}

	if l.Error != "" {
		fmt.Printf("  error: %s\n", l.Error)
	}

	for _, line := range l.Logs {
		fmt.Printf("  log: %s\n", line)
	}

	if l.StackTrace != "" {
		fmt.Println("  --- stack trace ---")
		for _, line := range strings.Split(l.StackTrace, "\n") {
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
		fmt.Println("  ---")
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func fmtBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	}
	return fmt.Sprintf("%.1fKB", float64(b)/1024)
}
