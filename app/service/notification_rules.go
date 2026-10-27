package service

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

var errUnknownEvent = errors.New("jenis event tidak dikenal")

// BuildMessage menyusun pesan dari sebuah event KRS. Fungsi murni: tidak
// menyentuh database, Redis, maupun email, jadi mudah diuji.
func BuildMessage(eventType string, e model.EnrollmentEvent, user model.User) (model.Message, error) {
	course := e.CourseCode + " " + e.CourseName

	var subject, body string
	switch eventType {
	case model.EventEnrollmentCreated:
		subject = "KRS: berhasil terdaftar di " + course
		body = fmt.Sprintf("Halo %s, Anda tercatat sebagai peserta kelas %s.", user.Username, course)
	case model.EventEnrollmentDropped:
		subject = "KRS: keluar dari " + course
		body = fmt.Sprintf("Halo %s, Anda sudah tidak terdaftar di kelas %s.", user.Username, course)
	case model.EventEnrollmentSwapped:
		from := e.FromCourseCode + " " + e.FromCourseName
		subject = "KRS: pindah dari " + from + " ke " + course
		body = fmt.Sprintf("Halo %s, Anda telah pindah dari kelas %s ke kelas %s.", user.Username, from, course)
	default:
		return model.Message{}, fmt.Errorf("%w: %q", errUnknownEvent, eventType)
	}

	return model.Message{To: user.Email, Subject: subject, Body: body}, nil
}

// BuildQuotaMessage menyusun pesan perubahan kuota untuk satu peserta
// (Tugas Mandiri D.2).
func BuildQuotaMessage(e model.QuotaChangedEvent, user model.User) model.Message {
	course := e.CourseCode + " " + e.CourseName
	return model.Message{
		To:      user.Email,
		Subject: "KRS: kuota " + course + " berubah menjadi " + strconv.Itoa(e.Quota),
		Body: fmt.Sprintf("Halo %s, kuota kelas %s yang Anda ikuti kini %d mahasiswa.",
			user.Username, course, e.Quota),
	}
}
