package proc

// JobLimits bounds a contained process tree (see Job). Zero fields mean "no
// limit" for that resource.
type JobLimits struct {
	// ActiveProcesses caps how many processes of the tree may be alive at once.
	// Process creation beyond it fails inside the tree; it is a fork-bomb brake.
	ActiveProcesses uint32
}
