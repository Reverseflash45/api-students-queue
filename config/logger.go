package config

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger membuat logger terstruktur yang menulis ke dua tujuan sekaligus:
// layar agar terlihat saat praktikum, dan berkas logs/app.log yang dirotasi
// otomatis agar tidak membengkak tanpa batas.
func NewLogger() *slog.Logger {
	return NewLoggerFile("app.log")
}

// NewLoggerFile sama dengan NewLogger, tetapi menulis ke logs/<name>.
// Sejak pertemuan 11 API dan worker bisa berjalan sebagai dua proses, dan
// masing-masing diberi berkas log sendiri (logs/api.log, logs/worker.log)
// supaya dua proses tidak berebut merotasi berkas yang sama, dan supaya
// log worker mudah dihitung saat pengujian.
func NewLoggerFile(name string) *slog.Logger {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		panic("gagal membuat folder logs: " + err.Error())
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join("logs", name),
		MaxSize:    10, // rotasi setiap berkas mencapai 10 MB
		MaxBackups: 5,  // simpan 5 berkas lama
		MaxAge:     14, // hapus berkas yang lebih tua dari 14 hari
		Compress:   true,
	}

	// io.MultiWriter mengirim tulisan yang sama ke dua tujuan sekaligus,
	// seperti kertas karbon.
	writer := io.MultiWriter(os.Stdout, rotator)

	// Log berbentuk JSON kurang nyaman dibaca manusia, tetapi dapat disaring
	// berdasarkan field, misalnya "tampilkan semua permintaan berstatus 500".
	// Ketika jumlah barisnya mencapai jutaan, kemampuan menyaring itulah yang
	// menentukan apakah masalah dapat ditemukan.
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: parseLevel(GetEnv("LOG_LEVEL", "info")),
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
