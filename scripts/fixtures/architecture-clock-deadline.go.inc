// Measure the context supplied by the real foreground worker around the
// original test executor, which holds its first execution until the deadline.
type architectureClockDeadline struct {
	*deadlineTrialExecutor
	t *testing.T
}

func (d *architectureClockDeadline) ExecuteTask(ctx context.Context, lease md.TaskLease) error {
	if !d.first {
		return d.deadlineTrialExecutor.ExecuteTask(ctx, lease)
	}
	start := time.Now()
	deadline, ok := ctx.Deadline()
	remaining := deadline.Sub(start)
	err := d.deadlineTrialExecutor.ExecuteTask(ctx, lease)
	elapsed := time.Since(start)
	if !ok || remaining <= 0 || remaining > 3*time.Second || ctx.Err() != context.DeadlineExceeded || elapsed < remaining {
		d.t.Errorf("unexpected foreground deadline remaining=%v elapsed=%v context=%v", remaining, elapsed, ctx.Err())
	}
	d.t.Logf("clock foreground deadline remaining_ns=%d waited_ns=%d", remaining.Nanoseconds(), elapsed.Nanoseconds())
	return err
}
