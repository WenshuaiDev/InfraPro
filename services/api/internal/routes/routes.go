// Package routes binds the public method and Gin template to one process.
package routes

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/gin-gonic/gin"
)

//go:embed ownership.tsv
var manifest string

type Operation struct {
	Method      string           `json:"method"`
	Path        string           `json:"path"`
	Owner       string           `json:"owner"`
	OperationID string           `json:"operation_id"`
	Permission  access.Operation `json:"access_operation"`
	Class       string           `json:"access_class"`
}

var Operations, byKey = loadManifest()

func loadManifest() ([]Operation, map[string]Operation) {
	var operations []Operation
	index := map[string]Operation{}
	ids := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 6 {
			panic("invalid route manifest row")
		}
		op := Operation{f[0], f[1], f[2], f[3], access.Operation(f[4]), f[5]}
		key := op.Method + " " + op.Path
		policy, ok := access.RoutePolicy(op.Method, op.Path)
		if !ok || policy.Operation != op.Permission || policy.Class != op.Class ||
			index[key].Path != "" || ids[op.OperationID] || op.OperationID == "" ||
			!slices.Contains([]string{"api", "identity-api", "control-api", "monitoring-query-api", "transfer-api"}, op.Owner) {
			panic("invalid operation ownership: " + key)
		}
		index[key] = op
		ids[op.OperationID] = true
		operations = append(operations, op)
	}
	if len(operations) != len(access.Policies()) {
		panic("missing route ownership")
	}
	return operations, index
}

func Lookup(method, path string) (Operation, bool) { op, ok := byKey[method+" "+path]; return op, ok }

// Registrar keeps group middleware with the handler when filtering by owner.
type Registrar struct {
	engine   *gin.Engine
	owner    string
	prefix   string
	handlers []gin.HandlerFunc
}

func New(r *gin.Engine, owner string) Registrar { return Registrar{engine: r, owner: owner} }
func (r Registrar) Group(prefix string, handlers ...gin.HandlerFunc) Registrar {
	r.prefix += prefix
	r.handlers = append(slices.Clone(r.handlers), handlers...)
	return r
}
func (r Registrar) Handle(method, path string, handlers ...gin.HandlerFunc) {
	path = r.prefix + path
	op, ok := byKey[method+" "+path]
	if !ok {
		panic("unassigned operation: " + method + " " + path)
	}
	if r.owner == "" || r.owner == op.Owner {
		r.engine.Handle(method, path, append(slices.Clone(r.handlers), handlers...)...)
	}
}
func (r Registrar) Check() {
	registered := map[string]bool{}
	for _, route := range r.engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for key, op := range byKey {
		if registered[key] != (r.owner == "" || op.Owner == r.owner) {
			panic(fmt.Sprintf("owner %q registration mismatch: %s", r.owner, key))
		}
	}
}
func (r Registrar) GET(path string, handlers ...gin.HandlerFunc) { r.Handle("GET", path, handlers...) }
func (r Registrar) POST(path string, handlers ...gin.HandlerFunc) {
	r.Handle("POST", path, handlers...)
}
func (r Registrar) HEAD(path string, handlers ...gin.HandlerFunc) {
	r.Handle("HEAD", path, handlers...)
}
func (r Registrar) PUT(path string, handlers ...gin.HandlerFunc) { r.Handle("PUT", path, handlers...) }
func (r Registrar) PATCH(path string, handlers ...gin.HandlerFunc) {
	r.Handle("PATCH", path, handlers...)
}
func (r Registrar) DELETE(path string, handlers ...gin.HandlerFunc) {
	r.Handle("DELETE", path, handlers...)
}
