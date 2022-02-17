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
func WriteBFFormat(profile *pprof_reader.Profile, w io.Writer, options ProbeOptions, title string) (err error) {
	const headerCostDimensions = "cpu pmu"
	const headerProfiledLanguage = "go"
	const headerProfilerType = "statistical"

	osInfo, err := osinfo.GetOSInfo()
	if err != nil {
		return
	}

	headers := make(map[string]string)
	headers["Cost-Dimensions"] = headerCostDimensions
	headers["graph-root-id"] = "go"
	headers["probed-os"] = osInfo.Name
	headers["profiler-type"] = headerProfilerType
	headers["probed-language"] = headerProfiledLanguage
	headers["probed-runtime"] = runtime.Version()
	headers["probed-cpu-sample-rate"] = strconv.Itoa(profile.CpuSampleRateHz)
	headers["probed-features"] = generateProbedFeaturesHeader(options)
	headers["Context"] = generateContextHeader()

	if title != "" {
		headers["Profile-Title"] = fmt.Sprintf(`{"blackfire-metadata":{"title":"%s"}}`, title)
	}

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
		if err = writeTimelineData(profile, bufW); err != nil {
			return
		}
	}

	// End of headers
	if _, err = bufW.WriteString("\n"); err != nil {
		return
	}

	// Profile data
	err = writeSamples(profile, bufW)

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

func writeSamples(profile *pprof_reader.Profile, bufW *bufio.Writer) (err error) {
	totalCPUTime := uint64(0)
	totalMemUsage := uint64(0)

	for _, sample := range profile.Samples {
		totalCPUTime += sample.CPUTime

		if len(sample.Stack) == 0 {
			continue
		}

		// Fake "go" top-of-stack
		if _, err = bufW.WriteString(fmt.Sprintf("go==>%s//%d %d %d\n",
			sample.Stack[0].Name,
			sample.Count, sample.CPUTime, sample.MemUsage)); err != nil {
			return
		}

		stackMemUsage := uint64(0)
		// Skip index 0 because every edge needs a begin and end node
		for iStack := len(sample.Stack) - 1; iStack > 0; iStack-- {
			f := sample.Stack[iStack]
			edgeMemCost := f.DistributedMemoryCost * uint64(sample.Count)
			totalMemUsage += edgeMemCost
			stackMemUsage += edgeMemCost

			fPrev := sample.Stack[iStack-1]
			if _, err = bufW.WriteString(fmt.Sprintf("%s==>%s//%d %d %d\n",
				fPrev.Name, f.Name,
				sample.Count, sample.CPUTime, stackMemUsage)); err != nil {
				return
			}
		}
	}

	if _, err = bufW.WriteString(fmt.Sprintf("==>go//%d %d %d\n", 1, totalCPUTime, totalMemUsage)); err != nil {
		return
	}

	return
}

type timelineEntry struct {
	Parent   *pprof_reader.Function
	Function *pprof_reader.Function
	CPUStart uint64
	CPUEnd   uint64
	MemStart uint64
	MemEnd   uint64
}

func (t *timelineEntry) String() string {
	return fmt.Sprintf("%v==>%v", t.Parent, t.Function)
}

