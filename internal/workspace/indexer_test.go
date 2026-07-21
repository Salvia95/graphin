package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/parse"
)

const javaFixtureDir = "../../testdata/fixtures/java"

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// indexOneFile pushes a single on-disk file through the single-writer path.
func indexOneFile(t *testing.T, w *Workspace, rel string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.File(rel, src)
	if err != nil {
		t.Fatal(err)
	}
	w.indexMu.Lock()
	w.applyFileResult(res)
	w.indexMu.Unlock()
}

const cancelID = "com.example.order.domain.OrderService.cancelPayment(long,String)"

const orderServiceRel = "src/main/java/com/example/order/domain/OrderService.java"

func tempWorkspaceWithOrderService(t *testing.T) *Workspace {
	t.Helper()
	root := t.TempDir()
	copyTree(t, filepath.Join(javaFixtureDir, "src"), filepath.Join(root, "src"))
	w := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	indexOneFile(t, w, orderServiceRel)
	return w
}

func TestReadCodeExactSlice(t *testing.T) {
	w := tempWorkspaceWithOrderService(t)
	cb, err := w.ReadCode(cancelID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cb.Code, "public void cancelPayment(long orderId, String reason)") {
		t.Fatalf("unexpected slice start: %q", cb.Code[:60])
	}
	if cb.Reparsed {
		t.Fatal("fresh index must not report reparsed")
	}
	if cb.StartLine <= 1 || cb.EndLine <= cb.StartLine {
		t.Fatalf("suspicious line range %d-%d", cb.StartLine, cb.EndLine)
	}
}

// TestReadCodeAutoReparseAfterEdit proves §2.1.2: a stale index entry is
// healed inline — fresh offsets, fresh code, reparsed="true", no error.
func TestReadCodeAutoReparseAfterEdit(t *testing.T) {
	w := tempWorkspaceWithOrderService(t)
	abs := filepath.Join(w.Root, filepath.FromSlash(orderServiceRel))
	src, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	// Shift every offset without touching the method body.
	if err := os.WriteFile(abs, append([]byte("// edited\n"), src...), 0o644); err != nil {
		t.Fatal(err)
	}

	cb, err := w.ReadCode(cancelID)
	if err != nil {
		t.Fatal(err)
	}
	if !cb.Reparsed {
		t.Fatal("hash mismatch must trigger inline reparse (reparsed=true)")
	}
	if !strings.HasPrefix(cb.Code, "public void cancelPayment(long orderId, String reason)") {
		t.Fatalf("stale offsets returned wrong text: %q", cb.Code[:60])
	}

	// A second read is served from the healed index without reparsing.
	cb2, err := w.ReadCode(cancelID)
	if err != nil {
		t.Fatal(err)
	}
	if cb2.Reparsed {
		t.Fatal("index should be fresh after inline reparse")
	}
}

func TestReadCodeNodeGoneAfterDeletion(t *testing.T) {
	w := tempWorkspaceWithOrderService(t)
	abs := filepath.Join(w.Root, filepath.FromSlash(orderServiceRel))
	src, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(src),
		"    public void cancelPayment(long orderId, String reason) {\n"+
			"        logger.info(\"cancel payment for \" + orderId + \": \" + reason);\n"+
			"        paymentPort.refund(orderId);\n"+
			"    }\n", "", 1)
	if edited == string(src) {
		t.Fatal("fixture layout changed")
	}
	if err := os.WriteFile(abs, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = w.ReadCode(cancelID)
	if !errors.Is(err, ErrNodeGone) {
		t.Fatalf("expected ErrNodeGone, got %v", err)
	}
}

func TestReadCodeUnknownNode(t *testing.T) {
	w := tempWorkspaceWithOrderService(t)
	_, err := w.ReadCode("com.example.NoSuch.method()")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}

// TestBootstrapIndexesFixtureTree: full bootstrap over a copied fixture repo
// makes symbols searchable and readable.
func TestBootstrapIndexesFixtureTree(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtureDir, root)

	w := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	defer w.Close()
	if _, err := w.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && w.FSM.Phase() < PhaseLexicalReady {
		time.Sleep(20 * time.Millisecond)
	}
	if w.FSM.Phase() < PhaseLexicalReady {
		t.Fatal("initial scan never finished")
	}

	res := w.Router.Search("cancelPayment", 5)
	if len(res) == 0 || res[0].NodeID != cancelID {
		t.Fatalf("search results: %+v", res)
	}
	if _, err := w.ReadCode(cancelID); err != nil {
		t.Fatal(err)
	}
	// Partial file was indexed with its salvageable nodes.
	if got := w.Router.Search("Broken", 5); len(got) == 0 {
		t.Fatal("partial file (Broken.java) should still be indexed")
	}
}

// TestExploreOverRealFixtures: the full pipeline (parse → confidence →
// shards → reverse) reproduces §3.3's example edges.
func TestExploreOverRealFixtures(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtureDir, root)

	w := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	defer w.Close()
	if _, err := w.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && w.FSM.Phase() < PhaseLexicalReady {
		time.Sleep(20 * time.Millisecond)
	}

	// PgPaymentAdapter implements PaymentPort at confidence 1.0.
	page, err := w.Explore("com.example.payment.adapter.PgPaymentAdapter", "uses", "", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	foundImpl := false
	for _, e := range page.Uses {
		if e.NodeID == "com.example.payment.port.PaymentPort" && e.Type == "implements" && e.Confidence == 1.0 {
			foundImpl = true
		}
	}
	if !foundImpl {
		t.Fatalf("implements edge missing: %+v", page.Uses)
	}

	// OrderService.process(ProcessRequest) is used_by OrderController.handle.
	procID := "com.example.order.domain.OrderService.process(ProcessRequest)"
	page, err = w.Explore(procID, "used_by", "", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	foundCaller := false
	for _, e := range page.UsedBy {
		if e.NodeID == "com.example.order.adapter.in.web.OrderController.handle(ProcessRequest)" && e.Type == "call" {
			foundCaller = true
		}
	}
	if !foundCaller {
		t.Fatalf("used_by missing controller (§3.3 예시 재현 실패): %+v", page.UsedBy)
	}

	// Self-call inside OrderService: process(1) → process(2) at 1.0.
	page, err = w.Explore(procID, "uses", "", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	foundSelf := false
	for _, e := range page.Uses {
		if e.NodeID == "com.example.order.domain.OrderService.process(ProcessRequest,boolean)" && e.Confidence == 1.0 {
			foundSelf = true
		}
	}
	if !foundSelf {
		t.Fatalf("same-file overload call must be 1.0: %+v", page.Uses)
	}
}
