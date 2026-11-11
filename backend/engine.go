package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Engine drives a single long-lived Stockfish process over UCI.
// Requests are serialized; a new request interrupts the running search.
type Engine struct {
	path string

	mu      sync.Mutex // one search at a time
	writeMu sync.Mutex // guards stdin
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lines   chan string
	busy    bool
}

type EngineInfo struct {
	Depth int      `json:"depth"`
	Score int      `json:"score"` // centipawns, from WHITE's point of view
	Mate  int      `json:"mate"`  // moves to mate, from WHITE's point of view (0 = none)
	PV    []string `json:"pv"`
	Best  string   `json:"best"`
	Nodes int64    `json:"nodes"`
	NPS   int64    `json:"nps"`
}

func newEngine(path string) *Engine {
	if _, err := exec.LookPath(path); err != nil {
		for _, p := range []string{"/usr/games/stockfish", "/usr/bin/stockfish", "/usr/local/bin/stockfish"} {
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
	}
	return &Engine{path: path}
}

func (e *Engine) write(s string) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	if e.stdin == nil {
		return errors.New("engine not running")
	}
	_, err := io.WriteString(e.stdin, s+"\n")
	return err
}

func (e *Engine) waitFor(prefix string, timeout time.Duration) error {
	t := time.After(timeout)
	for {
		select {
		case l, ok := <-e.lines:
			if !ok {
				return errors.New("engine exited")
			}
			if strings.HasPrefix(l, prefix) {
				return nil
			}
		case <-t:
			return fmt.Errorf("engine timeout waiting for %q", prefix)
		}
	}
}

// startLocked launches Stockfish if it is not running. Caller holds e.mu.
func (e *Engine) startLocked() error {
	if e.cmd != nil {
		return nil
	}
	cmd := exec.Command(e.path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start stockfish (%s): %w", e.path, err)
	}
	lines := make(chan string, 256)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	e.writeMu.Lock()
	e.cmd, e.stdin, e.lines = cmd, stdin, lines
	e.writeMu.Unlock()

	threads := runtime.NumCPU()
	if threads > 8 {
		threads = 8
	}
	e.write("uci")
	if err := e.waitFor("uciok", 5*time.Second); err != nil {
		e.killLocked()
		return err
	}
	e.write(fmt.Sprintf("setoption name Threads value %d", threads))
	e.write("setoption name Hash value 128")
	e.write("isready")
	if err := e.waitFor("readyok", 10*time.Second); err != nil {
		e.killLocked()
		return err
	}
	log.Printf("stockfish started (%s, %d threads)", e.path, threads)
	return nil
}

func (e *Engine) killLocked() {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	if e.cmd != nil {
		e.cmd.Process.Kill()
		e.cmd.Wait()
	}
	e.cmd, e.stdin = nil, nil
}

func (e *Engine) Close() {
	e.write("quit")
	e.mu.Lock()
	defer e.mu.Unlock()
	e.killLocked()
}

// Interrupt asks a running search to finish now (it still returns its best move).
func (e *Engine) Interrupt() { e.write("stop") }

// Analyze searches `fen` for `movetime` and returns the final result.
// onInfo receives throttled progress updates. If fresh is set the hash table is cleared,
// so a re-run does not reuse the previous search's conclusions.
func (e *Engine) Analyze(fen string, movetime time.Duration, fresh bool, onInfo func(EngineInfo)) (EngineInfo, error) {
	e.Interrupt() // cut short any search that is still running
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.startLocked(); err != nil {
		return EngineInfo{}, err
	}
	blackToMove := strings.Fields(fen)[1] == "b"
	if fresh {
		e.write("ucinewgame")
	}
	e.write("position fen " + fen)
	e.write("isready")
	if err := e.waitFor("readyok", 10*time.Second); err != nil {
		e.killLocked()
		return EngineInfo{}, err
	}
	e.write(fmt.Sprintf("go movetime %d", movetime.Milliseconds()))

	var cur EngineInfo
	lastSent := time.Time{}
	deadline := time.After(movetime + 10*time.Second)
	for {
		select {
		case l, ok := <-e.lines:
			if !ok {
				e.killLocked()
				return cur, errors.New("engine crashed")
			}
			if strings.HasPrefix(l, "info ") {
				if inf, ok := parseInfo(l, blackToMove); ok {
					cur = inf
					if onInfo != nil && time.Since(lastSent) > 250*time.Millisecond {
						lastSent = time.Now()
						onInfo(cur)
					}
				}
			} else if strings.HasPrefix(l, "bestmove") {
				f := strings.Fields(l)
				if len(f) > 1 && f[1] != "(none)" {
					cur.Best = f[1]
					if len(cur.PV) == 0 || cur.PV[0] != cur.Best {
						cur.PV = []string{cur.Best}
					}
				}
				return cur, nil
			}
		case <-deadline:
			e.killLocked()
			return cur, errors.New("engine timeout")
		}
	}
}

func parseInfo(line string, blackToMove bool) (EngineInfo, bool) {
	f := strings.Fields(line)
	var inf EngineInfo
	hasPV := false
	for i := 1; i < len(f); i++ {
		switch f[i] {
		case "depth":
			if i+1 < len(f) {
				inf.Depth, _ = strconv.Atoi(f[i+1])
			}
		case "multipv":
			if i+1 < len(f) && f[i+1] != "1" {
				return inf, false
			}
		case "nodes":
			if i+1 < len(f) {
				inf.Nodes, _ = strconv.ParseInt(f[i+1], 10, 64)
			}
		case "nps":
			if i+1 < len(f) {
				inf.NPS, _ = strconv.ParseInt(f[i+1], 10, 64)
			}
		case "score":
			if i+2 < len(f) {
				v, _ := strconv.Atoi(f[i+2])
				if blackToMove {
					v = -v
				}
				if f[i+1] == "mate" {
					inf.Mate = v
				} else {
					inf.Score = v
				}
			}
		case "pv":
			inf.PV = append([]string{}, f[i+1:]...)
			hasPV = len(inf.PV) > 0
			i = len(f)
		}
	}
	if !hasPV {
		return inf, false
	}
	inf.Best = inf.PV[0]
	if len(inf.PV) > 8 {
		inf.PV = inf.PV[:8]
	}
	return inf, true
}
