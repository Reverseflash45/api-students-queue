package helper

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// Kode error yang stabil dan dapat dibaca mesin.
//
// Message boleh berubah kapan saja, ia ditulis untuk manusia.
// Code TIDAK boleh berubah, ia bagian dari kontrak API, karena client
// menuliskan percabangan berdasarkan nilainya.
const (
	CodeValidation       = "VALIDATION_ERROR"
	CodeBadRequest       = "BAD_REQUEST"
	CodeUnauthorized     = "UNAUTHORIZED"
	CodeForbidden        = "FORBIDDEN"
	CodeNotFound         = "NOT_FOUND"
	CodeConflict         = "CONFLICT"
	CodeUnsupportedMedia = "UNSUPPORTED_MEDIA_TYPE"
	CodeNotAcceptable    = "NOT_ACCEPTABLE"
	CodeTooManyRequests  = "TOO_MANY_REQUESTS"
	CodeUnavailable      = "SERVICE_UNAVAILABLE"
	CodeInternal         = "INTERNAL_ERROR"
)

// AppError adalah satu-satunya bentuk kegagalan yang dikenal aplikasi ini.
//
// Perhatikan bahwa ia TIDAK menyentuh fiber.Ctx. Sebuah error hanya
// menggambarkan apa yang salah; urusan menuliskannya sebagai response
// diserahkan sepenuhnya kepada ErrorHandler terpusat.
type AppError struct {
	Status  int               // status HTTP yang akan dikirim
	Code    string            // kode stabil untuk client
	Message string            // penjelasan untuk manusia
	Fields  map[string]string // detail per-field, khusus kegagalan validasi
	cause   error             // error asli, untuk log, tidak pernah dikirim
}

func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap membuat errors.Is dan errors.As tetap dapat menembus AppError
// untuk menemukan error asli di bawahnya.
func (e *AppError) Unwrap() error { return e.cause }

// Cause dibaca ErrorHandler untuk keperluan log. Field aslinya berhuruf
// kecil sehingga tidak dapat disentuh package lain; yang dibuka hanyalah
// pembacaannya, dan hanya lewat method ini.
//
// Mengembalikan string, bukan error, supaya pemanggilnya tidak perlu
// memeriksa nil lebih dulu, pemeriksaan yang mudah terlupakan dan
// berakhir sebagai panic pada kegagalan 4xx yang memang tidak bercause.
func (e *AppError) CauseText() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func BadRequest(message string) *AppError {
	return &AppError{Status: fiber.StatusBadRequest, Code: CodeBadRequest, Message: message}
}

func Unauthorized(message string) *AppError {
	return &AppError{Status: fiber.StatusUnauthorized, Code: CodeUnauthorized, Message: message}
}

func Forbidden(message string) *AppError {
	return &AppError{Status: fiber.StatusForbidden, Code: CodeForbidden, Message: message}
}

func NotFound(message string) *AppError {
	return &AppError{Status: fiber.StatusNotFound, Code: CodeNotFound, Message: message}
}

func Conflict(message string) *AppError {
	return &AppError{Status: fiber.StatusConflict, Code: CodeConflict, Message: message}
}

// Validation memakai 422 Unprocessable Content, BUKAN 400.
//
// 400 berarti "permintaan Anda tidak dapat saya baca", JSON rusak,
// query string salah bentuk. 422 berarti "saya paham permintaan Anda,
// tetapi isinya melanggar aturan". Bagian C modul menuntut 422, dan
// pembedaan ini yang memungkinkan client menampilkan pesan per field.
func Validation(fields map[string]string) *AppError {
	return &AppError{
		Status: fiber.StatusUnprocessableEntity, Code: CodeValidation,
		Message: "validasi gagal", Fields: fields,
	}
}

func UnsupportedMediaType(message string) *AppError {
	return &AppError{
		Status: fiber.StatusUnsupportedMediaType, Code: CodeUnsupportedMedia, Message: message,
	}
}

func NotAcceptable(message string) *AppError {
	return &AppError{
		Status: fiber.StatusNotAcceptable, Code: CodeNotAcceptable, Message: message,
	}
}

func TooManyRequests(message string) *AppError {
	return &AppError{
		Status: fiber.StatusTooManyRequests, Code: CodeTooManyRequests, Message: message,
	}
}

func ServiceUnavailable(message string) *AppError {
	return &AppError{
		Status: fiber.StatusServiceUnavailable, Code: CodeUnavailable, Message: message,
	}
}

// PayloadTooLarge dipakai ErrorHandler untuk body yang melebihi BodyLimit.
func PayloadTooLarge(message string) *AppError {
	return &AppError{
		Status: fiber.StatusRequestEntityTooLarge, Code: "PAYLOAD_TOO_LARGE",
		Message: message,
	}
}

// FromFiberError membungkus error bawaan Fiber (misalnya 405 Method Not
// Allowed) agar bentuk responsnya tetap sama dengan kegagalan lain.
func FromFiberError(e *fiber.Error) *AppError {
	return &AppError{Status: e.Code, Code: "HTTP_ERROR", Message: e.Message}
}

// Internal sengaja memakai pesan yang seragam dan tidak informatif.
// Detail teknisnya disimpan pada cause dan hanya muncul di log, karena
// pesan error database sering membocorkan nama tabel dan struktur query.
func Internal(cause error) *AppError {
	return &AppError{
		Status: fiber.StatusInternalServerError, Code: CodeInternal,
		Message: "terjadi kesalahan pada server", cause: cause,
	}
}

// NewError dipakai bila sebuah kegagalan butuh kode yang lebih spesifik
// daripada kode umum di atas, misalnya COURSE_FULL alih-alih CONFLICT.
// Client bisa menampilkan pesan berbeda untuk "kelas penuh" dan "sudah
// terdaftar", padahal keduanya sama-sama 409.
func NewError(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}
