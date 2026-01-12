package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	serverPort = "8080"
	serverURL  = "http://localhost:" + serverPort
	wssURL     = "ws://localhost:" + serverPort + "/wss"
)

type AgentInfo struct {
	AgentID        string `json:"agent_id"`
	Hostname       string `json:"hostname"`
	MaxConcurrency int    `json:"max_concurrency"`
	Paused         bool   `json:"paused"`
	RunningJobs    int    `json:"running_jobs"`
	LastHeartbeat  string `json:"last_heartbeat"`
	ConnectedAt    string `json:"connected_at"`
}

func main() {
	fmt.Println("=== E2E Test (M0): WSS + Register/Heartbeat + Agent Online List ===")
	testStart := time.Now()

	projectRoot := ".."

	// Build binaries first
	fmt.Println("\n[0/4] Building binaries...")
	serverBin, agentBin, err := buildBinaries(projectRoot)
	if err != nil {
		fmt.Printf("Failed to build binaries: %v\n", err)
		os.Exit(1)
	}
	// Cleanup binaries on exit (even if test fails)
	defer func() {
		fmt.Println("\nCleaning up binaries...")
		cleanupBinaries(serverBin, agentBin)
	}()
	fmt.Println("Binaries built successfully")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: Start cloud server
	fmt.Println("\n[1/4] Starting cloud server...")
	serverCmd := exec.CommandContext(ctx, serverBin, "-addr", ":"+serverPort, "-dev")
	serverCmd.Stdout = os.Stdout
	serverCmd.Stderr = os.Stderr
	if err := serverCmd.Start(); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
	defer terminateProcess(serverCmd, "server")

	// Wait for server to be ready
	fmt.Println("Waiting for server to be ready...")
	if !waitForServer(15 * time.Second) {
		fmt.Println("Server failed to become ready (/health != 200)")
		os.Exit(1)
	}
	fmt.Println("Server is ready")

	// Step 2: Start agent (use unique agent ID to avoid stale data / collisions)
	fmt.Println("\n[2/4] Starting agent...")
	agentID := fmt.Sprintf("test-agent-%d", time.Now().UnixNano())

	agentCmd := exec.CommandContext(ctx, agentBin,
		"-server", wssURL,
		"-agent-id", agentID,
		"-agent-token", "dev-token",
		"-max-concurrency", "1",
	)
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	if err := agentCmd.Start(); err != nil {
		fmt.Printf("Failed to start agent: %v\n", err)
		os.Exit(1)
	}
	defer terminateProcess(agentCmd, "agent")

	// Step 3: Wait for agent to appear in online list (no fixed sleep)
	fmt.Println("\n[3/4] Waiting for agent to register and become ONLINE...")
	agent1, err := waitForAgentOnline(agentID, 30*time.Second)
	if err != nil {
		fmt.Printf("✗ Agent did not become ONLINE: %v\n", err)
		dumpAgentsForDebug()
		os.Exit(1)
	}

	// Basic assertions to ensure we didn't bypass register/fields
	if agent1.AgentID != agentID {
		fmt.Printf("✗ Unexpected agent_id: got=%s want=%s\n", agent1.AgentID, agentID)
		os.Exit(1)
	}
	if agent1.MaxConcurrency != 1 {
		fmt.Printf("✗ MaxConcurrency mismatch: got=%d want=1 (did Register/config propagate?)\n", agent1.MaxConcurrency)
		os.Exit(1)
	}
	if strings.TrimSpace(agent1.Hostname) == "" {
		fmt.Printf("✗ Hostname is empty (did agent send Register properly?)\n")
		os.Exit(1)
	}

	// If ConnectedAt is parseable, make sure it's "fresh" (avoid stale registry)
	if tConn, ok := parseTimeLoose(agent1.ConnectedAt); ok {
		// allow small skew; connected should not be long before test start
		if tConn.Before(testStart.Add(-30 * time.Second)) {
			fmt.Printf("✗ ConnectedAt looks stale: %s (parsed=%s) testStart=%s\n",
				agent1.ConnectedAt, tConn.Format(time.RFC3339Nano), testStart.Format(time.RFC3339Nano))
			os.Exit(1)
		}
	}

	fmt.Printf("✓ Agent ONLINE: %s (hostname=%s, max_concurrency=%d, last_heartbeat=%s)\n",
		agent1.AgentID, agent1.Hostname, agent1.MaxConcurrency, agent1.LastHeartbeat)

	// Step 4: Verify heartbeat is updating (REAL check, not just "still in list")
	fmt.Println("\n[4/4] Verifying heartbeat updates...")
	lastHB1Raw := strings.TrimSpace(agent1.LastHeartbeat)
	lastHB1Time, lastHB1Parsed := parseTimeLoose(lastHB1Raw)

	agent2, err := waitForHeartbeatAdvance(agentID, lastHB1Raw, lastHB1Time, lastHB1Parsed, 45*time.Second)
	if err != nil {
		fmt.Printf("✗ Heartbeat did not advance: %v\n", err)
		dumpAgentsForDebug()
		os.Exit(1)
	}

	fmt.Printf("✓ Heartbeat advanced: before=%s after=%s\n", agent1.LastHeartbeat, agent2.LastHeartbeat)

	// Optional: If HB is parseable, ensure it is recent (ONLINE meaning is meaningful)
	if tHB, ok := parseTimeLoose(agent2.LastHeartbeat); ok {
		age := time.Since(tHB)
		if age > 30*time.Second {
			fmt.Printf("✗ Heartbeat is too old (%s). ONLINE list may not be filtered by liveness.\n", age)
			os.Exit(1)
		}
	}

	fmt.Println("\n=== E2E Test (M0) PASSED ===")
}

