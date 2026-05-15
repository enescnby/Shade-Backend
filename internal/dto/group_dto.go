package dto

// ── Group requests ───────────────────────────────────────────────────────────

type CreateGroupRequest struct {
	Name      string   `json:"name"`       // required
	MemberIDs []string `json:"member_ids"` // list of user_id (UUID strings)
}

type AddMemberRequest struct {
	UserID string `json:"user_id"` // UUID string
}

// ── Group responses ──────────────────────────────────────────────────────────

type GroupMemberResponse struct {
	UserID  string `json:"user_id"`
	ShadeID string `json:"shade_id"`
	Role    string `json:"role"`
}

type GroupResponse struct {
	GroupID   string                `json:"group_id"`
	Name      string                `json:"name"`
	OwnerID   string                `json:"owner_id"`
	AvatarURL string                `json:"avatar_url,omitempty"`
	Members   []GroupMemberResponse `json:"members"`
	CreatedAt string                `json:"created_at"`
}

// ── Invite requests / responses ──────────────────────────────────────────────

type CreateInviteRequest struct {
	GroupID string `json:"group_id,omitempty"` // omit = personal contact invite
	MaxUses int    `json:"max_uses,omitempty"` // 0 → default 1
}

type InviteResponse struct {
	Code      string  `json:"code"`
	GroupID   *string `json:"group_id,omitempty"`
	MaxUses   int     `json:"max_uses"`
	UseCount  int     `json:"use_count"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

type RedeemInviteResponse struct {
	Type    string          `json:"type"`              // "contact" | "group"
	Contact *LookupResponse `json:"contact,omitempty"` // if type == "contact"
	Group   *GroupResponse  `json:"group,omitempty"`   // if type == "group"
}
