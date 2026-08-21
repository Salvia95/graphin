// Package console serves the read-only views the CLI already produces, over
// HTTP on loopback.
//
// # It does not open the index
//
// This is the constraint that shapes everything here. graph.Open truncates the
// reverse delta log the moment it runs (internal/graph/deltalog.go), so a
// second process must not open a workspace the MCP server is holding — the
// same reason diagnostics became an MCP tool rather than a subcommand
// (internal/mcp/tools/diagnose.go). The console is a separate process, so it
// reads files and nothing else: proposals under docs/wiki, the friction log,
// the usage log. TestConsoleDoesNotReachTheIndex keeps it that way.
//
// # Handlers are shells
//
// Every endpoint calls the same function its command calls and marshals the
// result unchanged. Two implementations of one question drift until they
// disagree in front of a user, so there is only ever one (docs/console-spec.md §5).
package console

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/Salvia95/graphin/internal/usage"
	"github.com/Salvia95/graphin/internal/wiki"
)

// DefaultAddr is loopback on a port unlikely to collide with a dev server.
const DefaultAddr = "127.0.0.1:7673"

const usageLine = `usage: graphin console [--root <dir>] [--addr <host:port>] [--ui <dir>]

Serves the knowledge queue and adoption report at a local address. Read-only,
and it never opens the index — it can run while the MCP server holds the
workspace.`

// Run executes `graphin console` and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("console", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "workspace root")
	addr := fs.String("addr", DefaultAddr, "loopback address to serve on")
	ui := fs.String("ui", "", "directory of static files to serve at / (default: built-in placeholder)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, usageLine)
		return 2
	}

	if err := checkLoopback(*addr); err != nil {
		fmt.Fprintf(stderr, "graphin console: %v\n", err)
		return 2
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           NewMux(*root, *ui),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(stderr, "graphin console: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "graphin console → http://%s  (ctrl-c to stop)\n", ln.Addr())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "graphin console: %v\n", err)
		return 1
	}
	return 0
}

// checkLoopback refuses to bind anywhere a second machine can reach.
//
// The design omits authentication and CORS, and the whole justification for
// that is the bind address. Leaving it to the caller's discretion would mean
// one --addr 0.0.0.0:7673 publishes an unauthenticated view of a private
// repository's vocabulary and adoption metrics. So the assumption is enforced
// where it is made rather than documented and hoped for.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad --addr %q: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("--addr %q binds every interface; the console has no authentication, so it serves loopback only", addr)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("bad --addr %q: %w", addr, err)
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return fmt.Errorf("--addr %q resolves to %s, which is not loopback; the console has no authentication, so it serves loopback only", addr, ip)
		}
	}
	return nil
}

// NewMux builds the routes. Exported so a test can exercise them without
// binding a port.
func NewMux(root, ui string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/queue", readOnly(func(w http.ResponseWriter, _ *http.Request) {
		q, err := wiki.BuildQueueReport(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, q)
	}))
	mux.HandleFunc("/api/usage", readOnly(func(w http.ResponseWriter, r *http.Request) {
		rep, err := usageReport(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, rep)
	}))
	if ui != "" {
		mux.Handle("/", http.FileServer(http.Dir(ui)))
	} else {
		mux.HandleFunc("/", readOnly(placeholder))
	}
	return mux
}

// usageReport is the same pair of calls `graphin usage report` makes. The
// response is exactly usage.Report — identical to `usage report --json` — and
// that identity is the point: a field the CLI grows appears here for free, and
// one it renames cannot quietly mean something else here.
func usageReport(root string) (usage.Report, error) {
	dir := filepath.Join(root, ".graphin", "usage")
	events, problems, err := usage.Load(dir)
	if err != nil {
		return usage.Report{}, err
	}
	return usage.Compute(events, problems, usage.Options{}), nil
}

// readOnly rejects anything but GET.
//
// Writing arrives in its own step with its own decisions — approving a
// candidate moves a file, and the boundary of that write (the working tree,
// never a commit) is settled in docs/console-spec.md §8. Until then a method
// that is not GET is a bug in the caller, and answering it would be worse.
func readOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("%s not allowed; the console is read-only", r.Method))
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(append(b, '\n'))
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// placeholder stands in until the SPA exists. It is deliberately plain: the
// step that adds a frontend toolchain is the first one that is hard to undo,
// so nothing here should look like a reason to skip that decision.
func placeholder(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html>
<meta charset="utf-8">
<title>graphin console</title>
<style>body{font:16px/1.6 system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem}code{background:#8881;padding:.1em .3em;border-radius:3px}</style>
<h1>graphin console</h1>
<p>The interface is not built yet. The data it will read is already served:</p>
<ul>
  <li><a href="/api/queue">/api/queue</a> — same as <code>graphin wiki queue --json</code></li>
  <li><a href="/api/usage">/api/usage</a> — same as <code>graphin usage report --json</code></li>
</ul>
<p>Pass <code>--ui &lt;dir&gt;</code> to serve a build from disk.</p>
`)
}
