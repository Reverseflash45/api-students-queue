package helper

import "sort"

// PermissionSet menyimpan pemetaan role -> daftar permission di memori.
// Dibaca SEKALI dari database saat aplikasi menyala (lihat main.go).
//
// map[string]struct{} adalah cara idiomatik membuat "set" di Go:
// pencarian langsung ke sasaran, dan struct{} berukuran nol byte.
type PermissionSet struct {
	byRole map[string]map[string]struct{}
}

// NewPermissionSet mengubah hasil query menjadi bentuk yang cepat dicari.
func NewPermissionSet(raw map[string][]string) *PermissionSet {
	byRole := make(map[string]map[string]struct{}, len(raw))

	for role, permissions := range raw {
		set := make(map[string]struct{}, len(permissions))
		for _, permission := range permissions {
			set[permission] = struct{}{}
		}
		byRole[role] = set
	}

	return &PermissionSet{byRole: byRole}
}

// Can menjawab: apakah role ini memiliki permission itu?
//
// FAIL CLOSED: role tak dikenal, permission tak dikenal, bahkan receiver
// nil, jawabannya selalu false. Tidak ada jalur yang mengembalikan true
// hanya karena "tidak menemukan alasan untuk menolak".
func (p *PermissionSet) Can(role, permission string) bool {
	if p == nil {
		return false
	}

	permissions, ok := p.byRole[role]
	if !ok {
		return false
	}

	_, granted := permissions[permission]
	return granted
}

// PermissionsOf mengembalikan seluruh permission milik role, terurut.
func (p *PermissionSet) PermissionsOf(role string) []string {
	result := []string{}
	if p == nil {
		return result
	}

	for permission := range p.byRole[role] {
		result = append(result, permission)
	}
	sort.Strings(result)
	return result
}

// KnownRoles mengembalikan daftar role yang dikenal sistem, terurut.
func (p *PermissionSet) KnownRoles() []string {
	result := []string{}
	if p == nil {
		return result
	}

	for role := range p.byRole {
		result = append(result, role)
	}
	sort.Strings(result)
	return result
}

// IsKnownRole dipakai saat memvalidasi permintaan pergantian role.
func (p *PermissionSet) IsKnownRole(role string) bool {
	if p == nil {
		return false
	}
	_, ok := p.byRole[role]
	return ok
}
