package agent

// trajectoryWorkPending keeps fixture cleanup behind all binder writes.
func (r *Runtime) trajectoryWorkPending() bool { return r.trajWork.pending.Load() > 0 }
