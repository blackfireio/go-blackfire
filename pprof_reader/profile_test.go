package pprof_reader

import (
	"reflect"
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

func newTestStack(entries ...string) (stack []*Function) {
	for _, e := range entries {
		stack = append(stack, &Function{
			ID:   uint64(e[0] - 'a'),
			Name: e,
		})
	}
	return
}

func TestDecycleStack(t *testing.T) {
	expected := newTestStack("a", "b", "c", "b@1", "c@1", "d")
	actual := newTestStack("a", "b", "c", "b", "c", "d")
	decycleStack(actual)
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected %v but got %v", expected, actual)
	}
}

func TestDecycleStackComplex(t *testing.T) {
	expected := newTestStack("a", "b", "c", "b@1", "c@1", "d", "a@1", "b@2", "c@2", "f")
	actual := newTestStack("a", "b", "c", "b", "c", "d", "a", "b", "c", "f")
	decycleStack(actual)
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected %v but got %v", expected, actual)
	}
}
