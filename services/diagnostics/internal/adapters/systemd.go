package adapters

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/process"
)

type PropertyReader interface {
	Get(context.Context, string, string, string) (any, error)
	Close() error
}
type dbusReader struct{ conn *dbus.Conn }

func (d *dbusReader) Get(ctx context.Context, unit, iface, property string) (any, error) {
	var path dbus.ObjectPath
	if err := d.conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").CallWithContext(ctx, "org.freedesktop.systemd1.Manager.GetUnit", 0, unit).Store(&path); err != nil {
		return nil, err
	}
	var v dbus.Variant
	if err := d.conn.Object("org.freedesktop.systemd1", path).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, iface, property).Store(&v); err != nil {
		return nil, err
	}
	return v.Value(), nil
}
func (d *dbusReader) Close() error { return d.conn.Close() }
func (r *Runtime) systemd(ctx context.Context, p settings.Probe, b settings.Binding, o model.Observation) model.Observation {
	reader := r.Systemd
	if reader == nil {
		var e error
		reader, e = systemBus()
		if e != nil {
			if settings.Code(e) == "adapter_unsupported" {
				return model.Failure(o, "unsupported", "adapter_unsupported")
			}
			return failure(o, e)
		}
		defer reader.Close()
	}
	o.Evidence.Scope = "host"
	o.Facets.Presence = "unknown"
	o.Facets.Readiness = "unknown"
	values := map[string]any{}
	for _, item := range []struct{ iface, name string }{{"Unit", "LoadState"}, {"Unit", "ActiveState"}, {"Unit", "SubState"}, {"Service", "MainPID"}, {"Service", "ExecMainCode"}, {"Service", "ExecMainStatus"}, {"Service", "Result"}, {"Service", "ExecMainStartTimestamp"}} {
		v, e := reader.Get(ctx, b.UnitName, "org.freedesktop.systemd1."+item.iface, item.name)
		if e != nil {
			var dbErr dbus.Error
			if errors.As(e, &dbErr) {
				switch dbErr.Name {
				case "org.freedesktop.systemd1.NoSuchUnit", "org.freedesktop.systemd1.LoadFailed":
					o.Facets.Presence = "missing"
					return model.Failure(o, "failed", "process_not_found")
				case "org.freedesktop.DBus.Error.AccessDenied":
					return model.Failure(o, "permission_denied", "permission_denied")
				}
			}
			return failure(o, e)
		}
		values[item.name] = v
	}
	load, _ := values["LoadState"].(string)
	active, _ := values["ActiveState"].(string)
	if load == "not-found" {
		o.Facets.Presence = "missing"
		o.Evidence.SystemdState = "not-found"
		return model.Failure(o, "failed", "process_not_found")
	}
	if !settings.OneOf(active, "active", "activating", "deactivating", "inactive", "failed", "reloading") {
		return model.Failure(o, "invalid_report", "response_mismatch")
	}
	o.Evidence.SystemdState = active
	switch active {
	case "active", "reloading":
		o.Facets.Presence = "running"
		o.Facets.Readiness = "ready"
	case "activating":
		o.Facets.Presence = "running"
		o.Facets.Readiness = "starting"
	case "inactive", "failed", "deactivating":
		o.Facets.Presence = "stopped"
		o.Facets.Readiness = "not_ready"
		o = model.Failure(o, "failed", "process_exited")
	}
	if code, ok := values["ExecMainStatus"].(int32); ok && code >= 0 {
		v := int(code)
		o.Evidence.ExitCode = &v
	}
	pid, ok := values["MainPID"].(uint32)
	start, ok2 := values["ExecMainStartTimestamp"].(uint64)
	if ok && ok2 && pid > 0 && pid <= 2147483647 && start > 0 {
		v := int32(pid)
		if b.ExpectedUID != nil {
			proc, e := process.NewProcessWithContext(ctx, v)
			if e != nil {
				return failure(o, e)
			}
			uids, e := proc.UidsWithContext(ctx)
			if e != nil {
				return failure(o, e)
			}
			if len(uids) == 0 || int(uids[0]) != *b.ExpectedUID {
				o.Facets = model.UnknownFacets()
				return model.Failure(o, "invalid_report", "identity_mismatch")
			}
		}
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(b.UnitName+":"+strconv.FormatUint(start, 10))).String()
		rep, _ := r.Manifest.ReplicaFor(p)
		o.Process = &model.ProcessIdentity{ReplicaID: rep.ID, BootID: id, PID: &v, OSStartTimeMS: strconv.FormatInt(time.UnixMicro(int64(start)).UnixMilli(), 10), PIDNamespace: "host", UID: b.ExpectedUID}
	}
	return o
}
