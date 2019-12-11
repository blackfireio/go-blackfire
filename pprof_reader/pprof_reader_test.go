package pprof_reader

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

func toLineSet(data []byte) map[string]bool {
	result := make(map[string]bool)
	r := bytes.NewReader(data)
	s := bufio.NewScanner(r)
	for s.Scan() {
		result[s.Text()] = true
	}

	return result
}

func TestConversion(t *testing.T) {
	filename := "fixtures/cpu.prof"
	fr, err := os.Open(filename)
	if err != nil {
		t.Error(err)
		return
	}
	defer fr.Close()

	profile, err := ReadFromPProf(fr)
	if err != nil {
		t.Error(err)
		return
	}

	expectedEntryPointCount := 6
	if len(profile.EntryPoints) != expectedEntryPointCount {
		t.Errorf("Expected %v entry points but got %v", expectedEntryPointCount, len(profile.EntryPoints))
	}

	expectedBytes, err := ioutil.ReadFile("fixtures/cpu.blackfireprof")
	if err != nil {
		t.Error(err)
		return
	}
	expected := toLineSet(expectedBytes)

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	entryPoint := profile.BiggestImpactEntryPoint()
	err = WriteBFFormat(profile, entryPoint, writer)
	if err != nil {
		t.Error(err)
		return
	}

	actual := toLineSet(b.Bytes())

	if len(actual) != len(expected) {
		t.Errorf("Expected lines (%v) != actual lines (%v)", len(expected), len(actual))
	}

	for k, _ := range expected {
		_, ok := actual[k]
		if !ok {
			t.Errorf("Expected line [%v] not found in actual output", k)
		}
	}

	for k, _ := range actual {
		_, ok := expected[k]
		if !ok {
			t.Errorf("Unexpected line [%v] found in actual output", k)
		}
	}
}
