package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
)

const pipeName = `\\.\pipe\copilot-daemon`

// StatusResponse is returned by the STATUS command.
type StatusResponse struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	Uptime  string `json:"uptime"`
	Version string `json:"version"`
}

// --- Server (runs inside daemon process) ---

type IPCServer struct {
	listener  net.Listener
	startTime time.Time
	port      int
	cancel    context.CancelFunc
	logger    *log.Logger

	mu      sync.Mutex
	logSubs []net.Conn
}

func newIPCServer(port int, cancel context.CancelFunc) (*IPCServer, error) {
	l, err := winio.ListenPipe(pipeName, &winio.PipeConfig{
		InputBufferSize:  4096,
		OutputBufferSize: 65536,
	})
	if err != nil {
		return nil, fmt.Errorf("listen pipe %s: %w", pipeName, err)
	}
	return &IPCServer{
		listener:  l,
		startTime: time.Now(),
		port:      port,
		cancel:    cancel,
	}, nil
}

func (s *IPCServer) Serve(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go s.handle(ctx, conn)
	}
}

func (s *IPCServer) handle(ctx context.Context, conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		conn.Close()
		return
	}
	cmd := strings.TrimSpace(scanner.Text())

	switch cmd {
	case "STATUS":
		resp := StatusResponse{
			Running: isPortInUse(s.port),
			Port:    s.port,
			Uptime:  time.Since(s.startTime).Round(time.Second).String(),
			Version: version,
		}
		json.NewEncoder(conn).Encode(resp)
		conn.Close()

	case "STOP":
		fmt.Fprintln(conn, "stopping")
		conn.Close()
		s.cancel()

	case "LOGS":
		// Add this connection to the log broadcast list
		s.mu.Lock()
		s.logSubs = append(s.logSubs, conn)
		s.mu.Unlock()

		// Block until client disconnects or daemon shuts down.
		// io.Copy reads from conn — when the client closes, Read returns EOF.
		done := make(chan struct{})
		go func() {
			io.Copy(io.Discard, conn)
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
		}

		s.mu.Lock()
		for i, c := range s.logSubs {
			if c == conn {
				s.logSubs = append(s.logSubs[:i], s.logSubs[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		conn.Close()

	default:
		fmt.Fprintln(conn, "unknown command")
		conn.Close()
	}
}

// BroadcastLog sends a log chunk to all connected LOGS subscribers.
// Dead connections are pruned automatically.
func (s *IPCServer) BroadcastLog(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	alive := s.logSubs[:0]
	for _, conn := range s.logSubs {
		_, err := conn.Write(p)
		if err == nil {
			alive = append(alive, conn)
		} else {
			conn.Close()
		}
	}
	s.logSubs = alive
}

func (s *IPCServer) Close() {
	s.listener.Close()
	s.mu.Lock()
	for _, c := range s.logSubs {
		c.Close()
	}
	s.mu.Unlock()
}

// --- Client (runs in CLI commands: status, stop, logs) ---

func dialDaemon() (net.Conn, error) {
	timeout := 2 * time.Second
	conn, err := winio.DialPipe(pipeName, &timeout)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to daemon (is it running?): %w", err)
	}
	return conn, nil
}

func queryStatus() error {
	conn, err := dialDaemon()
	if err != nil {
		fmt.Println("Daemon:         not running")
		if isPortInUse(4141) {
			fmt.Println("copilot-api:    listening on :4141 (unmanaged)")
		} else {
			fmt.Println("copilot-api:    not running")
		}
		return nil
	}
	defer conn.Close()

	fmt.Fprintln(conn, "STATUS")

	var resp StatusResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("decode status: %w", err)
	}

	apiStatus := "not reachable"
	if resp.Running {
		apiStatus = fmt.Sprintf("listening on :%d", resp.Port)
	}

	fmt.Printf("Daemon:         running (v%s, uptime %s)\n", resp.Version, resp.Uptime)
	fmt.Printf("copilot-api:    %s\n", apiStatus)
	return nil
}

func stopDaemon() error {
	conn, err := dialDaemon()
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Fprintln(conn, "STOP")
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		fmt.Println("Daemon:", scanner.Text())
	}
	return nil
}

func tailLogs() error {
	conn, err := dialDaemon()
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Fprintln(conn, "LOGS")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	return nil
}
