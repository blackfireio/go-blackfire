package bf_format

import (
	"testing"
)

func TestGenerateContextStringFromSlice(t *testing.T) {
	args := []string{"./test", "--bar"}
	expected := "script=.%2Ftest&argv%5B0%5D=.%2Ftest&argv%5B1%5D=--bar"
	got := generateContextHeaderFromArgs(args)
	if expected != got {
		t.Errorf("generateContextStringFromSlice: Expected %v. Got %v", expected, got)
	}
}
