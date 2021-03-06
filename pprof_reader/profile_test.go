package pprof_reader

import (
	"reflect"
	"testing"
)

func newTestStack(entries ...string) (stack []*Function) {
	for _, e := range entries {
		stack = append(stack, &Function{
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
