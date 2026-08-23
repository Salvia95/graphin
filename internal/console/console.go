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
	"os/user"
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
//
// Patterns carry their method, so the mux answers a wrong one with 405 and an
// Allow header of its own accord. That matters more here than tidiness: the
// write routes and the read routes sit on the same path prefix, and a hand-
// rolled method check is exactly where one of them would eventually be let
// through by accident.
func NewMux(root, ui string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/queue", func(w http.ResponseWriter, _ *http.Request) {
		q, err := wiki.BuildQueueReport(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, q)
	})
	// One candidate in full. The list endpoint deliberately does not carry a
	// proposal's body and aliases: a queue of thirty would ship thirty drafts
	// to render four lines each, and the form that needs them opens one at a
	// time.
	mux.HandleFunc("GET /api/queue/{canonical}", candidateHandler(root))
	// Which repository this is. The console binds one port and a developer
	// with two of them open has no other way to tell which window is which.
	mux.HandleFunc("GET /api/workspace", func(w http.ResponseWriter, _ *http.Request) {
		abs, err := filepath.Abs(root)
		if err != nil {
			abs = root
		}
		writeJSON(w, workspace{Root: abs, Name: filepath.Base(abs)})
	})
	mux.HandleFunc("GET /api/usage", func(w http.ResponseWriter, _ *http.Request) {
		rep, err := usageReport(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, rep)
	})
	mux.HandleFunc("GET /api/wiki", func(w http.ResponseWriter, _ *http.Request) {
		o, err := wiki.BuildOverview(root, filepath.Join(root, wiki.DefaultSkillDir))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, o)
	})
	// Repin is a write, and it is here rather than left to the command for the
	// reason the whole surface exists: deciding a summary still holds after
	// re-reading a section is a judgement, and judgements are what this console
	// is for. It writes pins.lock and stops — the commit stays the reviewer's.
	mux.HandleFunc("POST /api/wiki/repin", repinHandler(root))
	// Changes arrive from outside this process by construction: every action a
	// card names is performed in an editor. Without this the page would keep
	// showing the problem the reader just fixed.
	mux.HandleFunc("GET /api/events", eventsHandler(root))
	mux.HandleFunc("POST /api/queue/{canonical}/approve", approveHandler(root))
	mux.HandleFunc("POST /api/queue/{canonical}/discard", discardHandler(root))
	switch sub, ok := embeddedUI(); {
	case ui != "":
		// An explicit directory wins over the embedded copy: this is how the
		// interface is developed against a real workspace without rebuilding
		// the binary between edits.
		mux.Handle("GET /", http.FileServer(http.Dir(ui)))
	case ok:
		mux.Handle("GET /", http.FileServerFS(sub))
	default:
		mux.HandleFunc("GET /", placeholder)
	}
	return mux
}

// workspace names the repository being served.
type workspace struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

// candidate is one queued proposal as the approval form needs it: the term's
// own fields, prefilled, plus where the file is.
//
// Term is embedded rather than nested because the form's field names are the
// frontmatter's field names, and putting them one level down would invite a
// client to invent a wrapper name for something that already has one.
type candidate struct {
	*wiki.Term
	File string `json:"file"`
	Seen int    `json:"seen"`
}

func candidateHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := wiki.Load(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		queue, err := st.Queue()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		name := r.PathValue("canonical")
		for _, p := range queue {
			if p.Canonical == name {
				writeJSON(w, candidate{Term: &p.Term, File: p.File, Seen: p.Seen})
				return
			}
		}
		writeError(w, http.StatusNotFound, wiki.ErrNoProposal)
	}
}

// repinRequest scopes a repin. An empty body means every pin — what the group
// control sends — and a (set, node_id) pair means the one entry a person just
// re-read.
type repinRequest struct {
	Set    string `json:"set,omitempty"`
	NodeID string `json:"node_id,omitempty"`
}

func repinHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req repinRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var (
			res wiki.RepinResult
			err error
		)
		switch {
		case req.Set != "" && req.NodeID != "":
			res, err = wiki.RepinEntry(root, req.Set, req.NodeID)
		case req.Set != "" || req.NodeID != "":
			// Half a pair is never a repin-everything request. Guessing which
			// one they meant would either vouch for entries nobody read or
			// silently do nothing.
			writeError(w, http.StatusBadRequest,
				errors.New("repinning one entry needs both set and node_id"))
			return
		default:
			res, err = wiki.RepinAll(root, false)
		}
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, res)
	}
}

// maxBody caps a reviewer's form. A glossary entry is a paragraph; anything
// approaching this is a mistake or an attempt.
const maxBody = 256 << 10

// approved is what a successful approval answers with.
//
// File and Note are not decoration. The whole design rests on the reviewer
// still seeing an ordinary diff, so the response says where the change landed
// and states that it stopped there — a UI that renders this cannot leave
// someone believing the term is published.
type approved struct {
	Term *wiki.Term `json:"term"`
	File string     `json:"file"`
	Note string     `json:"note"`
}

func approveHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var edits wiki.Term
		if err := decodeBody(r, &edits); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		st, err := wiki.Load(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		canonical := r.PathValue("canonical")
		t, err := st.Approve(canonical, &edits, reviewer())
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, approved{
			Term: t,
			File: wiki.GlossaryPath(root, canonical),
			Note: "written to the working tree and not committed — review it with git diff",
		})
	}
}

func discardHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := wiki.Load(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := st.Discard(r.PathValue("canonical")); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// statusFor maps the failures a reviewer can actually cause. Everything else
// is ours and answers 500.
func statusFor(err error) int {
	switch {
	case errors.Is(err, wiki.ErrNoProposal):
		return http.StatusNotFound
	case errors.Is(err, wiki.ErrAlreadyInGlossary), errors.Is(err, wiki.ErrGlossaryFull):
		return http.StatusConflict
	case errors.Is(err, wiki.ErrNotHuman):
		return http.StatusBadRequest
	case errors.Is(err, wiki.ErrNoEntry):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func decodeBody(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// reviewer names who vouched, in the actor convention Trust reads.
//
// The account on this machine is the honest answer: the socket is loopback, so
// whoever reached it is sitting here. It is not an identity claim and does not
// need to be — the durable record of who approved what is the commit the
// reviewer makes next, which is the same place it lived before this endpoint
// existed.
func reviewer() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return "human:" + u.Username
	}
	return "human:local"
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
