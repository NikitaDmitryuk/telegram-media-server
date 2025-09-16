package domain

import (
	"testing"
	"time"
)

func TestUserRole_String(t *testing.T) {
	tests := []struct {
		name     string
		role     UserRole
		expected string
	}{
		{"admin", AdminRole, "admin"},
		{"regular", RegularRole, "regular"},
		{"temporary", TemporaryRole, "temporary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.String(); got != tt.expected {
				t.Errorf("UserRole.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUserRole_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		role     UserRole
		expected bool
	}{
		{"admin", AdminRole, true},
		{"regular", RegularRole, true},
		{"temporary", TemporaryRole, true},
		{"invalid", "invalid", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.expected {
				t.Errorf("UserRole.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUserRole_HasPermission(t *testing.T) {
	tests := []struct {
		name     string
		role     UserRole
		action   string
		expected bool
	}{
		{"admin can download", AdminRole, "download", true},
		{"regular can download", RegularRole, "download", true},
		{"temporary can download", TemporaryRole, "download", true},
		{"admin can delete", AdminRole, "delete", true},
		{"regular can delete", RegularRole, "delete", true},
		{"temporary cannot delete", TemporaryRole, "delete", false},
		{"admin can manage users", AdminRole, "manage_users", true},
		{"regular cannot manage users", RegularRole, "manage_users", false},
		{"admin can generate temp password", AdminRole, "generate_temp_password", true},
		{"regular cannot generate temp password", RegularRole, "generate_temp_password", false},
		{"unknown action", AdminRole, "unknown_action", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.HasPermission(tt.action); got != tt.expected {
				t.Errorf("UserRole.HasPermission(%s) = %v, want %v", tt.action, got, tt.expected)
			}
		})
	}
}

func TestTemporaryPassword_IsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		password TemporaryPassword
		expected bool
	}{
		{
			name: "not expired",
			password: TemporaryPassword{
				ExpiresAt: now.Add(time.Hour),
			},
			expected: false,
		},
		{
			name: "expired",
			password: TemporaryPassword{
				ExpiresAt: now.Add(-time.Hour),
			},
			expected: true,
		},
		{
			name: "expires now",
			password: TemporaryPassword{
				ExpiresAt: now,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.password.IsExpired(); got != tt.expected {
				t.Errorf("TemporaryPassword.IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUser_IsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		user     User
		expected bool
	}{
		{
			name: "not expired",
			user: User{
				ExpiresAt: &[]time.Time{now.Add(time.Hour)}[0],
			},
			expected: false,
		},
		{
			name: "expired",
			user: User{
				ExpiresAt: &[]time.Time{now.Add(-time.Hour)}[0],
			},
			expected: true,
		},
		{
			name: "no expiration",
			user: User{
				ExpiresAt: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.IsExpired(); got != tt.expected {
				t.Errorf("User.IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUser_IsActive(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		user     User
		expected bool
	}{
		{
			name: "active",
			user: User{
				ExpiresAt: &[]time.Time{now.Add(time.Hour)}[0],
			},
			expected: true,
		},
		{
			name: "inactive",
			user: User{
				ExpiresAt: &[]time.Time{now.Add(-time.Hour)}[0],
			},
			expected: false,
		},
		{
			name: "no expiration",
			user: User{
				ExpiresAt: nil,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.IsActive(); got != tt.expected {
				t.Errorf("User.IsActive() = %v, want %v", got, tt.expected)
			}
		})
	}
}
