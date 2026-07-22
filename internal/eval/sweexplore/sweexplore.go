// Package sweexplore is the Phase 7c evaluation harness
// (docs/phase7-spec.md §3): it replays a deterministic, LLM-free explorer
// policy over SWE-Explore-Bench task repositories and emits ranked
// (path, start, end) region submissions for the benchmark's official scorer.
// The harness never scores — grading stays with eval.py/ExploreEvaluator so
// the numbers are comparable with published baselines.
package sweexplore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Region is one ranked submission entry (1-based inclusive lines).
type Region struct {
	Path  string `json:"path"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Task is one benchmark instance. Only the fields the explorer needs are
// decoded — ground truth stays opaque (the scorer owns it).
type Task struct {
	InstanceID string
	RepoDir    string // resolved absolute repo snapshot path
	Issue      string
}

// issueKeys are tried in order on the task object and its meta object; the
// dataset stores the SWE-bench problem statement under one of these.
var issueKeys = []string{"problem_statement", "issue", "issue_text"}

// LoadBench reads a SWE-Explore-Bench JSONL file. Relative repo_dir values
// resolve against reposRoot; instances that cannot resolve an existing repo
// directory or an issue text are returned as errors, not tasks.
func LoadBench(benchPath, reposRoot string) ([]Task, []string, error) {
	f, err := os.Open(benchPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var tasks []Task
	var problems []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20) // ground-truth payloads can be large
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			problems = append(problems, fmt.Sprintf("line %d: invalid JSON: %v", line, err))
			continue
		}
		id := jsonString(obj["instance_id"])
		if id == "" {
			problems = append(problems, fmt.Sprintf("line %d: missing instance_id", line))
			continue
		}
		repo := jsonString(obj["repo_dir"])
		if repo == "" {
			repo = jsonString(obj["repo_path"])
		}
		if repo != "" && !filepath.IsAbs(repo) {
			repo = filepath.Join(reposRoot, repo)
		}
		if repo == "" || !isDir(repo) {
			// 폴백: <reposRoot>/<instance_id> 레이아웃
			alt := filepath.Join(reposRoot, id)
			if isDir(alt) {
				repo = alt
			} else {
				problems = append(problems, id+": repo snapshot not found")
				continue
			}
		}
		issue := extractIssue(obj)
		if issue == "" {
			problems = append(problems, id+": no issue text (tried problem_statement/issue/meta)")
			continue
		}
		tasks = append(tasks, Task{InstanceID: id, RepoDir: repo, Issue: issue})
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	return tasks, problems, nil
}

func extractIssue(obj map[string]json.RawMessage) string {
	for _, k := range issueKeys {
		if s := jsonString(obj[k]); s != "" {
			return s
		}
	}
	if metaRaw, ok := obj["meta"]; ok {
		var meta map[string]json.RawMessage
		if json.Unmarshal(metaRaw, &meta) == nil {
			for _, k := range issueKeys {
				if s := jsonString(meta[k]); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func jsonString(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
