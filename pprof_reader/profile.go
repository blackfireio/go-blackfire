package pprof_reader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"

	pprof "github.com/blackfireio/go-blackfire/pprof_reader/internal/profile"
	"github.com/blackfireio/osinfo"
)

type Function struct {
	ID   uint64
	Name string
}

func (f *Function) String() string {
	return f.Name
}

func newFunctionFromPProf(f *pprof.Function) *Function {
	return &Function{
		ID:   f.ID,
		Name: f.Name,
	}
}

// Edge represents an edge of the graph, which is a call from one function to another.
type Edge struct {
	Count                  int64
	CumulativeCPUTimeValue int64
	CumulativeMemValue     int64
	FromFunction           *Function
	ToFunction             *Function
	Name                   string
}

func generateEdgeName(fromFunction, toFunction *Function) string {
	if fromFunction != nil {
		return fmt.Sprintf("%s==>%s", fromFunction.Name, toFunction.Name)
	}
	return toFunction.Name
}

func NewEdge(fromFunction, toFunction *Function) *Edge {
	return &Edge{
		FromFunction: fromFunction,
		ToFunction:   toFunction,
		Name:         generateEdgeName(fromFunction, toFunction),
	}
}

func (e *Edge) String() string {
	return e.Name
}

func (e *Edge) AddCount(count int64) {
	e.Count += count
}

func (e *Edge) AddCPUtimeValue(value int64) {
	e.CumulativeCPUTimeValue += value
}

func (e *Edge) AddMemValue(value int64) {
	e.CumulativeMemValue += value
}

func (e *Edge) SetMinimumCount() {
	// Because we are sampling, some of the functions in the stack won't have
	// actually been sampled. We just set their counts to 1.
	if e.Count == 0 {
		e.Count = 1
	}
}

// EntryPoint represents a top level entry point into a series of edges.
// All contained edges originate from this entry point.
type EntryPoint struct {
	Function *Function
	CPUValue int64
	MemValue int64
	Edges    map[string]*Edge
}

func NewEntryPoint(function *Function) *EntryPoint {
	return &EntryPoint{
		Function: function,
		Edges:    make(map[string]*Edge),
	}
}

func (ep *EntryPoint) addStatisticalSample(stack []*Function, count int64, cpuValue int64, memValue int64) {
	// EntryPoint's value mesures how much of the profile it encompasses
	ep.CPUValue += cpuValue
	ep.MemValue += memValue

	fromFunction := &Function{}
	var edge *Edge

	// Every edge from the stack gets value applied to it.
	for _, toFunction := range stack {
		edgeName := generateEdgeName(fromFunction, toFunction)
		var ok bool
		edge, ok = ep.Edges[edgeName]
		if !ok {
			edge = NewEdge(fromFunction, toFunction)
			ep.Edges[edgeName] = edge
		}
		edge.AddCPUtimeValue(cpuValue)
		edge.AddMemValue(memValue)
		fromFunction = toFunction
	}

	// Only the leaf edge gets count applied to it.
	edge.AddCount(count)
}

func (ep *EntryPoint) SetMinimumCounts() {
	for _, edge := range ep.Edges {
		edge.SetMinimumCount()
	}
}

type timelineEntry struct {
	Parent   *Function
	Function *Function
	Start    uint64
	End      uint64
}

func (t *timelineEntry) String() string {
	return fmt.Sprintf("%v==>%v", t.Parent, t.Function)
}

// Profle contains a set of entry points, which collectively contain all sampled data
type Profile struct {
	EntryPoints             map[string]*EntryPoint
	EntryPointsLargeToSmall []*EntryPoint
	CpuSampleRate           int
	AllCPUSamples           [][]*Function
}

func NewProfile() *Profile {
	return &Profile{
		EntryPoints: make(map[string]*EntryPoint),
	}
}

func (p *Profile) HasData() bool {
	return len(p.EntryPoints) > 0
}

func (p *Profile) biggestImpactEntryPoint() *Function {
	if len(p.EntryPointsLargeToSmall) == 0 {
		panic(fmt.Errorf("No entry points found!"))
	}
	return p.EntryPointsLargeToSmall[0].Function
}

func generateFullStack(sample *pprof.Sample) []*Function {
	// Every stack begins with a fictional "go" root function so that the BF
	// visualizer (which is single-threaded) can display all goroutines as if
	// the whole thing were a single thread.
	var commonStackTop = &Function{
		ID:   ^uint64(0),
		Name: "go",
	}

	// A sample contains a stack trace, which is made of locations.
	// A location has one or more lines (>1 if functions are inlined).
	// Each line points to a function.
	stack := make([]*Function, 0, 10)
	stack = append(stack, commonStackTop)

	// PProf stack data is stored leaf-first. We need it to be root-first.
	for i := len(sample.Location) - 1; i >= 0; i-- {
		location := sample.Location[i]
		for j := len(location.Line) - 1; j >= 0; j-- {
			line := location.Line[j]
			stack = append(stack, newFunctionFromPProf(line.Function))
		}
	}

	decycleStack(stack)
	return stack
}

