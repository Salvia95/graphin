package usage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const sampleLog = "../../testdata/fixtures/usage/events-sample.jsonl"

func TestRunReportJSONOnSample(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"report", "--log", sampleLog, "--json"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if rep.Events != 12 || rep.Sessions != 3 || rep.SessionsWithGraphin != 2 {
		t.Fatalf("report = %+v", rep)
	}
	g := rep.Groups["all"]
	// s1/p1: same-intent fallback. s1/p2: adoption (g_read + action). s3/p1:
	// late switch then adoption. s2/p1: discovery failure.
	if g.Adoptions != 2 || g.Fallbacks != 1 || g.SameIntentFallbacks != 1 {
		t.Fatalf("headline = %+v", g)
	}
	if g.LateSwitches != 1 || g.DiscoveryFailures != 1 {
		t.Fatalf("headline = %+v", g)
	}
	if g.FunnelSearches != 3 || g.FunnelAdherent != 1 {
		t.Fatalf("funnel = %+v", g)
	}
}

func TestRunReportMarkdownOnSample(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"report", "--log", sampleLog}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	md := out.String()
	for _, want := range []string{
		"# graphin usage report",
		"## Headline",
		"| all |",
		"Fallback pairs",
		"cancelOrder",
		"## Bigram transitions",
		"## Daily",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRunReportSinceFilters(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"report", "--log", sampleLog, "--json", "--since", "2026-08-02"},
		strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Events != 7 || rep.Sessions != 2 { // s1 (08-01) filtered out
		t.Fatalf("report = %+v", rep)
	}
}

func TestRunUsageErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run(nil, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("no verb: exit %d", code)
	}
	if code := Run([]string{"bogus"}, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("unknown verb: exit %d", code)
	}
	if !strings.Contains(errb.String(), "ingest | report") {
		t.Fatalf("stderr = %s", errb.String())
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if c, err := parseSince("72h", now); err != nil || !c.Equal(now.Add(-72*time.Hour)) {
		t.Fatalf("duration: %v %v", c, err)
	}
	if c, err := parseSince("2026-08-01", now); err != nil || c.Day() != 1 {
		t.Fatalf("date: %v %v", c, err)
	}
	if _, err := parseSince("yesterday", now); err == nil {
		t.Fatal("want error")
	}
}
