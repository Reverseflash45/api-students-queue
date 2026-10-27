package service

import "testing"

func TestWithinCreditLimit(t *testing.T) {
	cases := []struct {
		taken, adding int
		want          bool
	}{
		{0, 3, true},
		{21, 3, true},
		{24, 0, true},
		{22, 3, false},
		{24, 3, false},
	}
	for _, c := range cases {
		if got := WithinCreditLimit(c.taken, c.adding); got != c.want {
			t.Errorf("WithinCreditLimit(%d, %d) = %v, mau %v", c.taken, c.adding, got, c.want)
		}
	}
}

func TestCreditsAfterSwap(t *testing.T) {
	if got := CreditsAfterSwap(24, 3, 3); got != 24 {
		t.Errorf("tukar 3 SKS dengan 3 SKS pada 24 SKS: dapat %d, mau 24", got)
	}
	if got := CreditsAfterSwap(21, 2, 4); got != 23 {
		t.Errorf("dapat %d, mau 23", got)
	}
}
