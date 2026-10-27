package service

// MaxCreditsPerSemester adalah batas SKS yang boleh diambil dalam satu KRS.
const MaxCreditsPerSemester = 24

// WithinCreditLimit adalah aturan murni: tidak menyentuh database dan
// tidak menyentuh Fiber, jadi bisa diuji cukup dengan memanggilnya.
//
// Aturan yang benar ini TIDAK cukup untuk menjamin batas SKS. Ia hanya
// menjawab benar bila angka "taken" masih berlaku saat keputusan disimpan.
// Yang menjamin hal itu adalah kunci LockStudent, bukan fungsi ini.
func WithinCreditLimit(taken, adding int) bool {
	return taken+adding <= MaxCreditsPerSemester
}

// CreditsAfterSwap menghitung SKS setelah pindah kelas.
func CreditsAfterSwap(taken, leaving, joining int) int {
	return taken - leaving + joining
}
