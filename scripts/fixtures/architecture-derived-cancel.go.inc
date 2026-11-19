type architectureCancellableSQL struct {
	*dm.Service
	execute func(context.Context) error
}

func (x *architectureCancellableSQL) ExecuteTask(c context.Context, _ monitoring.TaskLease) error {
	return x.execute(c)
}

func TestArchitectureCancelActiveDerivedSQL(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	host := monitoring.New(f.e.Service.Access()).WithStorage(new(monitoring.Storage))
	f.s.WithMonitoring(host)
	must(t, f.s.RegisterTasks())
	definition := f.publish(t, f.create(t, "CANCEL_SQL").Definition)
	created, e := f.s.CreateChannel(ctx, actor, f.scope, dm.CreateChannelInput{
		WriteMeta: wm(""), Code: "CANCEL_SQL", Name: "运行中撤销",
		Owner:               dm.Owner{Kind: resources.Point, FacilityID: f.facility, ID: f.point},
		DefinitionVersionID: definition.Version.ID,
		InputBindings:       map[string]dm.InputBinding{"x": {ChannelID: f.direct}},
	}, access.Meta{})
	must(t, e)
	channel := created.Channel
	configs, e := f.s.ChannelConfigs(ctx, actor, f.scope, channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	config := configs.Items[0]
	from, to := "100", "200"
	interval := dm.NanosecondRange{From: from, To: &to}
	trial, e := f.s.ChannelTrial(ctx, actor, f.scope, channel.Resource.ID, dm.TrialInput{
		WriteMeta: wm(channel.ConfigRevision), ConfigID: config.ID, ConfigHash: config.ConfigHash,
		OutputRange: interval,
	}, access.Meta{})
	must(t, e)
	if trial.Job.State == "queued" {
		_, e = host.RunTask(ctx, "cancellation-trial", f.scope.Tenant, trial.Job.ID)
		must(t, e)
		trial, e = f.s.Run(ctx, actor, f.scope, trial.ID)
		must(t, e)
	}
	if trial.Trial == nil || !trial.Trial.WaitingForInput {
		t.Fatal("missing empty-input trial receipt", trial.Job.State)
	}
	preview, e := f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, dm.ControlPreviewInput{
		Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{
			Action: "enable", AllowWaiting: true, AutoStartNS: &from, StopNS: &to,
			TrialReceiptID: trial.Trial.ReceiptID,
		},
	})
	must(t, e)
	enabled, e := f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{
		WriteMeta: wm(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash,
	}, access.Meta{})
	must(t, e)
	channel = enabled.Channel
	choice, e := f.s.PreviewBackfill(ctx, actor, f.scope, channel.Resource.ID,
		dm.BackfillPreviewInput{Revision: channel.ConfigRevision, Range: interval})
	must(t, e)
	result, e := f.s.BackfillChannel(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{
		WriteMeta: wm(channel.ConfigRevision), PreviewID: choice.ID, PreviewHash: choice.Hash,
	}, access.Meta{})
	must(t, e)
	if len(result.Runs) != 1 {
		t.Fatal("expected one accepted backfill", len(result.Runs))
	}
	run := result.Runs[0]
	query := "SELECT pg_sleep(45) /* single_derived_cancellation_" + uuid.NewString() + " */"
	sqlEnded := make(chan bool, 1)
	_, e = host.WithTaskExecutors(map[string]monitoring.TaskExecutor{"derive": &architectureCancellableSQL{
		Service: f.s, execute: func(c context.Context) error {
			_, err := f.e.Pool.Exec(c, query)
			sqlEnded <- err != nil && c.Err() != nil
			return err
		},
	}})
	must(t, e)
	work, stop := context.WithTimeout(t.Context(), 20*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = host.RunTask(work, "active-sql-worker", f.scope.Tenant, run.Job.ID)
	}()
	defer func() { stop(); <-done }()
	var backend int
	observe := func() int {
		t.Helper()
		var pid int
		must(t, f.e.Pool.QueryRow(work, `SELECT COALESCE(max(pid),0) FROM pg_stat_activity
			WHERE datname=current_database() AND query=$1 AND state='active' AND wait_event='PgSleep'`, query).Scan(&pid))
		return pid
	}
	for backend == 0 {
		backend = observe()
		if backend == 0 {
			select {
			case <-done:
				t.Fatal("worker finished before the query became active")
			case <-time.After(15 * time.Millisecond):
			}
		}
	}
	running, e := f.s.Run(work, actor, f.scope, run.ID)
	must(t, e)
	if running.Job.ID != run.Job.ID || running.Job.State != "running" {
		t.Fatal("active query is not owned by the submitted job", running.Job)
	}
	command := wm(running.Job.Revision)
	accepted, e := f.s.ControlRun(work, actor, f.scope, run.ID, "cancel", command, access.Meta{})
	must(t, e)
	if accepted.Replayed || accepted.Job.ID != run.Job.ID {
		t.Fatal("cancel receipt refers to a different job or command")
	}
	select {
	case cancelled := <-sqlEnded:
		if !cancelled {
			t.Fatal("the active SQL returned normally")
		}
	case <-work.Done():
		t.Fatal("worker did not cancel the active database query")
	}
	<-done
	for observe() != 0 {
		time.Sleep(15 * time.Millisecond)
	}
	final, e := f.s.Run(work, actor, f.scope, run.ID)
	must(t, e)
	if final.Job.State != "cancelled" || final.Job.ID != run.Job.ID {
		t.Fatal("terminal state or job identity differs", final.Job)
	}
	replayed, e := f.s.ControlRun(work, actor, f.scope, run.ID, "cancel", command, access.Meta{})
	must(t, e)
	var receipts int
	must(t, f.e.Pool.QueryRow(work, `SELECT count(*) FROM derived_operation_receipts
		WHERE tenant_id=$1 AND project_id IS NULL AND actor_id=$2 AND request_id=$3 AND operation='runs.cancel'
		AND result->>'id'=$4 AND result->'job'->>'id'=$5`, f.scope.Tenant, actor.ID, command.RequestID, run.ID, run.Job.ID).Scan(&receipts))
	if !replayed.Replayed || replayed.Job.ID != accepted.Job.ID || receipts != 1 {
		t.Fatal("replay lost or duplicated the durable cancel receipt", replayed.Replayed, receipts)
	}
	t.Logf("ARCH_PG_CANCEL run=%s job=%s request=%s active_pid=%d active_after=false state=%s replayed=%t receipts=%d",
		run.ID, final.Job.ID, command.RequestID, backend, final.Job.State, replayed.Replayed, receipts)
}
