package insight

import "strings"

// Non-destructive display clustering. A single root cause often surfaces as many
// findings with differently-worded titles/signatures (observed: ~12 variants of
// "a disabled tool was offered to the model"). Exact/canonical signature dedup
// cannot merge those — the words differ. ClusterFindings groups them at query
// time by lexical similarity so a caller (panel, tool) can collapse a cluster to
// one representative + a count, WITHOUT mutating or losing any stored finding.

// clusterThreshold is the minimum token-set Jaccard similarity for two findings
// to land in the same cluster. Tuned so clear paraphrases group while distinct
// issues stay apart; deliberately conservative (over-splitting is safer than
// over-merging for a human triaging).
const clusterThreshold = 0.5

// Cluster is a group of similar findings. Representative is the highest-priority
// member (clustering runs over a priority-sorted list); Members includes it.
type Cluster struct {
	Representative Finding   `json:"representative"`
	Members        []Finding `json:"members"`
	Size           int       `json:"size"`
}

// ClusterFindings groups findings by lexical similarity of their title+rootCause.
// Input order is honored (pass a priority-sorted list so each cluster's first
// member — its Representative — is the most urgent). Greedy single-pass: each
// finding joins the first existing cluster it is similar enough to, else starts
// a new one.
func ClusterFindings(findings []Finding) []Cluster {
	var clusters []Cluster
	toks := make([]map[string]struct{}, len(findings))
	for i, f := range findings {
		toks[i] = tokenize(f.Title + " " + f.RootCause)
	}
	// repToks tracks the token set of each cluster's representative for comparison.
	var repToks []map[string]struct{}
	for i, f := range findings {
		placed := false
		for c := range clusters {
			if jaccard(toks[i], repToks[c]) >= clusterThreshold {
				clusters[c].Members = append(clusters[c].Members, f)
				clusters[c].Size++
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, Cluster{Representative: f, Members: []Finding{f}, Size: 1})
			repToks = append(repToks, toks[i])
		}
	}
	return clusters
}

// tokenize returns the lowercased, stopword-filtered word set of s (tokens under
// three characters dropped as noise).
func tokenize(s string) map[string]struct{} {
	out := map[string]struct{}{}
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= 3 {
			w := cur.String()
			if !clusterStopwords[w] {
				out[w] = struct{}{}
			}
		}
		cur.Reset()
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// jaccard is |a∩b| / |a∪b|; 0 when both empty.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if _, ok := b[w]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// clusterStopwords are common words that carry no grouping signal — English glue
// plus a few finding-generic terms ("tool", "agent") that appear in almost every
// title and would otherwise inflate similarity.
var clusterStopwords = map[string]bool{
	"the": true, "and": true, "but": true, "for": true, "with": true, "that": true,
	"this": true, "into": true, "from": true, "not": true, "was": true, "were": true,
	"are": true, "its": true, "has": true, "had": true, "when": true, "which": true,
	"instead": true, "than": true, "then": true, "even": true, "though": true,
	"tool": true, "tools": true, "agent": true, "model": true, "call": true,
	"calls": true, "called": true, "using": true, "use": true, "used": true,
}
