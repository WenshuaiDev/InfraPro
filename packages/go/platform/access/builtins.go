package access

const AdminKey = "platform_admin"
const MemberKey = "member"

type Builtin struct {
	Key, Name, Description string
	Permissions            []Code
}

func Builtins() []Builtin {
	// Scope is an explicit catalog property; future business permissions are not included.
	admin := []Code{}
	for _, p := range Catalog() {
		if p.Lifecycle == "active" && (p.Scope == "self" || p.Scope == "platform") {
			admin = append(admin, p.Code)
		}
	}
	return []Builtin{
		{AdminKey, "平台管理员", "平台管理及本人账号能力，由系统维护。", admin},
		{MemberKey, "基础成员", "本人资料、密码、会话及安全记录，由系统维护。", []Code{ProfileRead, ProfileUpdate, PasswordUpdate, SessionRead, SessionRevoke, SecurityEventRead}},
	}
}
func ManagementCodes() []Code { return Builtins()[0].Permissions }
