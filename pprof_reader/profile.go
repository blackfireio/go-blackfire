package pprof_reader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	pprof "github.com/blackfireio/go-blackfire/pprof_reader/internal/profile"
	"github.com/blackfireio/osinfo"
)

// Edge represents an edge of the graph, which is a call from one function to another.
type Edge struct {
	Count                   int64
	CumulativeWalltimeValue int64
	CumulativeMemValue      int64
	FromFunction            string
	ToFunction              string
}

func NewEdge(fromFunction string, toFunction string) *Edge {
	return &Edge{
		FromFunction: fromFunction,
		ToFunction:   toFunction,
	}
}

func (e *Edge) AddCount(count int64) {
	e.Count += count
}

func (e *Edge) AddWalltimeValue(value int64) {
	e.CumulativeWalltimeValue += value
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
	Name     string
	WTValue  int64
	MemValue int64
	Edges    map[string]*Edge
}

func NewEntryPoint(name string) *EntryPoint {
	return &EntryPoint{
		Name:  name,
		Edges: make(map[string]*Edge),
	}
}

func (ep *EntryPoint) AddStatisticalSample(stack []string, count int64, wtValue int64, memValue int64) {
	// EntryPoint's value mesures how much of the profile it encompasses
	ep.WTValue += wtValue
	ep.MemValue += memValue

	fromFunction := ""
	var edge *Edge

	generateEdgeName := func(fromFunction string, toFunction string) string {
		if fromFunction != "" {
			return fmt.Sprintf("%s==>%s", fromFunction, toFunction)
		}
		return toFunction
	}

	// Every edge from the stack gets value applied to it.
	for _, toFunction := range stack {
		edgeName := generateEdgeName(fromFunction, toFunction)
		var ok bool
		edge, ok = ep.Edges[edgeName]
		if !ok {
			edge = NewEdge(fromFunction, toFunction)
			ep.Edges[edgeName] = edge
		}
		edge.AddWalltimeValue(wtValue)
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

// Profle contains a set of entry points, which collectively contain all sampled data
type Profile struct {
	EntryPoints             map[string]*EntryPoint
	EntryPointsLargeToSmall []*EntryPoint
	CpuSampleRate           int
}

func NewProfile() *Profile {
	return &Profile{
		EntryPoints: make(map[string]*EntryPoint),
	}
}

func (p *Profile) HasData() bool {
	return len(p.EntryPoints) > 0
}

func (p *Profile) biggestImpactEntryPoint() string {
	if len(p.EntryPointsLargeToSmall) == 0 {
		panic(fmt.Errorf("No entry points found!"))
	}
	return p.EntryPointsLargeToSmall[0].Name
}

func (p *Profile) AddStatisticalSample(stack []string, count int64, wtValue int64, memValue int64) {
	entryPointName := stack[0]
	entryPoint, ok := p.EntryPoints[entryPointName]
	if !ok {
		entryPoint = NewEntryPoint(entryPointName)
		p.EntryPoints[entryPointName] = entryPoint
	}
	entryPoint.AddStatisticalSample(stack, count, wtValue, memValue)
}

func (p *Profile) Finish() {
	p.EntryPointsLargeToSmall = make([]*EntryPoint, 0, len(p.EntryPoints))
	for _, entryPoint := range p.EntryPoints {
		entryPoint.SetMinimumCounts()
		p.EntryPointsLargeToSmall = append(p.EntryPointsLargeToSmall, entryPoint)
	}

	sort.Slice(p.EntryPointsLargeToSmall, func(i, j int) bool {
		return p.EntryPointsLargeToSmall[i].WTValue > p.EntryPointsLargeToSmall[j].WTValue
	})
}

func convertPProfsToInternal(cpuProfiles, memProfiles []*pprof.Profile) *Profile {
	// All pprof profiles have count in index 0, and whatever value in index 1.
	// I haven't encountered a profile with sample value index > 1, and in fact
	// it cannot happen the way runtime.pprof does profiling atm.
	const countIndex = 0
	const valueIndex = 1

	generateFullStack := func(sample *pprof.Sample) []string {
		// A sample contains a stack trace, which is made of locations.
		// A location has one or more lines (>1 if functions are inlined).
		// Each line points to a function.
		stack := make([]string, 0, 10)
		stack = append(stack, "go")
		for i := len(sample.Location) - 1; i >= 0; i-- {
			location := sample.Location[i]
			for j := len(location.Line) - 1; j >= 0; j-- {
				line := location.Line[j]
				stack = append(stack, line.Function.Name)
			}
		}
		return stack
	}

	profile := NewProfile()

	for _, cpuProfile := range cpuProfiles {
		for _, sample := range cpuProfile.Sample {
			profile.AddStatisticalSample(generateFullStack(sample), sample.Value[countIndex], sample.Value[valueIndex], 0)
		}
	}

	for _, memProfile := range memProfiles {
		for _, sample := range memProfile.Sample {
			profile.AddStatisticalSample(generateFullStack(sample), 0, 0, sample.Value[valueIndex])
		}
	}

	profile.Finish()
	return profile
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

	profile := convertPProfsToInternal(cpuProfiles, memProfiles)
	return profile, nil
}

func DumpProfiles(cpuBuffers, memBuffers []*bytes.Buffer) (err error) {
	for i, buff := range cpuBuffers {
		filename := fmt.Sprintf("cpu-%d.pprof", i+1)
		if err = ioutil.WriteFile(filename, buff.Bytes(), 0644); err != nil {
			return
		}
	}
	for i, buff := range memBuffers {
		filename := fmt.Sprintf("mem-%d.pprof", i+1)
		if err = ioutil.WriteFile(filename, buff.Bytes(), 0644); err != nil {
			return
		}
	}
	return
}

func generateContextString() string {
	s := strings.Builder{}
	s.WriteString("script=")
	s.WriteString(url.QueryEscape(os.Args[0]))
	for i := 1; i < len(os.Args); i++ {
		argv := url.QueryEscape(fmt.Sprintf("argv[%d]", i))
		value := url.QueryEscape(os.Args[i])
		s.WriteString(fmt.Sprintf("&%s=%s", argv, value))
	}
	return s.String()
}

// Write a parsed profile out as a Blackfire profile.
func WriteBFFormat(profile *Profile, w io.Writer) error {
	osInfo, err := osinfo.GetOSInfo()
	if err != nil {
		return err
	}

	// TODO: Profile title should be user-generated somehow
	// profileTitle := fmt.Sprintf(`{"blackfire-metadata":{"title":"%s"}}`, os.Args[0])

	headers := make(map[string]string)
	headers["Cost-Dimensions"] = "cpu pmu"
	headers["graph-root-id"] = profile.biggestImpactEntryPoint()
	headers["probed-os"] = osInfo.Name
	headers["probed-language"] = "go"
	headers["probed-runtime"] = runtime.Version()
	headers["probed-cpu-sample-rate"] = strconv.Itoa(profile.CpuSampleRate)
	// headers["Profile-Title"] = profileTitle
	headers["Context"] = generateContextString()

	bufW := bufio.NewWriter(w)

	if _, err := bufW.WriteString("file-format: BlackfireProbe\n"); err != nil {
		return err
	}

	for k, v := range headers {
		if _, err := bufW.WriteString(fmt.Sprintf("%s: %s\n", k, v)); err != nil {
			return err
		}
	}

	if _, err := bufW.WriteString("\n"); err != nil {
		return err
	}

	entryPoint := profile.EntryPoints[headers["graph-root-id"]]
	for name, edge := range entryPoint.Edges {
		if _, err := bufW.WriteString(fmt.Sprintf("%s//%d %d %d\n", name, edge.Count, edge.CumulativeWalltimeValue/1000, edge.CumulativeMemValue)); err != nil {
			return err
		}

	}

	return bufW.Flush()
}
