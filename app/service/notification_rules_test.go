package service

import (
	"errors"
	"testing"

	"github.com/Reverseflash45/api-students-queue/app/model"
)

func TestBuildMessage(t *testing.T) {
	u := model.User{Username: "mhsuji", Email: "mhsuji@student.unair.ac.id"}
	e := model.EnrollmentEvent{CourseCode: "IF302", CourseName: "Keamanan Informasi",
		FromCourseCode: "IF102", FromCourseName: "Basis Data Lanjut"}

	cases := map[string]string{
		model.EventEnrollmentCreated: "KRS: berhasil terdaftar di IF302 Keamanan Informasi",
		model.EventEnrollmentDropped: "KRS: keluar dari IF302 Keamanan Informasi",
		model.EventEnrollmentSwapped: "KRS: pindah dari IF102 Basis Data Lanjut ke IF302 Keamanan Informasi",
	}
	for ev, want := range cases {
		msg, err := BuildMessage(ev, e, u)
		if err != nil || msg.Subject != want || msg.To != u.Email {
			t.Errorf("%s: dapat %q (%v), mau %q", ev, msg.Subject, err, want)
		}
	}
	if _, err := BuildMessage("course.deleted", e, u); !errors.Is(err, errUnknownEvent) {
		t.Errorf("event tak dikenal harus errUnknownEvent, dapat %v", err)
	}
}

func TestBuildQuotaMessage(t *testing.T) {
	msg := BuildQuotaMessage(model.QuotaChangedEvent{CourseCode: "IF301", CourseName: "Kecerdasan Buatan", Quota: 45},
		model.User{Username: "mhs001", Email: "mhs001@student.unair.ac.id"})
	if msg.Subject != "KRS: kuota IF301 Kecerdasan Buatan berubah menjadi 45" {
		t.Errorf("subjek salah: %q", msg.Subject)
	}
}
