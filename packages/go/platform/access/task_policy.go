package access

func taskPolicies() []Policy {
	return []Policy{
		{"TasksList", "GET", "/api/v1/tasks", "authenticated", nil, false, false},
		{"TasksSummary", "GET", "/api/v1/tasks/summary", "authenticated", nil, false, false},
		{"TasksCatalog", "GET", "/api/v1/tasks/catalog", "authenticated", nil, false, false},
		{"TasksResolve", "GET", "/api/v1/tasks/resolve", "authenticated", nil, false, false},
		{"TasksOperations", "GET", "/api/v1/tasks/operations", "authenticated", nil, false, false},
		{"TasksDetail", "GET", "/api/v1/tasks/:id", "authenticated", nil, false, false},
		{"TasksExecutions", "GET", "/api/v1/tasks/:id/executions", "authenticated", nil, false, false},
		{"TasksEvents", "GET", "/api/v1/tasks/:id/events", "authenticated", nil, false, false},
		{"TasksCommand", "POST", "/api/v1/tasks/:id/commands", "authenticated", nil, false, false},
		{"TasksTarget", "GET", "/api/v1/tasks/:id/target", "authenticated", nil, false, false},
	}
}
