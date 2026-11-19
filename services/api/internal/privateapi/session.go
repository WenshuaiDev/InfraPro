package privateapi

import (
	"context"
	"net/http"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/routes"
)

const SessionPath = "/internal/v1/sessions/verify"

type VerifyFunc func(context.Context, string) (identity.Principal, error)

func LocalVerify(service *identity.Service) VerifyFunc {
	return func(ctx context.Context, token string) (identity.Principal, error) {
		if service == nil {
			return identity.Principal{}, identity.ErrUnavailable
		}
		if e := service.Ready(ctx); e != nil {
			return identity.Principal{}, e
		}
		return service.Authenticate(ctx, token)
	}
}

type routeContextKey struct{}
type permissionRoute struct {
	Method   string `json:"method"`
	Template string `json:"template"`
}

func SessionRoute(ctx context.Context, method, path string) context.Context {
	return context.WithValue(ctx, routeContextKey{}, permissionRoute{method, path})
}

type verification struct {
	Route    permissionRoute `json:"route"`
	Deadline time.Time       `json:"deadline"`
	Token    string          `json:"session"`
}
type sessionProof struct {
	Account  string     `json:"account_id"`
	Session  string     `json:"session_id"`
	Version  int64      `json:"auth_version"`
	Activity time.Time  `json:"last_activity"`
	Recent   time.Time  `json:"reauthenticated_at"`
	Factor   *int64     `json:"factor_version,omitempty"`
	Verified *time.Time `json:"verified_at,omitempty"`
	Policy   int64      `json:"policy_generation"`
}
type verificationError struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	Status     int               `json:"status"`
	Fields     map[string]string `json:"fields,omitempty"`
	RetryAfter int               `json:"retry_after,omitempty"`
}
type verified struct {
	Proof *sessionProof      `json:"principal,omitempty"`
	Error *verificationError `json:"error,omitempty"`
}

func VerifyHandler(s *identity.Service) http.Handler {
	return Allow([]string{config.Control, config.Query, config.Transfer, config.Gateway}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in verification
		if !Read(w, r, 16<<10, &in) {
			return
		}
		caller, _ := r.Context().Value(peerKey{}).(string)
		op, exists := routes.Lookup(in.Route.Method, in.Route.Template)
		fallback := caller == config.Gateway && in.Route.Template == "" && in.Route.Method != ""
		trace := r.Header.Get(RequestID)
		if (!fallback && (!exists || op.Owner != caller || op.Class == "credential" || op.Class == "public")) || trace == "" || len(trace) > 128 {
			http.NotFound(w, r)
			return
		}
		if in.Deadline.IsZero() || !in.Deadline.After(time.Now()) {
			http.Error(w, "verification expired", http.StatusRequestTimeout)
			return
		}
		ctx, cancel := context.WithDeadline(r.Context(), in.Deadline)
		defer cancel()
		ctx, bound := context.WithTimeout(ctx, 5*time.Second)
		defer bound()
		p, e := LocalVerify(s)(WithTrace(ctx, trace), in.Token)
		if e != nil {
			err := identity.PublicError(e)
			Write(w, verified{Error: &verificationError{err.Code, err.Message, err.Status, err.Fields, err.RetryAfter}})
			return
		}
		Write(w, verified{Proof: &sessionProof{p.Account.ID, p.Session.ID, p.Version, p.Session.LastActivityAt, p.ReauthenticatedAt, p.MFAFactorVersion, p.MFAVerifiedAt, p.PolicyGeneration}})
	}))
}
func (n *Network) Verify(ctx context.Context, token string) (identity.Principal, error) {
	var p identity.Principal
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	end, _ := ctx.Deadline()
	route, _ := ctx.Value(routeContextKey{}).(permissionRoute)
	// No valid browser cookie exceeds this private schema bound. The identity
	// process still runs Ready before returning its normal credential error.
	if len(token) > 4096 {
		token = ""
	}
	var v verified
	if n.Call(ctx, n.Arch.Target(config.Identity), SessionPath, verification{Route: route, Deadline: end, Token: token}, &v, 16<<10) != nil {
		return p, identity.ErrUnavailable
	}
	if v.Error != nil {
		err := v.Error
		return p, &identity.Error{Code: err.Code, Message: err.Message, Status: err.Status, Fields: err.Fields, RetryAfter: err.RetryAfter}
	}
	s := v.Proof
	if s == nil || s.Account == "" || s.Session == "" {
		return p, identity.ErrUnavailable
	}
	p.Account.ID = s.Account
	p.Session.ID = s.Session
	p.Session.LastActivityAt = s.Activity
	p.Version = s.Version
	p.ReauthenticatedAt = s.Recent
	p.MFAFactorVersion = s.Factor
	p.MFAVerifiedAt = s.Verified
	p.PolicyGeneration = s.Policy
	return p, nil
}
