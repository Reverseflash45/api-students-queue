package service

import (
	"reflect"
	"testing"
)

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{"Tugas-Banyak", " dosen-jelas ", "tugas-banyak", "", "  "})
	want := []string{"tugas-banyak", "dosen-jelas"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dapat %v, mau %v", got, want)
	}
	if got := NormalizeTags(nil); got == nil || len(got) != 0 {
		t.Errorf("nil harus menjadi slice kosong, dapat %#v", got)
	}
}
