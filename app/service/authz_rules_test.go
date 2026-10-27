package service

import (
	"testing"

	"github.com/Reverseflash45/api-students-queue/app/model"
	"github.com/Reverseflash45/api-students-queue/helper"
)

func permsUser() *helper.PermissionSet {
	return helper.NewPermissionSet(map[string][]string{
		"admin": {"user:list", "user:read:any", "user:update:any", "user:delete", "role:assign"},
		"staff": {"user:list", "user:read:any"},
		"user":  {},
	})
}

func TestCanAccessUser(t *testing.T) {
	perms := permsUser()
	budi := model.AuthUser{UserID: 3, Role: "user"}
	staff := model.AuthUser{UserID: 2, Role: "staff"}

	if !CanAccessUser(budi, 3, perms, "user:read:any") {
		t.Fatal("user harus boleh membaca akunnya sendiri")
	}
	if CanAccessUser(budi, 2, perms, "user:read:any") {
		t.Fatal("user tidak boleh membaca akun orang lain")
	}
	if !CanAccessUser(staff, 3, perms, "user:read:any") {
		t.Fatal("staff punya user:read:any")
	}
	if CanAccessUser(staff, 3, perms, "user:update:any") {
		t.Fatal("staff tidak punya user:update:any")
	}
}

func TestValidateAssignRole(t *testing.T) {
	perms := permsUser()
	admin := model.AuthUser{UserID: 1, Role: "admin"}

	cases := []struct {
		nama   string
		target int
		role   string
		lolos  bool
	}{
		{"role valid untuk orang lain", 3, "staff", true},
		{"turunkan ke user (role tanpa permission tetap dikenal)", 3, "user", true},
		{"role kosong", 3, "  ", false},
		{"role tak dikenal", 3, "superadmin", false},
		{"ubah role diri sendiri", 1, "user", false},
	}

	for _, c := range cases {
		t.Run(c.nama, func(t *testing.T) {
			errs := ValidateAssignRole(admin, c.target, model.AssignRoleRequest{Role: c.role}, perms)
			if (len(errs) == 0) != c.lolos {
				t.Fatalf("errs = %v, harapan lolos = %v", errs, c.lolos)
			}
		})
	}
}
