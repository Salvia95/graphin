package sweexplore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SweepConfig is one point on the §3.3 sweep matrix. Name becomes the
// submission file suffix.
type SweepConfig struct {
	Name    string
	TopK    int
	RRFK    int
	MinConf float32
}

// DefaultSweep is the spec's H2/H3 matrix (lexical/hybrid is a run-level
// toggle, not a per-config axis — model warmup happens once per task).
func DefaultSweep() []SweepConfig {
	var out []SweepConfig
	for _, tk := range []int{5, 10, 20} {
		for _, rk := range []int{20, 60, 100} {
			for _, mc := range []float32{0, 0.5, 0.8} {
				out = append(out, SweepConfig{
					Name: fmt.Sprintf("k%d-rrf%d-mc%02.0f", tk, rk, mc*10),
					TopK: tk, RRFK: rk, MinConf: mc,
				})
			}
		}
	}
	return out
}

// RunOptions bundles one harness invocation.
type RunOptions struct {
	BenchPath   string
	ReposRoot   string
	OutDir      string
	Policy      string // "graphin" | "grep"
	GrepContext int
	MaxTasks    int // 0 = all
	Base        Options
	Configs     []SweepConfig // nil → single config from Base
}

type submissionLine struct {
	InstanceID string   `json:"instance_id"`
	Regions    []Region `json:"regions"`
}

// Run executes the harness: each task repo indexes once, every sweep config
// replays against that index, and per-config submission JSONL files plus a
// summary.md land in OutDir. Errors on individual tasks are recorded, not
// fatal — a 500-task run must survive one broken snapshot.
func Run(ctx context.Context, o RunOptions, progress io.Writer) error {
	tasks, problems, err := LoadBench(o.BenchPath, o.ReposRoot)
	if err != nil {
		return err
	}
	if o.MaxTasks > 0 && len(tasks) > o.MaxTasks {
		tasks = tasks[:o.MaxTasks]
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no usable tasks in %s (%d problems)", o.BenchPath, len(problems))
	}
	configs := o.Configs
	if len(configs) == 0 {
		configs = []SweepConfig{{Name: "default", TopK: o.Base.TopK, RRFK: o.Base.RRFK, MinConf: o.Base.MinConf}}
	}
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return err
	}

	files := map[string]*os.File{}
	for _, c := range configs {
		f, err := os.Create(filepath.Join(o.OutDir, "submission-"+o.Policy+"-"+c.Name+".jsonl"))
		if err != nil {
			return err
		}
		files[c.Name] = f
		defer f.Close()
	}

	var taskErrs, caveats []string
	start := time.Now()
	for i, task := range tasks {
		fmt.Fprintf(progress, "[%d/%d] %s\n", i+1, len(tasks), task.InstanceID)
		if err := runTask(ctx, task, o, configs, files, &caveats); err != nil {
			taskErrs = append(taskErrs, task.InstanceID+": "+err.Error())
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return writeSummary(o, len(tasks), configs, problems, taskErrs, caveats, time.Since(start))
}

func runTask(ctx context.Context, task Task, o RunOptions, configs []SweepConfig, files map[string]*os.File, caveats *[]string) error {
	emit := func(name string, regions []Region) error {
		b, err := json.Marshal(submissionLine{InstanceID: task.InstanceID, Regions: regions})
		if err != nil {
			return err
		}
		_, err = files[name].Write(append(b, '\n'))
		return err
	}

	if o.Policy == "grep" {
		regions, err := GrepRegions(task.RepoDir, task.Issue, o.Base, o.GrepContext)
		if err != nil {
			return err
		}
		for _, c := range configs {
			if err := emit(c.Name, regions); err != nil {
				return err
			}
		}
		return nil
	}

	// graphin policy: 태스크당 인덱싱 1회, 설정별 인메모리 리플레이. 설정은
	// 검색·탐색 파라미터만 다르므로 재부트스트랩이 필요 없고, 반복
	// 부트스트랩이 유발하던 ORT 세션 누적도 사라진다.
	sess, err := Open(ctx, task.RepoDir, o.Base)
	if err != nil {
		for _, c := range configs { // 인덱싱 실패: 빈 제출로 정렬을 맞춘다
			if emitErr := emit(c.Name, nil); emitErr != nil {
				return emitErr
			}
		}
		return err
	}
	defer sess.Close()

	if sess.Stats.EmbedDropped > 0 {
		*caveats = append(*caveats, fmt.Sprintf(
			"%s: %d embeds dropped (vector index incomplete for this task)",
			task.InstanceID, sess.Stats.EmbedDropped))
	}
	for _, c := range configs {
		cfg := o.Base
		cfg.TopK, cfg.RRFK, cfg.MinConf = c.TopK, c.RRFK, c.MinConf
		if err := emit(c.Name, sess.Explore(task.Issue, cfg)); err != nil {
			return err
		}
	}
	return nil
}

func writeSummary(o RunOptions, taskCount int, configs []SweepConfig, problems, taskErrs, caveats []string, elapsed time.Duration) error {
	var md strings.Builder
	md.WriteString("# SWE-Explore harness run\n\n")
	fmt.Fprintf(&md, "- bench: `%s`\n- policy: `%s`\n- tasks: %d\n- configs: %d\n- elapsed: %s\n",
		o.BenchPath, o.Policy, taskCount, len(configs), elapsed.Round(time.Second))
	fmt.Fprintf(&md, "- base: top_k=%d rrf_k=%d min_conf=%.1f queries=%d semantic=%t\n\n",
		o.Base.TopK, o.Base.RRFK, o.Base.MinConf, o.Base.Queries, o.Base.Semantic)
	md.WriteString("| config | submission |\n|---|---|\n")
	names := make([]string, 0, len(configs))
	for _, c := range configs {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(&md, "| %s | submission-%s-%s.jsonl |\n", n, o.Policy, n)
	}
	writeList := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&md, "\n## %s (%d)\n\n", title, len(items))
		for _, it := range items {
			fmt.Fprintf(&md, "- %s\n", it)
		}
	}
	writeList("skipped instances", problems)
	writeList("task errors", taskErrs)
	writeList("coverage caveats", caveats)
	md.WriteString("\n채점: SWE-Explore-Bench의 `eval.py`/`ExploreEvaluator`에 위 JSONL을 투입한다 — 하니스는 제출만 생성하고 점수는 공식 스코어러가 낸다.\n")
	return os.WriteFile(filepath.Join(o.OutDir, "summary.md"), []byte(md.String()), 0o644)
}
