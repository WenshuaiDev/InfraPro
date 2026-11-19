package privateapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	displayhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/display"
)

const ReadsPath = "/internal/v1/monitoring/reads/execute"

type readReply struct {
	Result json.RawMessage `json:"result,omitempty"`
	Named  bool            `json:"named"`
	Error  *display.Error  `json:"error,omitempty"`
	Status int             `json:"status"`
}

func ReadsHandler(s *display.Service) http.Handler {
	return Allow([]string{"display-runtime"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req display.BoundRead
		if !Read(w, r, 1<<20, &req) {
			return
		}
		id := r.Header.Get(RequestID)
		if id == "" || len(id) > 128 {
			http.Error(w, "invalid request identifier", 400)
			return
		}
		ctx, cancel := context.WithTimeout(WithTrace(r.Context(), id), displayread.SyncDeadline)
		defer cancel()
		out, e := s.ExecuteBoundRead(ctx, req)
		if e != nil {
			p := displayhttp.PublicError(e)
			Write(w, readReply{Error: p, Status: p.Status})
			return
		}
		body, e := json.Marshal(out)
		if e != nil || int64(len(body)) > s.Options().MaxResultBytes+(256<<10) {
			p := &display.Error{Code: "budget_exceeded", Message: "返回结果超过预算，请缩小范围或分页。", Status: 429}
			Write(w, readReply{Error: p, Status: p.Status})
			return
		}
		Write(w, readReply{Result: body, Named: out.NamedRead, Status: 200})
	}))
}
func (n *Network) ExecuteRead(ctx context.Context, req display.BoundRead) (display.Envelope, error) {
	var answer readReply
	var out display.Envelope
	if e := n.Call(ctx, n.Arch.Target(config.Query), ReadsPath, req, &answer, (16<<20)+(512<<10)); e != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, &display.Error{Code: "dependency_unavailable", Message: "当前依赖暂不可用，请稍后重试。", Status: 503}
	}
	if answer.Error != nil {
		answer.Error.Status = answer.Status
		return out, answer.Error
	}
	// Preserve numeric JSON tokens and domain-specific fields without a float64 round trip.
	var data json.RawMessage
	out.Data = &data
	if e := json.Unmarshal(answer.Result, &out); e != nil {
		return out, e
	}
	out.NamedRead = answer.Named
	return out, nil
}
