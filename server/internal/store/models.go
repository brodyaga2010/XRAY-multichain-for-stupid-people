package store

import "time"

// Роли, статусы и типы (значения совпадают с Python-версией).
const (
	RoleUser  = "user"
	RoleAdmin = "admin"

	UserPending   = "pending"
	UserApproved  = "approved"
	UserSuspended = "suspended"

	KeyTypeMulti = "multi"

	KeyActive    = "active"
	KeySuspended = "suspended"
	KeyRevoked   = "revoked"
)

// User — пользователь панели.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	Status       string
	KeyLimit     int
	SubToken     *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// VpnKey — ключ доступа.
type VpnKey struct {
	ID               int64
	OwnerID          int64
	UUID             string
	Name             string
	Type             string
	SNI              string
	Status           string
	PublicKey        string
	ShortID          string
	ConnectionURI    string
	ClientConfigJSON *string
	TrafficUp        int64
	TrafficDown      int64
	LastSeenAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// KeyWithOwner — ключ вместе с email владельца.
type KeyWithOwner struct {
	VpnKey
	OwnerEmail string
}

// AuditLog — запись журнала аудита.
type AuditLog struct {
	ID         int64
	ActorID    *int64
	ActorEmail *string
	Action     string
	TargetType string
	TargetID   string
	Metadata   string // JSON-текст
	CreatedAt  time.Time
}
