package pprof_reader

import "testing"

func TestGenerateContextStringFromSlice(t *testing.T) {
	args := []string{"./test", "--bar"}
	expected := "argv%5B0%5D%3D.%2Ftest&argv%5B1%5D=--bar"
	got := generateContextStringFromSlice(args)
	if expected != got {
		t.Errorf("generateContextStringFromSlice: Expected %v. Got %v", expected, got)
	}
}
