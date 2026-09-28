package tools

import "regexp"

// Go 1.27 includes slice indexes in JSON field paths; older toolchains omit them.
var graphBranchesErrorPath = regexp.MustCompile(`\b(?:Node\.nodes\.branches|Graph\.nodes\.\d+\.branches\.\d+)\b`)
