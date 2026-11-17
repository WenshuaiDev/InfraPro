package displayread

import (
	"encoding/json"
	"errors"
)

// ReadError translates the owning services' established errors at the named
// read boundary. Legacy binding calls keep their original error vocabulary.
// Unknown failures remain errors for the HTTP boundary to sanitize.
func ReadError(err error) error {
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		var value struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(Bytes(cause), &value) != nil {
			continue
		}
		switch value.Code {
		case "monitoring_storage_unavailable", "monitoring_replica_unavailable", "monitoring_integrity_unavailable", "calculation_storage_unavailable", "calculation_unavailable", "monitoring_unavailable", "monitoring_restoring":
			return Problem("dependency_unavailable", "读取依赖暂不可用，请稍后重试。", 503)
		case "selection_stale", "snapshot_stale", "authorization_changed":
			return Problem("reset_required", "读取依据已变化，请重新建立查询。", 409)
		case "selection_expired", "snapshot_expired":
			return Problem("query_expired", "读取快照已过期，请重新建立查询。", 409)
		}
	}
	return err
}
