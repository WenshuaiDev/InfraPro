package access

// Personnel operations intentionally have no tenant/project permission gate.
// Each service rejects ReadSubject and obtains the owner from the live session.
func notificationPolicies() []Policy {
	return []Policy{
		{"MyNotifications", "GET", "/api/v1/me/notifications", "authenticated", nil, false, false},
		{"MyNotificationSummary", "GET", "/api/v1/me/notifications/summary", "authenticated", nil, false, false},
		{"MyNotification", "GET", "/api/v1/me/notifications/:id", "authenticated", nil, false, false},
		{"ChangeMyNotification", "PATCH", "/api/v1/me/notifications/:id", "authenticated", nil, false, false},
		{"ReadMyNotifications", "POST", "/api/v1/me/notifications/read-all", "authenticated", nil, false, false},
		{"MyNotificationTarget", "GET", "/api/v1/me/notifications/:id/target", "authenticated", nil, false, false},
		{"MyNotificationPreferences", "GET", "/api/v1/me/notification-preferences", "authenticated", nil, false, false},
		{"SaveMyNotificationPreferences", "PUT", "/api/v1/me/notification-preferences", "authenticated", nil, false, false},
		{"MyTenantInvitations", "GET", "/api/v1/me/tenant-invitations", "authenticated", nil, false, false},
		{"MyTenantInvitation", "GET", "/api/v1/me/tenant-invitations/:id", "authenticated", nil, false, false},
		{"AcceptMyTenantInvitation", "POST", "/api/v1/me/tenant-invitations/:id/accept", "authenticated", nil, false, true},
	}
}
