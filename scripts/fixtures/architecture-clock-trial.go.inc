func completeArchitectureClockTrial(t *testing.T, f monitoringFixture, service *dm.Service, scope dm.Scope, channel dm.Channel, config dm.ChannelConfig, from, to string, initial dm.Run, elapsed time.Duration) dm.Run {
	t.Helper()
	if initial.ID == "" || initial.Job.ID == "" || initial.Job.Target != initial.ID || initial.Job.Kind != "derive" ||
		initial.Mode != "trial" || initial.ChannelID != channel.Resource.ID || initial.ConfigID != config.ID ||
		initial.Job.TargetRevision != channel.ConfigRevision || initial.PlanHash == "" {
		t.Fatal("clock setup received a different trial, channel, configuration or target revision")
	}
	expectedRange := dm.NanosecondRange{From: from, To: &to}
	if !reflect.DeepEqual(initial.OutputRange, expectedRange) {
		t.Fatal("clock setup accepted a different output range", initial.OutputRange)
	}
	work, stop := context.WithTimeout(t.Context(), 30*time.Second)
	defer stop()
	switch initial.Job.State {
	case "queued":
		if initial.Trial != nil {
			t.Fatal("queued clock trial already has a receipt")
		}
		claimed, err := f.service.RunTask(work, "clock-horizon-trial-setup", scope.Tenant, initial.Job.ID)
		must(t, err)
		if !claimed {
			t.Fatal("accepted clock trial was unavailable to its background worker", initial.Job.ID)
		}
	case "succeeded":
		if initial.Trial == nil {
			t.Fatal("completed foreground clock trial has no receipt")
		}
	default:
		t.Fatal("clock setup could not use the submitted trial", initial.Job.ID, initial.Job.State)
	}
	finished, err := service.Run(work, f.actor, scope, initial.ID)
	must(t, err)
	if finished.ID != initial.ID || finished.Job.ID != initial.Job.ID || finished.Job.Target != initial.ID ||
		finished.Job.TargetRevision != initial.Job.TargetRevision || finished.ChannelID != initial.ChannelID ||
		finished.ConfigID != initial.ConfigID || finished.PlanHash != initial.PlanHash || finished.Mode != "trial" ||
		!reflect.DeepEqual(finished.OutputRange, expectedRange) {
		t.Fatal("clock trial readback changed the original job, revisions or range")
	}
	if finished.Job.State != "succeeded" || finished.Trial == nil || finished.Trial.ReceiptID == "" ||
		finished.Trial.PlanHash != initial.PlanHash || !reflect.DeepEqual(finished.Trial.OutputRange, expectedRange) {
		t.Fatal("clock trial readback has no complete receipt for its original plan and range", finished.Job.State)
	}
	var matched bool
	err = f.env.Pool.QueryRow(work, `SELECT EXISTS (
		SELECT FROM derived_runs WHERE id=$1 AND job_id=$2 AND tenant_id=$3
		 AND COALESCE(project_id::text,'')=$4 AND actor_id=$5 AND channel_id=$6 AND config_id=$7
		 AND config_revision::text=$8 AND control_revision::text=$9 AND mandate_revision::text=$10
		 AND plan_hash=$11 AND lower(output_range)::text=$12 AND upper(output_range)::text=$13
	)`, initial.ID, initial.Job.ID, scope.Tenant, scope.Project, f.actor.ID, initial.ChannelID, initial.ConfigID,
		channel.ConfigRevision, channel.ControlRevision, channel.MandateRevision, initial.PlanHash, from, to).Scan(&matched)
	must(t, err)
	if !matched {
		t.Fatal("stored clock trial changed its scope, execution revisions or accepted range")
	}
	if initial.Trial != nil && finished.Trial.ReceiptID != initial.Trial.ReceiptID {
		t.Fatal("foreground and readback returned different trial receipts")
	}
	t.Logf("clock trial setup initial_state=%s submission_ms=%d run=%s job=%s target_revision=%s receipt=%s",
		initial.Job.State, elapsed.Milliseconds(), initial.ID, initial.Job.ID, initial.Job.TargetRevision, finished.Trial.ReceiptID)
	return finished
}