func writeTimelineData(profile *pprof_reader.Profile, bufW *bufio.Writer) (err error) {
	tlEntriesByEndTime := make([]*timelineEntry, 0, 10)

	// Insert 2-level fake root so that the timeline visualizer has "go" as the
	// top of the stack.
	fakeStackTop := []*pprof_reader.Function{
		&pprof_reader.Function{
			Name:           "golang",
			ReferenceCount: 1,
		},
		&pprof_reader.Function{
			Name:           "go",
			ReferenceCount: 1,
		},
	}

	var alteredSamples []*pprof_reader.Sample
	for _, sample := range profile.Samples {
		newStack := make([]*pprof_reader.Function, 0, len(sample.Stack)+len(fakeStackTop))
		newStack = append(newStack, fakeStackTop...)
		newStack = append(newStack, sample.Stack...)
		alteredSamples = append(alteredSamples, sample.CloneWithStack(newStack))
	}
	profile = profile.CloneWithSamples(alteredSamples)

	// Keeps track of the currently "active" functions as we move from stack to stack.
	activeTLEntries := make(map[string]*timelineEntry)
	// Since these are fake, we need to manually add them to the active list.
	for _, f := range fakeStackTop {
		activeTLEntries[f.Name] = &timelineEntry{}
	}

	prevSample := &pprof_reader.Sample{}
	currentCPUTime := uint64(0)
	lastMatchStackIndex := 0
	for _, nowSample := range profile.Samples {
		prevStackEnd := len(prevSample.Stack) - 1
		nowStackEnd := len(nowSample.Stack) - 1
		shortestStackEnd := prevStackEnd
		if nowStackEnd < shortestStackEnd {
			shortestStackEnd = nowStackEnd
		}

		// Find the last index where the previous and current stack are in the same function.
		lastMatchStackIndex = 0
		for i := 0; i <= shortestStackEnd; i++ {
			if nowSample.Stack[i].Name != prevSample.Stack[i].Name {
				break
			}
			tlEntry := activeTLEntries[nowSample.Stack[i].Name]
			tlEntry.CPUEnd += nowSample.CPUTime
			lastMatchStackIndex = i
		}

		// If the previous stack has entries that the current does not, those
		// functions have now ended. Mark them ended in leaf-to-root order.
		if lastMatchStackIndex < prevStackEnd {
			for i := prevStackEnd; i > lastMatchStackIndex; i-- {
				functionName := prevSample.Stack[i].Name
				tlEntry := activeTLEntries[functionName]
				activeTLEntries[functionName] = nil
				tlEntriesByEndTime = append(tlEntriesByEndTime, tlEntry)
			}
		}

		// If the current stack has entries that the previous does not, they
		// are newly invoked functions, so mark them started.
		if lastMatchStackIndex < nowStackEnd {
			for i := lastMatchStackIndex + 1; i <= nowStackEnd; i++ {
				tlEntry := &timelineEntry{
					Parent:   nowSample.Stack[i-1],
					Function: nowSample.Stack[i],
					MemStart: nowSample.MemUsage,
					MemEnd:   nowSample.MemUsage,
					CPUStart: currentCPUTime,
					CPUEnd:   currentCPUTime + nowSample.CPUTime,
				}
				activeTLEntries[tlEntry.Function.Name] = tlEntry
			}
		}

		currentCPUTime += nowSample.CPUTime
		prevSample = nowSample
	}

	// Artificially end all still-active functions because the profile is ended.
	// Like before, this must be done in leaf-to-root order.
	for i := lastMatchStackIndex; i >= 1; i-- {
		tlEntry := activeTLEntries[prevSample.Stack[i].Name]
		tlEntriesByEndTime = append(tlEntriesByEndTime, tlEntry)
	}

	for i, entry := range tlEntriesByEndTime {
		name := entry.Function.Name

		if entry.Parent != nil {
			pName := entry.Parent.Name

			if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-start: %s==>%s//%d %d\n", i, pName, name, entry.CPUStart, entry.MemStart)); err != nil {
				return
			}
			if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-end: %s==>%s//%d %d\n", i, pName, name, entry.CPUEnd, entry.MemEnd)); err != nil {
				return
			}
		} else {
			if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-start: %s//%d %d\n", i, name, entry.CPUStart, entry.MemStart)); err != nil {
				return
			}
			if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-end: %s//%d %d\n", i, name, entry.CPUEnd, entry.MemEnd)); err != nil {
				return
			}
		}
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
	// Super ugly, but the actual type can be anything the json decoder chooses,
	// so we must go by its string representation.
	return fmt.Sprintf("%v", p.getOption("flag_timespan")) == "1"
}
