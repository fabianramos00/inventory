package gormstore

import "testing"

func TestUserModelDefaultsNullableFields(t *testing.T) {
	user := userModel{Email: "nullable@example.com"}.toDomain()
	if user.FullName != "" {
		t.Fatalf("full_name = %q, want empty string", user.FullName)
	}
	if !user.IsActive {
		t.Fatal("is_active = false, want legacy default true")
	}
	if user.IsSuperuser {
		t.Fatal("is_superuser = true, want legacy default false")
	}
}