func (p *Profile) AddStatisticalSample(sample *pprof.Sample, count int64, cpuValue int64, memValue int64) {
	stack := generateFullStack(sample)
	function := stack[0]
	entryPointName := function.Name
	entryPoint, ok := p.EntryPoints[entryPointName]
	if !ok {
		entryPoint = NewEntryPoint(function)
		p.EntryPoints[entryPointName] = entryPoint
	}
	entryPoint.addStatisticalSample(stack, count, cpuValue, memValue)

	if cpuValue > 0 {
		p.AllCPUSamples = append(p.AllCPUSamples, stack)
	}
}

func (p *Profile) AddCPUSample(sample *pprof.Sample) {
	// All pprof profiles have count in index 0, and whatever value in index 1.
	// I haven't encountered a profile with sample value index > 1, and in fact
	// it cannot happen the way runtime.pprof does profiling atm.
	const countIndex = 0
	const valueIndex = 1

	p.AddStatisticalSample(sample, sample.Value[countIndex], sample.Value[valueIndex], 0)
}

func (p *Profile) AddMemSample(sample *pprof.Sample) {
	const valueIndex = 1
	p.AddStatisticalSample(sample, 0, 0, sample.Value[valueIndex])
}

func (p *Profile) Finish() {
	p.EntryPointsLargeToSmall = make([]*EntryPoint, 0, len(p.EntryPoints))
	for _, entryPoint := range p.EntryPoints {
		entryPoint.SetMinimumCounts()
		p.EntryPointsLargeToSmall = append(p.EntryPointsLargeToSmall, entryPoint)
	}

	sort.Slice(p.EntryPointsLargeToSmall, func(i, j int) bool {
		return p.EntryPointsLargeToSmall[i].CPUValue > p.EntryPointsLargeToSmall[j].CPUValue
	})
}

func decycleStack(stack []*Function) {
	// If the same function is encountered multiple times in a goroutine stack,
	// create duplicates with @1, @2, etc appended to the name so that they show
	// up as different names in the BF visualizer.
	seen := make(map[uint64]int)
	for i, f := range stack {
		if dupCount, ok := seen[f.ID]; ok {
			stack[i] = &Function{
				ID:   f.ID,
				Name: fmt.Sprintf("%s@%d", f.Name, dupCount),
			}
			seen[f.ID] = dupCount + 1
		} else {
			seen[f.ID] = 1
		}
	}
}

// Read a pprof format profile and convert to our internal format.
func ReadFromPProf(cpuBuffers, memBuffers []*bytes.Buffer) (*Profile, error) {
	cpuProfiles := []*pprof.Profile{}
	for _, buffer := range cpuBuffers {
		if profile, err := pprof.Parse(buffer); err != nil {
			return nil, err
		} else {
			cpuProfiles = append(cpuProfiles, profile)
		}
	}

	memProfiles := []*pprof.Profile{}
	for _, buffer := range memBuffers {
		if profile, err := pprof.Parse(buffer); err != nil {
			return nil, err
		} else {
			memProfiles = append(memProfiles, profile)
		}
	}

	profile := NewProfile()

	for _, cpuProfile := range cpuProfiles {
		for _, sample := range cpuProfile.Sample {
			profile.AddCPUSample(sample)
		}
	}

	for _, memProfile := range memProfiles {
		for _, sample := range memProfile.Sample {
			profile.AddMemSample(sample)
		}
	}

	profile.Finish()
	return profile, nil
}

func getBasename(path string) string {
	for i := len(path) - 1; i >= 0 && path[i] != '/'; i-- {
		if path[i] == '.' {
			return path[:i]
		}
	}
	return path
}

