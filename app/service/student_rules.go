package service

import (
	"strings"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

// ================================================================
// BUSINESS RULES MURNI
//
// Sejak pertemuan 7, seluruh pemeriksaan BENTUK data (wajib diisi,
// panjang minimum, rentang nilai, format NIM) pindah menjadi tag
// validate pada struct request. Yang tersisa di berkas ini hanyalah
// aturan yang memang tidak dapat dinyatakan sebagai tag.
//
// Yang dihapus: ValidateCreate, ValidateReplace, validasiNIM,
// validasiNama, validasiGrade.
// ================================================================

// ApplyPatch tidak lagi mengembalikan daftar error. Pemeriksaan bentuk
// sudah selesai dikerjakan tag sebelum fungsi ini dipanggil, sehingga di
// sini tugasnya tinggal satu: menggabungkan.
func ApplyPatch(current model.Student, req model.PatchStudentRequest) model.Student {
	if req.NIM != nil {
		current.NIM = strings.TrimSpace(*req.NIM)
	}
	if req.Name != nil {
		current.Name = strings.TrimSpace(*req.Name)
	}
	if req.Grade != nil {
		current.UpdateGrade(*req.Grade)
	}
	if req.IsActive != nil {
		if *req.IsActive {
			current.Activate()
		} else {
			current.Deactivate()
		}
	}
	return current
}

// IsEmptyPatch memeriksa body PATCH yang tidak berisi field apa pun.
//
// Aturan ini tidak dapat ditulis sebagai tag: tag memeriksa satu field
// pada satu waktu, sedangkan aturan ini berbicara tentang HUBUNGAN antar
// field, setidaknya satu di antara mereka harus ada.
func IsEmptyPatch(req model.PatchStudentRequest) bool {
	return req.NIM == nil && req.Name == nil &&
		req.Grade == nil && req.IsActive == nil
}
