package usage

import (
	"fmt"
	"sort"
	"strings"
)

// classOrder fixes bigram table axes to the spec §4.1 vocabulary.
var classOrder = []Class{
	ClassGSearch, ClassGExplore, ClassGRead, ClassGBoot, ClassGBench,
	ClassSearch, ClassRead, ClassAction, ClassOther,
}

// Markdown renders the report as the sweexplore-style pipe-table summary.
func Markdown(r Report) string {
	var md strings.Builder
	md.WriteString("# graphin usage report\n\n")
	fmt.Fprintf(&md, "- events: %d · sessions: %d (graphin 사용 %d · 미사용 %d)\n",
		r.Events, r.Sessions, r.SessionsWithGraphin, r.Sessions-r.SessionsWithGraphin)
	if r.MedianCallsToFirstNav >= 0 {
		fmt.Fprintf(&md, "- 첫 graphin 내비 콜까지 선행 툴콜 수(중앙값): %d\n", r.MedianCallsToFirstNav)
	}
	if n := len(r.Problems); n > 0 {
		fmt.Fprintf(&md, "- log problems: %d (torn/foreign lines skipped)\n", n)
	}

	md.WriteString("\n## Headline\n\n")
	md.WriteString("| group | windows | adoption | fallback | same-intent | inconclusive | late switch | discovery failure |\n")
	md.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, name := range []string{"all", "main", "subagent"} {
		g, ok := r.Groups[name]
		if !ok || g.Windows == 0 {
			continue
		}
		fmt.Fprintf(&md, "| %s | %d | %s | %d | %d | %d | %s | %s |\n",
			name, g.Windows,
			ratio(g.Adoptions, g.Adoptions+g.Fallbacks),
			g.Fallbacks, g.SameIntentFallbacks, g.Inconclusive,
			ratio(g.LateSwitches, g.WindowsWithGraphin),
			ratio(g.DiscoveryFailures, g.WindowsWithSearch))
	}
	md.WriteString("\nadoption = 채택/(채택+폴백) · late switch 분모 = graphin 사용 윈도우 · discovery failure 분모 = search 있는 윈도우\n")

	// Only when something other than code was touched. A single code row is
	// the headline table again, and a permanent empty db row invites the
	// reading that db adoption is 0% — a workspace with no schema snapshot
	// made no db queries, which is a different thing and should look
	// different.
	if r.Targets[targetDB].Runs > 0 || r.Targets[targetDocs].Runs > 0 {
		md.WriteString("\n## Target (code vs db 스키마 vs 문서)\n\n")
		md.WriteString("| target | runs | adoption | fallback | same-intent | inconclusive |\n")
		md.WriteString("|---|---|---|---|---|---|\n")
		for _, name := range []string{targetCode, targetDB, targetDocs} {
			t := r.Targets[name]
			if t.Runs == 0 {
				continue
			}
			fmt.Fprintf(&md, "| %s | %d | %s | %d | %d | %d |\n",
				name, t.Runs,
				ratio(t.Adoptions, t.Adoptions+t.Fallbacks),
				t.Fallbacks, t.SameIntentFallbacks, t.Inconclusive)
		}
		md.WriteString("\n단위는 윈도우가 아니라 **런**이다. 셋은 분할이 아니라 겹치는 " +
			"모집단이고(한 런이 여러 종류를 다루면 각각에 센다), 노드 id를 하나도 " +
			"참조하지 않은 런은 어느 쪽도 아니다 — 그래서 runs를 같이 낸다. " +
			"docs는 `.md`/`.markdown` 노드다(파일과 섹션 모두).\n")
	}

	if g, ok := r.Groups["all"]; ok && g.FunnelSearches > 0 {
		fmt.Fprintf(&md, "\n## Funnel (search → explore/read ID 핸드오프)\n\n- adherence: %s (%d/%d searches with result_ids)\n",
			ratio(g.FunnelAdherent, g.FunnelSearches), g.FunnelAdherent, g.FunnelSearches)
	}

	if len(r.FallbackPairs) > 0 {
		fmt.Fprintf(&md, "\n## Fallback pairs — 금맥 (최근 %d)\n\n", len(r.FallbackPairs))
		md.WriteString("| ts | graphin query | fallback pattern | same-intent |\n|---|---|---|---|\n")
		for _, p := range r.FallbackPairs {
			fmt.Fprintf(&md, "| %s | %s | %s | %t |\n",
				p.TS, cell(p.Query), cell(p.Pattern), p.SameIntent)
		}
		md.WriteString("\nsame-intent 쌍은 search_hybrid가 놓친 리터럴 재현 케이스다 — 인덱스/랭킹 개선 입력으로 쓴다.\n")
	}

	if len(r.Bigrams) > 0 {
		md.WriteString("\n## Bigram transitions\n\n")
		cols := activeClasses(r.Bigrams)
		md.WriteString("| from \\ to |")
		for _, c := range cols {
			fmt.Fprintf(&md, " %s |", c)
		}
		md.WriteString("\n|---|")
		md.WriteString(strings.Repeat("---|", len(cols)))
		md.WriteString("\n")
		for _, from := range cols {
			row := r.Bigrams[string(from)]
			fmt.Fprintf(&md, "| %s |", from)
			for _, to := range cols {
				if n := row[string(to)]; n > 0 {
					fmt.Fprintf(&md, " %d |", n)
				} else {
					md.WriteString(" |")
				}
			}
			md.WriteString("\n")
		}
	}

	if len(r.Daily) > 1 {
		md.WriteString("\n## Daily\n\n| date | adoptions | fallbacks |\n|---|---|---|\n")
		for _, d := range r.Daily {
			fmt.Fprintf(&md, "| %s | %d | %d |\n", d.Date, d.Adoptions, d.Fallbacks)
		}
	}
	return md.String()
}

func ratio(n, denom int) string {
	if denom == 0 {
		return "–"
	}
	return fmt.Sprintf("%d (%.0f%%)", n, 100*float64(n)/float64(denom))
}

// cell escapes a string for a markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

// activeClasses returns spec-ordered classes that appear in the matrix at all.
func activeClasses(m map[string]map[string]int) []Class {
	used := map[Class]bool{}
	for from, row := range m {
		used[Class(from)] = true
		for to := range row {
			used[Class(to)] = true
		}
	}
	var out []Class
	for _, c := range classOrder {
		if used[c] {
			out = append(out, c)
			delete(used, c)
		}
	}
	// Anything unexpected still renders, sorted, rather than vanishing.
	var extra []string
	for c := range used {
		extra = append(extra, string(c))
	}
	sort.Strings(extra)
	for _, c := range extra {
		out = append(out, Class(c))
	}
	return out
}
