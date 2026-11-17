package auditrecord

import "strings"

func registerDomainEvents(add func(string, string, string, string, string, string, []string)) {
	register := func(module, scope, metadata, prefix, actions string) {
		for _, action := range strings.Fields(actions) {
			add(module, "business", "command", scope, metadata, prefix+action, businessFields)
		}
	}
	register("ingestion", "entry", "ingestion", "ingestion.", "relays.create sources.create sources.revision sources.confirm sources.publish sources.pause sources.resume sources.block sources.unblock sources.planning terminals.create terminals.revision associations.create associations.state changes.cancel grants.save unmatched.close recovery.create recovery.cancel recovery.retry messages.read messages.response epochs.confirm")
	register("parsing", "entry", "parsing", "parsing.", "ingestion-catalog.sync capabilities.state catalog.sync methods.state configs.create configs.revision configs.state defaults.set device-configs.create device-configs.revision device.inherit_default device.upgrade_default device.override device.restore_inheritance device.rollback device.clear batch.commands samples.create samples.content samples.confirm samples.read runs.response runs.create runs.compare runs.cancel runs.retry")
	register("devicecatalog", "entry", "devicecatalog", "classification.", "draft publish assign")
	register("engineering", "entry", "engineering", "engineering.", "create save new-draft delete-draft copy publish state delete category-save category-delete")
	register("resources", "tenant", "resources", "resource.", "create update retire restore delete move adopt-type structure-records")
	register("resources", "project", "resources", "project.resource.", "create update retire restore delete move structure-records")
	register("resources", "tenant", "resources", "resource_access.", "policy.updated link.created link.revoked grants.changed")
	for _, scope := range []string{"platform", "tenant"} {
		register("devicecatalog", scope, "devicecatalog", "Device:"+scope+":manufacturer:", "create update state delete")
		register("devicecatalog", scope, "devicecatalog", "Device:"+scope+":product:", "create draft-save draft-create draft-delete publish state delete copy")
	}
	for kind, actions := range map[string]string{"device": "create update state delete", "output": "create update replace end", "installation": "create end relocate correct legacy-resolve", "access": "change"} {
		register("devices", "tenant", "devices", "Device:tenant:"+kind+":", actions)
	}
	for _, scope := range []string{"tenant", "project"} {
		register("sources", scope, "sources", "Device:"+scope+":source:", "bind switch unbind")
	}
	register("spatial", "business", "spatial", "spatial.", "access.save assets.create assets.current assets.restore assets.retire assets.update jobs.cancel jobs.retry uploads.cancel uploads.complete uploads.create versions.create versions.delete versions.edit versions.process versions.publish versions.withdraw views.save")
	register("display", "project", "display", "display.", "instance assign config create grant publish rollback settings terminal.create terminal.update upload upload.cancel upload.complete withdraw package.delete package.retry instance.update instance.delete")
	register("files", "business", "files", "files.", "upload.create upload.complete upload.cancel upload.reconfirm attachment.bound attachment.update attachment.trash attachment.restore attachment.revert task.cancel task.retry task.reconfirm")
	register("derived", "entry", "derived", "derived.", "analysis.artifacts.archive analysis.artifacts.exports analysis.comparisons analysis.configs.archive analysis.configs.copy analysis.configs.create analysis.configs.publish analysis.configs.save analysis.runs analysis.trials analysis.exports.cancel analysis.exports.retry channels.apply-config channels.assignments channels.backfill channels.configs.create metrics.copy metrics.create metrics.delete metrics.publish metrics.save versions.state resource.delete resource.restore resource.retire generations.exports evidence.exports.cancel evidence.exports.retry")
	register("monitoring", "business", "monitoring", "monitoring.", "imports.create upload.complete imports.mapping imports.validate imports.resolve imports.publish imports.readopt imports.cancel batches.revoke exports.create exports.download jobs.cancel sources.download selections.create snapshots.renew")
	add("monitoring", "business", "execution", "business", "monitoring", "monitoring.imports.published", businessFields)
	add("monitoring", "maintenance", "maintenance", "tenant", "monitoring", "monitoring.operator.verify.queued monitoring.operator.routes.create monitoring.operator.routes.copy monitoring.operator.routes.switch monitoring.operator.routes.revert monitoring.operator.routes.pause monitoring.operator.routes.switched monitoring.operator.summaries.rebuild monitoring.operator.summaries.rebuilt monitoring.operator.jobs.retry monitoring.operator.restore.open", businessFields)
}
