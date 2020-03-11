package pprof_reader

import (
	"bufio"
	"bytes"
	// "io/ioutil"
	// "os"
	// "testing"
)

func toLineSet(data []byte) map[string]bool {
	result := make(map[string]bool)
	r := bytes.NewReader(data)
	s := bufio.NewScanner(r)

	// Skip past headers, which are OS and go version dependent.
	for s.Scan() {
		if s.Text() == "" {
			break
		}
	}

	// We compare only the payload.
	for s.Scan() {
		result[s.Text()] = true
	}

	return result
}

// TODO: Disabled until the format settles down more.
// This will change again with RAM usage.
// func disableTestConversion(t *testing.T) {
// 	filename := "fixtures/wt.pprof.gz"
// 	fr, err := os.Open(filename)
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}
// 	defer fr.Close()

// 	// TODO: This will eventually load cpu and memory profile
// 	profile, err := ReadFromPProf(fr, fr)
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}

// 	expectedEntryPointCount := 6
// 	if len(profile.EntryPoints) != expectedEntryPointCount {
// 		t.Errorf("Expected %v entry points but got %v", expectedEntryPointCount, len(profile.EntryPoints))
// 	}

// 	expectedBytes, err := ioutil.ReadFile("fixtures/wt.bf")
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}
// 	expected := toLineSet(expectedBytes)

// 	var b bytes.Buffer
// 	writer := bufio.NewWriter(&b)
// 	err = WriteBFFormat(profile, writer)
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}

// 	actual := toLineSet(b.Bytes())

// 	if len(actual) != len(expected) {
// 		t.Errorf("Expected lines (%v) != actual lines (%v)", len(expected), len(actual))
// 	}

// 	for k, _ := range expected {
// 		_, ok := actual[k]
// 		if !ok {
// 			t.Errorf("Expected line [%v] not found in actual output", k)
// 		}
// 	}

// 	for k, _ := range actual {
// 		_, ok := expected[k]
// 		if !ok {
// 			t.Errorf("Unexpected line [%v] found in actual output", k)
// 		}
// 	}
// }
