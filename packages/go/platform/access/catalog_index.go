package access

import (
	"slices"
	"sync"
)

// Only release-defined metadata is indexed here. Account status, memberships,
// role assignments, grants and authorization revisions still come from the
// current transaction. Public policy lookups return independent slices.
var activePermissionIndex = sync.OnceValue(func() map[Code]bool {
	index := map[Code]bool{}
	for _, permission := range Catalog() {
		index[permission.Code] = index[permission.Code] || permission.Lifecycle == "active"
	}
	return index
})

type policyRoute struct{ method, path string }
type policyIndexes struct {
	operations map[Operation]Policy
	routes     map[policyRoute]Policy
}

var releasePolicyIndexes = sync.OnceValue(func() policyIndexes {
	index := policyIndexes{operations: map[Operation]Policy{}, routes: map[policyRoute]Policy{}}
	for _, policy := range Policies() {
		// Match the first definition, as the previous linear lookup did.
		if _, exists := index.operations[policy.Operation]; !exists {
			index.operations[policy.Operation] = policy
		}
		route := policyRoute{policy.Method, policy.Path}
		if _, exists := index.routes[route]; !exists {
			index.routes[route] = policy
		}
	}
	return index
})

func copyPolicy(policy Policy) Policy {
	policy.Permissions = slices.Clone(policy.Permissions)
	return policy
}

var releaseChecksum = sync.OnceValue(calculateChecksum)
