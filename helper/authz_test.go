package helper

import (
	"reflect"
	"testing"
)

func contohSet() *PermissionSet {
	return NewPermissionSet(map[string][]string{
		"admin": {"student:list", "student:delete"},
		"staff": {"student:list"},
		"user":  {},
	})
}

func TestCan(t *testing.T) {
	p := contohSet()

	cases := []struct {
		nama       string
		role, perm string
		mau        bool
	}{
		{"admin punya hak", "admin", "student:delete", true},
		{"staff punya hak", "staff", "student:list", true},
		{"staff tidak punya hak", "staff", "student:delete", false},
		{"user tanpa permission", "user", "student:list", false},
		{"role tak dikenal ditolak", "hacker", "student:list", false},
		{"permission salah ketik ditolak", "admin", "student:lis", false},
		{"role kosong ditolak", "", "student:list", false},
	}

	for _, c := range cases {
		t.Run(c.nama, func(t *testing.T) {
			if got := p.Can(c.role, c.perm); got != c.mau {
				t.Fatalf("Can(%q, %q) = %v, mau %v", c.role, c.perm, got, c.mau)
			}
		})
	}
}

// Fail closed: PermissionSet nil tidak boleh panic dan tidak boleh mengizinkan.
func TestNilPermissionSetFailClosed(t *testing.T) {
	var p *PermissionSet

	if p.Can("admin", "student:list") {
		t.Fatal("PermissionSet nil tidak boleh mengizinkan apa pun")
	}
	if p.IsKnownRole("admin") {
		t.Fatal("PermissionSet nil tidak boleh mengenal role apa pun")
	}
	if got := p.PermissionsOf("admin"); len(got) != 0 {
		t.Fatalf("PermissionsOf pada nil harus kosong, dapat %v", got)
	}
}

func TestKnownRolesDanPermissionsOf(t *testing.T) {
	p := contohSet()

	if got := p.KnownRoles(); !reflect.DeepEqual(got, []string{"admin", "staff", "user"}) {
		t.Fatalf("KnownRoles = %v", got)
	}
	// Role 'user' tetap dikenal walau permission-nya kosong.
	if !p.IsKnownRole("user") {
		t.Fatal("role user harus dikenal")
	}
	if got := p.PermissionsOf("admin"); !reflect.DeepEqual(got, []string{"student:delete", "student:list"}) {
		t.Fatalf("PermissionsOf(admin) = %v", got)
	}
}