func getExeName() string {
	name, err := os.Executable()
	if err != nil {
		return "go-unknown"
	}
	return getBasename(path.Base(name))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getCpuProfileDumpPath(pathPrefix string, index int) string {
	return fmt.Sprintf("%v-cpu-%v.pprof", pathPrefix, index)
}

func getMemProfileDumpPath(pathPrefix string, index int) string {
	return fmt.Sprintf("%v-mem-%v.pprof", pathPrefix, index)
}

func getDumpStartIndex(pathPrefix string) int {
	index := 1
	for {
		if !fileExists(getCpuProfileDumpPath(pathPrefix, index)) &&
			!fileExists(getMemProfileDumpPath(pathPrefix, index)) {
			return index
		}
		index++
	}
}

// DumpProfiles dumps the raw golang pprof files to the specified directory.
// It uses the naming scheme exename-type-index.pprof, starting at the next
// index after the last one found in the specified directory.
func DumpProfiles(cpuBuffers, memBuffers []*bytes.Buffer, dstDir string) (err error) {
	pathPrefix := path.Join(dstDir, getExeName())
	startIndex := getDumpStartIndex(pathPrefix)

	for i, buff := range cpuBuffers {
		filename := getCpuProfileDumpPath(pathPrefix, startIndex+i)
		if err = ioutil.WriteFile(filename, buff.Bytes(), 0644); err != nil {
			return
		}
	}
	for i, buff := range memBuffers {
		filename := getMemProfileDumpPath(pathPrefix, startIndex+i)
		if err = ioutil.WriteFile(filename, buff.Bytes(), 0644); err != nil {
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

func WriteTimelineData(profile *Profile, bufW *bufio.Writer) (err error) {
	tlEntriesByEndTime := make([]*timelineEntry, 0, 10)

	// Keeps track of the currently "active" functions as we move from stack to stack.
	activeTLEntries := make(map[string]*timelineEntry)

	prevStack := []*Function{}
	lastMatchIndex := 0
	for sampleIndex := 0; sampleIndex < len(profile.AllCPUSamples); sampleIndex++ {
		nowStack := profile.AllCPUSamples[sampleIndex]
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
		// are newly invoked functions.
		if lastMatchIndex < nowStackLength-1 {
			for i := lastMatchIndex + 1; i < nowStackLength; i++ {
				tlEntry := &timelineEntry{
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

	for i := lastMatchIndex; i >= 1; i-- {
		tlEntry := activeTLEntries[prevStack[i].Name]
		tlEntriesByEndTime = append(tlEntriesByEndTime, tlEntry)
	}

	var minValue1 = func(value uint64) uint64 {
		if value == 0 {
			return 1
		}
		return value
	}

	for i, entry := range tlEntriesByEndTime {
		// Assume 10ms per sample.
		const timePerSample = 10
		// Min value 1 because the BF visualizer doesn't like 0.
		start := minValue1(entry.Start * timePerSample)
		end := minValue1(entry.End * timePerSample)
		pName := entry.Parent.Name
		name := entry.Function.Name

		if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-start: %s==>%s//%d 0\n", i, pName, name, start)); err != nil {
			return
		}
		if _, err = bufW.WriteString(fmt.Sprintf("Threshold-%d-end: %s==>%s//%d 0\n", i, pName, name, end)); err != nil {
			return
		}
	}
	return
}

// Write a parsed profile out as a Blackfire profile.
func WriteBFFormat(profile *Profile, w io.Writer) (err error) {
	// TODO: reverse this temporary fix once the BF UI is fixed
	const headerCostDimensions = "wt pmu"
	const headerProfiledLanguage = "php"
	// const headerCostDimensions = "cpu pmu"
	// const headerProfiledLanguage = "go"
	const headerProfilerType = "statistical"

	osInfo, err := osinfo.GetOSInfo()
	if err != nil {
		return
	}

	graphRoot := profile.biggestImpactEntryPoint()

	// TODO: Profile title should be user-generated somehow
	// profileTitle := fmt.Sprintf(`{"blackfire-metadata":{"title":"%s"}}`, os.Args[0])

	headers := make(map[string]string)
	headers["Cost-Dimensions"] = headerCostDimensions
	headers["graph-root-id"] = graphRoot.Name
	headers["probed-os"] = osInfo.Name
	headers["profiler-type"] = headerProfilerType
	headers["probed-language"] = headerProfiledLanguage
	headers["probed-runtime"] = runtime.Version()
	headers["probed-cpu-sample-rate"] = strconv.Itoa(profile.CpuSampleRate)
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

	for k, v := range headers {
		if _, err = bufW.WriteString(fmt.Sprintf("%s: %s\n", k, v)); err != nil {
			return
		}
	}

	WriteTimelineData(profile, bufW)

	// End of headers
	if _, err = bufW.WriteString("\n"); err != nil {
		return
	}

	entryPoint := profile.EntryPoints[graphRoot.Name]
	for name, edge := range entryPoint.Edges {
		if _, err = bufW.WriteString(fmt.Sprintf("%s//%d %d %d\n", name, edge.Count, edge.CumulativeCPUTimeValue/1000, edge.CumulativeMemValue)); err != nil {
			return
		}

	}

	return
}
