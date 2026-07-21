package lexical

import (
	"math"
	"sort"
	"sync"
)

// BM25 parameters (§2.1.1 기본값).
const (
	defaultK1 = 1.2
	defaultB  = 0.75
)

// Hit is one ranked search result.
type Hit struct {
	DocID string
	Score float64
}

// Index is an in-memory BM25 inverted index. Safe for concurrent use; the
// document token lists are retained so deletes and persistence need no
// re-tokenization.
type Index struct {
	mu       sync.RWMutex
	k1, b    float64
	docs     map[string][]string       // docID → tokens
	docLen   map[string]int            // docID → token count
	postings map[string]map[string]int // term → docID → term frequency
	totalLen int
}

func NewIndex() *Index {
	return &Index{
		k1:       defaultK1,
		b:        defaultB,
		docs:     map[string][]string{},
		docLen:   map[string]int{},
		postings: map[string]map[string]int{},
	}
}

// Upsert replaces the document's tokens.
func (ix *Index) Upsert(docID string, tokens []string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.removeLocked(docID)
	cp := make([]string, len(tokens))
	copy(cp, tokens)
	ix.docs[docID] = cp
	ix.docLen[docID] = len(cp)
	ix.totalLen += len(cp)
	for _, t := range cp {
		m := ix.postings[t]
		if m == nil {
			m = map[string]int{}
			ix.postings[t] = m
		}
		m[docID]++
	}
}

// Delete removes the document if present.
func (ix *Index) Delete(docID string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.removeLocked(docID)
}

func (ix *Index) removeLocked(docID string) {
	tokens, ok := ix.docs[docID]
	if !ok {
		return
	}
	for _, t := range tokens {
		m := ix.postings[t]
		if m == nil {
			continue
		}
		if m[docID] > 1 {
			m[docID]--
		} else {
			delete(m, docID)
			if len(m) == 0 {
				delete(ix.postings, t)
			}
		}
	}
	ix.totalLen -= len(tokens)
	delete(ix.docs, docID)
	delete(ix.docLen, docID)
}

// Len returns the number of indexed documents.
func (ix *Index) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.docs)
}

// Snapshot copies docID → tokens for persistence.
func (ix *Index) Snapshot() map[string][]string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make(map[string][]string, len(ix.docs))
	for id, toks := range ix.docs {
		cp := make([]string, len(toks))
		copy(cp, toks)
		out[id] = cp
	}
	return out
}

// Search scores unique query tokens with BM25 and returns up to topK hits.
// Ties break on DocID so results are deterministic.
func (ix *Index) Search(queryTokens []string, topK int) []Hit {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	n := len(ix.docs)
	if n == 0 || topK <= 0 {
		return nil
	}
	avg := float64(ix.totalLen) / float64(n)
	if avg == 0 {
		avg = 1
	}
	seen := map[string]bool{}
	scores := map[string]float64{}
	for _, q := range queryTokens {
		if seen[q] {
			continue
		}
		seen[q] = true
		m := ix.postings[q]
		if len(m) == 0 {
			continue
		}
		df := float64(len(m))
		idf := math.Log(1 + (float64(n)-df+0.5)/(df+0.5))
		for docID, tf := range m {
			dl := float64(ix.docLen[docID])
			tfPart := float64(tf) * (ix.k1 + 1) / (float64(tf) + ix.k1*(1-ix.b+ix.b*dl/avg))
			scores[docID] += idf * tfPart
		}
	}
	hits := make([]Hit, 0, len(scores))
	for id, sc := range scores {
		hits = append(hits, Hit{DocID: id, Score: sc})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].DocID < hits[j].DocID
	})
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}