func waitForServer(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(serverURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func getOnlineAgents() ([]AgentInfo, error) {
	resp, err := http.Get(serverURL + "/api/agents/online")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var agents []AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func waitForAgentOnline(agentID string, timeout time.Duration) (AgentInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		agents, err := getOnlineAgents()
		if err == nil {
			for _, a := range agents {
				if a.AgentID == agentID {
					// Must have at least a heartbeat field populated or connected_at populated to count as ONLINE
					if strings.TrimSpace(a.ConnectedAt) != "" || strings.TrimSpace(a.LastHeartbeat) != "" {
						return a, nil
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return AgentInfo{}, fmt.Errorf("timeout after %s waiting for agent %s in /api/agents/online", timeout, agentID)
}

// waitForHeartbeatAdvance waits until LastHeartbeat advances.
// If the heartbeat timestamp can be parsed, we require time to move forward.
// Otherwise we fall back to raw string inequality check.
func waitForHeartbeatAdvance(agentID, lastRaw string, lastTime time.Time, lastParsed bool, timeout time.Duration) (AgentInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		agents, err := getOnlineAgents()
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, a := range agents {
			if a.AgentID != agentID {
				continue
			}
			curRaw := strings.TrimSpace(a.LastHeartbeat)
			if curRaw == "" {
				continue
			}

			if lastParsed {
				if t, ok := parseTimeLoose(curRaw); ok && t.After(lastTime) {
					return a, nil
				}
			} else {
				// fallback: any change indicates new heartbeat
				if curRaw != strings.TrimSpace(lastRaw) {
					return a, nil
				}
			}
		}
		time.Sleep(800 * time.Millisecond)
	}
	return AgentInfo{}, fmt.Errorf("timeout after %s waiting for heartbeat to advance for agent %s", timeout, agentID)
}

// parseTimeLoose tries RFC3339/RFC3339Nano, unix seconds, unix milliseconds.
func parseTimeLoose(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}

	// RFC3339Nano then RFC3339
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}

	// Numeric unix seconds/milliseconds
	if allDigits(s) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, false
		}
		// heuristic: >= 1e12 treat as ms, else seconds
		if n >= 1_000_000_000_000 {
			return time.UnixMilli(n), true
		}
		return time.Unix(n, 0), true
	}

	return time.Time{}, false
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func dumpAgentsForDebug() {
	agents, err := getOnlineAgents()
	if err != nil {
		fmt.Printf("Could not dump agents: %v\n", err)
		return
	}
	fmt.Printf("Online agents dump (%d):\n", len(agents))
	for _, a := range agents {
		fmt.Printf("- id=%s host=%s max=%d paused=%v running=%d connected_at=%s last_hb=%s\n",
			a.AgentID, a.Hostname, a.MaxConcurrency, a.Paused, a.RunningJobs, a.ConnectedAt, a.LastHeartbeat)
	}
}

func terminateProcess(cmd *exec.Cmd, name string) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	fmt.Printf("\nTerminating %s (PID: %d)...\n", name, pid)

	// Kill the process
	if err := cmd.Process.Kill(); err != nil {
		fmt.Printf("Warning: failed to kill %s (PID: %d): %v\n", name, pid, err)
		return
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		_, err := cmd.Process.Wait()
		done <- err
	}()

	select {
	case <-done:
		fmt.Printf("%s (PID: %d) terminated successfully\n", name, pid)
	case <-time.After(5 * time.Second):
		fmt.Printf("Warning: %s (PID: %d) did not exit within 5 seconds, but kill signal was sent\n", name, pid)
	}
}

func buildBinaries(projectRoot string) (serverBin, agentBin string, err error) {
	// Use temp directory for binaries
	tmpDir := os.TempDir()

	// Determine binary extension for Windows
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	serverBin = filepath.Join(tmpDir, fmt.Sprintf("e2e-server-%d%s", time.Now().UnixNano(), ext))
	agentBin = filepath.Join(tmpDir, fmt.Sprintf("e2e-agent-%d%s", time.Now().UnixNano(), ext))

	// Build server
	fmt.Printf("Building server to %s...\n", serverBin)
	buildServer := exec.Command("go", "build", "-o", serverBin, "./cmd/server")
	buildServer.Dir = filepath.Join(projectRoot, "cloud")
	buildServer.Stdout = os.Stdout
	buildServer.Stderr = os.Stderr
	if err := buildServer.Run(); err != nil {
		return "", "", fmt.Errorf("failed to build server: %w", err)
	}

	// Build agent
	fmt.Printf("Building agent to %s...\n", agentBin)
	buildAgent := exec.Command("go", "build", "-o", agentBin, "./cmd/agent")
	buildAgent.Dir = filepath.Join(projectRoot, "agent")
	buildAgent.Stdout = os.Stdout
	buildAgent.Stderr = os.Stderr
	if err := buildAgent.Run(); err != nil {
		return "", "", fmt.Errorf("failed to build agent: %w", err)
	}

	return serverBin, agentBin, nil
}

func cleanupBinaries(serverBin, agentBin string) {
	// Remove binaries after test
	if serverBin != "" {
		os.Remove(serverBin)
	}
	if agentBin != "" {
		os.Remove(agentBin)
	}
}
