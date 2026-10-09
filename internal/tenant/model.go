package tenant

import (
	"time"

	"github.com/lib/pq"
)

type Tenant struct {
	ID             int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	KeyIdentifier  string         `gorm:"column:key_identifier;size:100;not null;unique" json:"key_identifier"`
	AppName        string         `gorm:"column:app_name;size:100;not null" json:"app_name"`
	TenantName     string         `gorm:"column:tenant_name;size:150;not null" json:"tenant_name"`
	APIKeyHash     string         `gorm:"column:api_key_hash;size:64;not null;unique" json:"-"`
	AllowedOrigins pq.StringArray `gorm:"column:allowed_origins;type:text[];not null;default:'{}'" json:"allowed_origins"`
	IsActive       bool           `gorm:"column:is_active;not null;default:true" json:"is_active"`
	CreatedAt      time.Time      `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (Tenant) TableName() string {
	return "tenants"
}
