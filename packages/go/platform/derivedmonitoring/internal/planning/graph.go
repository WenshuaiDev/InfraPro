// Package planning handles immutable dependency/range plans. Database graph
// locks, authorization and snapshot acquisition belong to the owning service.
package planning

import (
	"fmt"
	"slices"
)

type GraphError struct {
	Code string
	Path []string
}

func (e *GraphError) Error() string { return e.Code }

// Order checks the conservative union of all current and scheduled assignments.
// Historical edges remain stored, but the caller excludes superseded-only edges
// from this activation graph. Callers MUST hold the tenant graph write lock
// through validation and commit; this pure check cannot prevent write skew.
func Order(graph map[string][]string, maxDepth int) ([]string, error) {
	if maxDepth < 1 {
		return nil, fmt.Errorf("invalid_graph_limit")
	}
	states := map[string]int{}
	depths := map[string]int{}
	path := []string{}
	out := []string{}
	var visit func(string) (int, error)
	visit = func(id string) (int, error) {
		if _, derived := graph[id]; !derived {
			return 0, nil
		}
		if states[id] == 1 {
			return 0, &GraphError{"dependency_cycle", append(append([]string(nil), path...), id)}
		}
		if states[id] == 2 {
			return depths[id], nil
		}
		states[id] = 1
		path = append(path, id)
		depth := 1
		inputs := append([]string(nil), graph[id]...)
		slices.Sort(inputs)
		inputs = slices.Compact(inputs)
		for _, input := range inputs {
			if input == "" {
				return 0, fmt.Errorf("invalid_dependency")
			}
			d, e := visit(input)
			if e != nil {
				return 0, e
			}
			if d+1 > depth {
				depth = d + 1
			}
		}
		if depth > maxDepth {
			return 0, &GraphError{"dependency_depth_exceeded", append([]string(nil), path...)}
		}
		path = path[:len(path)-1]
		states[id] = 2
		depths[id] = depth
		out = append(out, id)
		return depth, nil
	}
	keys := make([]string, 0, len(graph))
	for id := range graph {
		if id == "" {
			return nil, fmt.Errorf("invalid_dependency")
		}
		keys = append(keys, id)
	}
	slices.Sort(keys)
	for _, id := range keys {
		if _, e := visit(id); e != nil {
			return nil, e
		}
	}
	return out, nil
}

func Dependents(graph map[string][]string, changed []string, maxDepth int) ([]string, error) {
	ordered, err := Order(graph, maxDepth)
	if err != nil {
		return nil, err
	}
	affected := map[string]bool{}
	for _, id := range changed {
		affected[id] = true
	}
	out := []string{}
	for _, id := range ordered {
		hit := affected[id]
		for _, input := range graph[id] {
			hit = hit || affected[input]
		}
		if hit {
			affected[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}
