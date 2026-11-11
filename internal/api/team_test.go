package api

import "testing"

func TestRoleScopePolicy(t *testing.T) {
	cases := []struct {
		role, scope string
		want        bool
	}{
		{"owner", "team:write", true},
		{"admin", "team:write", false},
		{"admin", "endpoints:write", true},
		{"operator", "team:write", false},
		{"operator", "deliveries:write", true},
		{"viewer", "events:read", true},
		{"viewer", "events:write", false},
		{"unknown", "events:read", false},
	}
	for _, item := range cases {
		if got := roleAllows(item.role, item.scope); got != item.want {
			t.Fatalf("%s %s: got %v want %v", item.role, item.scope, got, item.want)
		}
	}
}
