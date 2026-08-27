package insight

import "github.com/bilal-arikan/tionharness/internal/textutil"

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
		toks[i] = textutil.Tokenize(f.Title + " " + f.RootCause)
	}
	// repToks tracks the token set of each cluster's representative for comparison.
	var repToks []map[string]struct{}
	for i, f := range findings {
		placed := false
		for c := range clusters {
			if textutil.Jaccard(toks[i], repToks[c]) >= clusterThreshold {
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

// The tokenizer, Jaccard and the stopword set now live in internal/textutil so
// the lessons store can reuse the exact same similarity notion (its LLM-written
// signature slugs suffer the same "same issue, new wording" problem).
