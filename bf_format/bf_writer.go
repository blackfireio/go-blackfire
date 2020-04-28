package bf_format

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/blackfireio/go-blackfire/pprof_reader"
	"github.com/blackfireio/osinfo"
)

// Write a parsed profile out as a Blackfire profile.
func WriteBFFormat(profile *pprof_reader.Profile, w io.Writer, options ProbeOptions) (err error) {
	const headerCostDimensions = "cpu pmu"
	const headerProfiledLanguage = "go"
	const headerProfilerType = "statistical"

	osInfo, err := osinfo.GetOSInfo()
	if err != nil {
		return
	}

	graphRoot := profile.BiggestImpactEntryPoint()

	// TODO: Profile title should be user-generated somehow
	// profileTitle := fmt.Sprintf(`{"blackfire-metadata":{"title":"%s"}}`, os.Args[0])

	headers := make(map[string]string)
	headers["Cost-Dimensions"] = headerCostDimensions
	headers["graph-root-id"] = graphRoot.Name
	headers["probed-os"] = osInfo.Name
	headers["profiler-type"] = headerProfilerType
	headers["probed-language"] = headerProfiledLanguage
	headers["probed-runtime"] = runtime.Version()
	headers["probed-cpu-sample-rate"] = strconv.Itoa(profile.CpuSampleRateHz)
	headers["probed-features"] = generateProbedFeaturesHeader(options)
	headers["Context"] = generateContextHeader()
	// headers["Profile-Title"] = profileTitle

	bufW := bufio.NewWriter(w)
	defer func() {
		bufErr := bufW.Flush()
		if err != nil {
			err = bufErr
		}
	}()

	if _, err = bufW.WriteString("file-format: BlackfireProbe\n"); err != nil {
		return
	}

	// Begin headers
	for k, v := range headers {
		if _, err = bufW.WriteString(fmt.Sprintf("%s: %s\n", k, v)); err != nil {
			return
		}
	}

	if options.IsTimespanFlagSet() {
		writeTimelineData(profile, bufW)
	}

	// End of headers
	if _, err = bufW.WriteString("\n"); err != nil {
		return
	}

	toUSec := func(pprofUnits int64) int64 {
		// pprof CPU units are nanoseconds
		return pprofUnits / 1000
	}

	// Profile data
	entryPoint := profile.EntryPoints[graphRoot.Name]
	for name, edge := range entryPoint.Edges {
		if _, err = bufW.WriteString(fmt.Sprintf("%s//%d %d %d\n", name,
			edge.Count, toUSec(edge.CumulativeCPUTimeValue),
			edge.CumulativeMemValue)); err != nil {
			return
		}
	}

	return
}

func generateContextHeaderFromArgs(args []string) string {
	s := strings.Builder{}
	s.WriteString("script=")
	s.WriteString(url.QueryEscape(args[0]))
	for i := 0; i < len(args); i++ {
		argv := url.QueryEscape(fmt.Sprintf("argv[%d]", i))
		value := url.QueryEscape(args[i])
		s.WriteString(fmt.Sprintf("&%s=%s", argv, value))
	}

	return s.String()
}

func generateContextHeader() string {
	return generateContextHeaderFromArgs(os.Args)
}

