// Command treepin captures and verifies a scoped working-tree pin for a
// validator (see internal/treepin).
//
//	treepin capture <path>...   print a pin token for the card's files
//	treepin verify '<token>'    FRESH (exit 0) or STALE (exit 3)
//
// Run it from inside the repository. Errors exit 1.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/treepin"
)

const exitStale = 3

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	switch os.Args[1] {
	case "capture":
		p, err := treepin.Capture(".", os.Args[2:])
		if err != nil {
			fail(err)
		}
		fmt.Println(p.String())
	case "verify":
		p, err := treepin.Parse(strings.Join(os.Args[2:], " "))
		if err != nil {
			fail(err)
		}
		r, err := treepin.Verify(".", p)
		if err != nil {
			fail(err)
		}
		fmt.Println(report(p, r))
		if r.Stale {
			os.Exit(exitStale)
		}
	default:
		usage()
	}
}

func report(p treepin.Pin, r treepin.Result) string {
	var b strings.Builder
	if r.Stale {
		fmt.Fprintf(&b, "STALE: in-scope content changed since the pin (%s)", strings.Join(p.Scope, ", "))
		if len(r.DirtyInScope) > 0 {
			fmt.Fprintf(&b, "; dirty in scope now: %s", strings.Join(r.DirtyInScope, ", "))
		}
	} else {
		fmt.Fprintf(&b, "FRESH: %d in-scope file(s) unchanged since the pin", p.Files)
	}
	var churn []string
	if r.HeadMoved {
		churn = append(churn, fmt.Sprintf("HEAD moved %s -> %s", short(p.Head), short(r.CurrentHead)))
	}
	if r.DirtyOutOfScope > 0 {
		churn = append(churn, fmt.Sprintf("%d dirty file(s) outside scope", r.DirtyOutOfScope))
	}
	if len(churn) > 0 {
		fmt.Fprintf(&b, "\nout-of-scope churn (ignored): %s", strings.Join(churn, ", "))
	}
	return b.String()
}

func short(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: treepin capture <path>... | treepin verify '<token>'")
	os.Exit(1)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
