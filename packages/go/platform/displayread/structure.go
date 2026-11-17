package displayread

// Version zero retains the pre-structure-extension contract. The opt-in is a
// delegation and package declaration, never inferred from newly visible nodes.
func StructureVersion(v int) int {
	if v == 2 {
		return 2
	}
	return 1
}
func NewStructureKind(kind string) bool { return kind == "monitoring_section" || kind == "borehole" }
func StructureUpgradeRequired() error {
	return Problem("structure_model_upgrade_required", "This object requires an explicitly authorized structure_model=2 view and compatible display package.", 409)
}
func RefKey(r Ref) string { return r.Kind + ":" + r.ID }
