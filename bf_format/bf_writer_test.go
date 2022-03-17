package bf_format

import (
	"bytes"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/blackfireio/go-blackfire/pprof_reader"
	"github.com/blackfireio/osinfo"
	"github.com/stretchr/testify/assert"
)

type Headers map[string]interface{}

func TestGenerateContextStringFromSlice(t *testing.T) {
	args := []string{"./test", "--bar"}
	expected := "script=.%2Ftest&argv%5B0%5D=.%2Ftest&argv%5B1%5D=--bar"
	got := generateContextHeaderFromArgs(args)
	if expected != got {
		t.Errorf("generateContextStringFromSlice: Expected %v. Got %v", expected, got)
	}
}

func TestProbeOptionsAccessors(t *testing.T) {
	assert := assert.New(t)
	options := make(ProbeOptions)

	assert.Nil(options.getOption("unknown"))

	expectedValue := "This is a string"
	options["my-key"] = expectedValue
	assert.Equal(expectedValue, options.getOption("my-key"))

	assert.False(options.IsTimespanFlagSet())

	options["flag_timespan"] = 0
	assert.False(options.IsTimespanFlagSet())

	options["flag_timespan"] = 1
	assert.True(options.IsTimespanFlagSet())
}

func TestWriteBFFormat(t *testing.T) {
	validProfile := pprof_reader.NewProfile()
	validProfile.CpuSampleRateHz = 42
	validProfile.Samples = append(validProfile.Samples, &pprof_reader.Sample{
		Count:   1,
		CPUTime: 100,
	})

	cases := []struct {
		name            string
		profile         *pprof_reader.Profile
		options         ProbeOptions
		title           string
		expectedHeaders Headers
		expectedBody    string
	}{
		{
			"Empty case",
			pprof_reader.NewProfile(),
			make(ProbeOptions),
			"",
			Headers{},
			"==>go//1 0 0\n",
		},
		{
			"With Title",
			pprof_reader.NewProfile(),
			make(ProbeOptions),
			"This is my Title",
			Headers{
				"Profile-Title": `{"blackfire-metadata":{"title":"This is my Title"}}`,
			},
			"==>go//1 0 0\n",
		},
		{
			"With Features",
			pprof_reader.NewProfile(),
			ProbeOptions{
				"signature":   "abcd",
				"auto_enable": "true",
				"no_pruning":  "false",
			},
			"",
			Headers{},
			"==>go//1 0 0\n",
		},
		{
			"With invalid features",
			pprof_reader.NewProfile(),
			ProbeOptions{
				"unknown": "true",
				"ignored": "true",
			},
			"",
			Headers{"probed-features": ProbeOptions{}},
			"==>go//1 0 0\n",
		},
		{
			"With valid profile",
			validProfile,
			ProbeOptions{},
			"",
			Headers{},
			"==>go//1 100 0\n",
		},
		{
			"All mixed",
			validProfile,
			ProbeOptions{
				"signature":  "abcd",
				"unknown":    "true",
				"no_pruning": "false",
				"ignored":    "true",
			},
			"My-title",
			Headers{
				"probed-features": ProbeOptions{
					"signature":  "abcd",
					"no_pruning": "false",
				},
				"Profile-Title": `{"blackfire-metadata":{"title":"My-title"}}`,
			},
			"==>go//1 100 0\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fullHeaders := defaultHeaders(c.profile, c.options, c.expectedHeaders)
			_TestWriteBFFormat(t, c.profile, c.options, c.title, fullHeaders, c.expectedBody)
		})
	}
}

func _TestWriteBFFormat(t *testing.T, profile *pprof_reader.Profile, options ProbeOptions, title string, expectedHeaders Headers, expectedBody string) {
	assert := assert.New(t)
	var buffer bytes.Buffer

	assert.Nil(WriteBFFormat(profile, &buffer, options, title))
	// file-format must always be first
	assert.Equal("file-format: BlackfireProbe\n", buffer.String()[:28])

	parts := strings.Split(buffer.String(), "\n\n")
	assert.Equal(2, len(parts))

	assert.Equal(expectedHeaders, headersToMap(parts[0]))
	assert.Equal(expectedBody, parts[1])
}

// headersToMap Order of headers in string is not predictable.
// Then we convert them back to a map since assert library can
// handle their comparison.
func headersToMap(headers string) (m Headers) {
	m = Headers{}
	for _, line := range strings.Split(headers, "\n") {
		parts := strings.Split(line, ": ")
		m[parts[0]] = parts[1]
	}
	// probed-features are also built upon a map
	if features, found := m["probed-features"]; found {
		options := ProbeOptions{}
		if len(features.(string)) > 0 {
			for _, feature := range strings.Split(features.(string), "&") {
				parts := strings.Split(feature, "=")
				options[parts[0]] = parts[1]
			}
		}
		m["probed-features"] = options
	}
	return
}

func defaultHeaders(profile *pprof_reader.Profile, options ProbeOptions, override Headers) (headers Headers) {
	osInfo, err := osinfo.GetOSInfo()
	if err != nil {
		panic("Cannot retrieve osInfo")
	}

	headers = Headers{
		"file-format":            "BlackfireProbe",
		"Cost-Dimensions":        "cpu pmu",
		"graph-root-id":          "go",
		"probed-os":              osInfo.Name,
		"profiler-type":          "statistical",
		"probed-language":        "go",
		"probed-runtime":         runtime.Version(),
		"probed-cpu-sample-rate": strconv.Itoa(profile.CpuSampleRateHz),
		"probed-features":        options,
		"Context":                generateContextHeader(),
	}
	for k, v := range override {
		headers[k] = v
	}
	return
}