func writeTimelineData(profile *pprof_reader.Profile, bufW *bufio.Writer) (err error) {
	tlEntriesByEndTime := make([]*pprof_reader.TimelineEntry, 0, 10)

	var commonStackTop = &pprof_reader.Function{
		ID:   ^uint64(0) - 1,
		Name: "golang",
	}
	allCPUSamples := make([][]*pprof_reader.Function, 0, len(profile.AllCPUSamples))
	for _, stack := range profile.AllCPUSamples {
		newStack := make([]*pprof_reader.Function, len(stack)+1, len(stack)+1)
		newStack[0] = commonStackTop
		copy(newStack[1:], stack)
		allCPUSamples = append(allCPUSamples, newStack)
	}

	// Keeps track of the currently "active" functions as we move from stack to stack.
	activeTLEntries := make(map[string]*pprof_reader.TimelineEntry)

	prevStack := []*pprof_reader.Function{}
	lastMatchIndex := 0
	for sampleIndex := 0; sampleIndex < len(allCPUSamples); sampleIndex++ {
		nowStack := allCPUSamples[sampleIndex]
		prevStackLength := len(prevStack)
		nowStackLength := len(nowStack)
		shortestStackLength := prevStackLength
		if nowStackLength < shortestStackLength {
			shortestStackLength = nowStackLength
		}

		// Find the last index where the previous and current stack are in the same function.
		lastMatchIndex = 0
		for i := 1; i < shortestStackLength; i++ {
			if nowStack[i].Name != prevStack[i].Name {
				break
			}
			tlEntry := activeTLEntries[nowStack[i].Name]
			tlEntry.End++
			lastMatchIndex = i
		}

		// If the previous stack has entries that the current does not, those
		// functions have now ended. Mark them ended in leaf-to-root order.
		if lastMatchIndex < prevStackLength-1 {
			for i := prevStackLength - 1; i > lastMatchIndex; i-- {
				functionName := prevStack[i].Name
				tlEntry := activeTLEntries[functionName]
				activeTLEntries[functionName] = nil
				tlEntriesByEndTime = append(tlEntriesByEndTime, tlEntry)
			}
		}

		// If the current stack has entries that the previous does not, they
		// are newly invoked functions, so mark them started.
		if lastMatchIndex < nowStackLength-1 {
			for i := lastMatchIndex + 1; i < nowStackLength; i++ {
				tlEntry := &pprof_reader.TimelineEntry{
					Parent:   nowStack[i-1],
					Function: nowStack[i],
					Start:    uint64(sampleIndex),
					End:      uint64(sampleIndex + 1),
				}
				activeTLEntries[tlEntry.Function.Name] = tlEntry
			}
		}

		prevStack = nowStack
	}

	// Artificially end all still-active functions because the profile is ended.
	// Like before, this must be done in leaf-to-root order.
	for i := lastMatchIndex; i >= 1; i-- {
		tlEntry := activeTLEntries[prevStack[i].Name]
		tlEntriesByEndTime = append(tlEntriesByEndTime, tlEntry)
	}

	sampleIdxToUSec := func(index uint64) uint64 {
		usecPerSample := float64(1000000 / float64(profile.CpuSampleRateHz))
		return uint64(float64(index) * usecPerSample)
	}

	for i, entry := range tlEntriesByEndTime {
		start := sampleIdxToUSec(entry.Start)
		end := sampleIdxToUSec(entry.End)
		pName := entry.Parent.Name
		name := entry.Function.Name

		if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-start: %s==>%s//%d 0\n", i, pName, name, start)); err != nil {
			return
		}
		if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-end: %s==>%s//%d 0\n", i, pName, name, end)); err != nil {
			return
		}
	}

	// Overall "go" timeline layer
	index := len(tlEntriesByEndTime)
	start := uint64(0)
	end := sampleIdxToUSec(tlEntriesByEndTime[len(tlEntriesByEndTime)-1].End)
	if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-start: %s//%d 0\n", index, "go", start)); err != nil {
		return
	}
	if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-end: %s//%d 0\n", index, "go", end)); err != nil {
		return
	}

	return
}

var allowedProbedFeatures = map[string]bool{
	"signature":               true,
	"expires":                 true,
	"agentIds":                true,
	"auto_enable":             true,
	"aggreg_samples":          true,
	"flag_cpu":                true,
	"flag_memory":             true,
	"flag_no_builtins":        true,
	"flag_nw":                 true,
	"flag_fn_args":            true,
	"flag_timespan":           true,
	"flag_pdo":                true,
	"flag_sessions":           true,
	"flag_yml":                true,
	"flag_composer":           true,
	"config_yml":              true,
	"profile_title":           true,
	"sub_profile":             true,
	"timespan_threshold":      true,
	"no_pruning":              true,
	"no_signature_forwarding": true,
	"no_anon":                 true,
}

func isAllowedProbedFeature(name string) bool {
	_, ok := allowedProbedFeatures[name]
	return ok
}

func generateProbedFeaturesHeader(options ProbeOptions) string {
	var builder strings.Builder
	firstItem := true
	for k, v := range options {
		if !isAllowedProbedFeature(k) {
			continue
		}
		if !firstItem {
			builder.WriteString("&")
		}
		builder.WriteString(fmt.Sprintf("%v=%v", k, v))
		firstItem = false
	}
	return builder.String()
}

type ProbeOptions map[string]interface{}

func (p ProbeOptions) getOption(name string) interface{} {
	if value, ok := p[name]; ok {
		return value
	}
	return nil
}

func (p ProbeOptions) IsTimespanFlagSet() bool {
	return fmt.Sprintf("%v", p.getOption("flag_timespan")) == "1"
}
