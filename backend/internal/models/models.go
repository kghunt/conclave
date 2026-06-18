package models

import "time"

type User struct {
	ID              string    `json:"id"`
	GoogleID        string    `json:"-"`
	Email           string    `json:"email,omitempty"`
	DisplayName     string    `json:"display_name"`
	Bio             string    `json:"bio"`
	AvatarURL       string    `json:"avatar_url"`
	IsInstanceAdmin bool      `json:"is_instance_admin,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Server struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Description            string    `json:"description"`
	IconURL                string    `json:"icon_url"`
	OwnerID                string    `json:"owner_id"`
	IsPublic               bool      `json:"is_public"`
	ShowInDiscovery        bool      `json:"show_in_discovery"`
	InviteCode             string    `json:"invite_code"`
	MemberInvitesEnabled   bool      `json:"member_invites_enabled"`
	MemberInviteExpiryDays int       `json:"member_invite_expiry_days"`
	CreatedAt              time.Time `json:"created_at"`
	// populated on list/get
	Role                   string    `json:"role,omitempty"`
}

type JoinRequest struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	User      *User     `json:"user"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Channel struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"server_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // "text" | "voice"
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UnreadCount int       `json:"unread_count,omitempty"`
}

type VoicePeer struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type Thread struct {
	ID            string    `json:"id"`
	ChannelID     string    `json:"channel_id"`
	Title         string    `json:"title"`
	CreatedBy     *User     `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	LastMessageAt time.Time `json:"last_message_at"`
	MessageCount  int       `json:"message_count"`
}

type ThreadMessage struct {
	ID        string     `json:"id"`
	ThreadID  string     `json:"thread_id"`
	Author    *User      `json:"author"`
	Content   string     `json:"content"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at,omitempty"`
}

type MessageReply struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	AuthorName string `json:"author_name"`
}

type Message struct {
	ID        string         `json:"id"`
	ChannelID string         `json:"channel_id"`
	Author    *User          `json:"author"`
	Content   string         `json:"content"`
	ReplyTo   *MessageReply  `json:"reply_to,omitempty"`
	EditedAt  *time.Time     `json:"edited_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type DMConversation struct {
	ID        string    `json:"id"`
	OtherUser *User     `json:"other_user"`
	CreatedAt time.Time `json:"created_at"`
}

type DirectMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Sender         *User     `json:"sender"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

type Invite struct {
	ID        string     `json:"id"`
	ServerID  string     `json:"server_id"`
	Code      string     `json:"code"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	MaxUses   *int       `json:"max_uses,omitempty"`
	UseCount  int        `json:"use_count"`
	CreatedAt time.Time  `json:"created_at"`
}

type ServerMember struct {
	User      *User     `json:"user"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}
