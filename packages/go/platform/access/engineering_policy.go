package access

// EngineeringOperation maps a validated metamodel resource and action to its fixed policy.
func EngineeringOperation(tenant, catalog bool, kind, action string) Operation {
	prefix := "Engineering"
	if tenant {
		prefix = "TenantEngineering"
	}
	if catalog {
		prefix = "TenantEngineeringCatalog"
	}
	return Operation(prefix + ":" + kind + ":" + action)
}
func engineeringPolicies() []Policy {
	out := []Policy{}
	for _, resource := range []struct {
		kind  string
		codes []Code
	}{
		{"type-definitions", []Code{EngineeringTypeRead, EngineeringTypeEdit, EngineeringTypePublish, EngineeringTypeState}},
		{"metrics", []Code{EngineeringMetricRead, EngineeringMetricEdit, EngineeringMetricPublish, EngineeringMetricState}},
		{"units", []Code{EngineeringMetricRead, EngineeringMetricEdit, EngineeringMetricPublish, EngineeringMetricState}},
		{"monitoring-templates", []Code{EngineeringTemplateRead, EngineeringTemplateEdit, EngineeringTemplatePublish, EngineeringTemplateState}},
	} {
		for _, scope := range []string{"platform", "tenant", "catalog"} {
			if scope == "tenant" && resource.kind != "monitoring-templates" {
				continue
			}
			base := "/api/v1/platform/engineering/" + resource.kind
			class := "permission"
			codes := resource.codes
			if scope == "tenant" {
				base = "/api/v1/tenants/:tenant_id/engineering/" + resource.kind
				class = "tenant"
				codes = []Code{TenantEngineeringTemplateRead, TenantEngineeringTemplateEdit, TenantEngineeringTemplatePublish, TenantEngineeringTemplateState}
			}
			if scope == "catalog" {
				base = "/api/v1/tenants/:tenant_id/engineering/catalog/" + resource.kind
				class = "tenant"
				codes = []Code{TenantEngineeringCatalogRead}
			}
			for _, route := range []struct {
				action, method, path string
				code                 int
			}{
				{"receipt", "GET", "/requests/:request_id", 0}, {"list", "GET", "", 0}, {"detail", "GET", "/:definition_id", 0}, {"preview", "POST", "/:definition_id/preview", 0}, {"diff", "GET", "/:definition_id/diff", 0}, {"impact", "GET", "/:definition_id/versions/:version_id/impact", 0},
				{"create", "POST", "", 1}, {"save", "PUT", "/:definition_id/draft", 1}, {"new-draft", "POST", "/:definition_id/draft", 1}, {"delete-draft", "POST", "/:definition_id/draft/delete", 1}, {"copy", "POST", "/copy", 1},
				{"publish", "POST", "/:definition_id/publish", 2}, {"state", "POST", "/:definition_id/versions/:version_id/state", 3},
			} {
				if scope == "catalog" && (route.code != 0 || (route.action == "impact" || route.action == "receipt")) {
					continue
				}
				permissions := []Code{codes[0]}
				if route.code != 0 {
					permissions = append(permissions, codes[route.code])
				}
				out = append(out, Policy{EngineeringOperation(scope == "tenant", scope == "catalog", resource.kind, route.action), route.method, base + route.path, class, permissions, false, false})
			}
		}
	}
	for _, r := range []struct {
		action, method, path string
		codes                []Code
		class                string
	}{
		{"list", "GET", "/api/v1/platform/engineering/unit-categories", []Code{EngineeringMetricRead}, "permission"},
		{"save", "POST", "/api/v1/platform/engineering/unit-categories", []Code{EngineeringMetricRead, EngineeringMetricEdit}, "permission"},
		{"delete", "POST", "/api/v1/platform/engineering/unit-categories/:category_key/delete", []Code{EngineeringMetricRead, EngineeringMetricEdit}, "permission"},
		{"catalog", "GET", "/api/v1/tenants/:tenant_id/engineering/unit-categories", []Code{TenantEngineeringCatalogRead}, "tenant"},
	} {
		out = append(out, Policy{Operation("EngineeringCategories:" + r.action), r.method, r.path, r.class, r.codes, false, false})
	}
	return out
}
