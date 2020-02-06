package pprof_reader

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"

	pprof "github.com/blackfireio/go-blackfire/pprof_reader/internal/profile"
	"github.com/blackfireio/osinfo"
)

// Edge represents an edge of the graph, which is a call from one function to another.
type Edge struct {
	Count           int64
	CumulativeValue int64
	FromFunction    string
	ToFunction      string
}

func NewEdge(fromFunction string, toFunction string) *Edge {
	this := new(Edge)
	this.FromFunction = fromFunction
	this.ToFunction = toFunction
	return this
}

func (this *Edge) AddCount(count int64) {
	this.Count += count
}

func (this *Edge) AddValue(value int64) {
	this.CumulativeValue += value
}

func (this *Edge) SetMinimumCount() {
	// Because we are sampling, some of the functions in the stack won't have
	// actually been sampled. We just set their counts to 1.
	if this.Count == 0 {
		this.Count = 1
	}
}

// EntryPoint represents a top level entry point into a series of edges.
// All contained edges originate from this entry point.
type EntryPoint struct {
	Name  string
	Value int64
	Edges map[string]*Edge
}

func NewEntryPoint(name string) *EntryPoint {
	this := new(EntryPoint)
	this.Name = name
	this.Edges = make(map[string]*Edge)
	return this
}

func (this *EntryPoint) AddStatisticalSample(stack []string, count int64, value int64) {
	// EntryPoint's value mesures how much of the profile it encompasses
	this.Value += value

	fromFunction := ""
	var edge *Edge

	generateEdgeName := func(fromFunction string, toFunction string) string {
		if fromFunction != "" {
			return fmt.Sprintf("%v==>%v", fromFunction, toFunction)
		}
		return toFunction
	}

	// Every edge from the stack gets value applied to it.
	for _, toFunction := range stack {
		edgeName := generateEdgeName(fromFunction, toFunction)
		var ok bool
		edge, ok = this.Edges[edgeName]
		if !ok {
			edge = NewEdge(fromFunction, toFunction)
			this.Edges[edgeName] = edge
		}
		edge.AddValue(value)
		fromFunction = toFunction
	}

	// Only the leaf edge gets count applied to it.
	edge.AddCount(count)
}

func (this *EntryPoint) SetMinimumCounts() {
	for _, edge := range this.Edges {
		edge.SetMinimumCount()
	}
}

// Profle contains a set of entry points, which collectively contain all sampled data
type Profile struct {
	EntryPoints             map[string]*EntryPoint
	EntryPointsLargeToSmall []*EntryPoint
}

func NewProfile() *Profile {
	this := new(Profile)
	this.EntryPoints = make(map[string]*EntryPoint)
	return this
}

func (this *Profile) HasData() bool {
	return len(this.EntryPoints) > 0
}

func (this *Profile) BiggestImpactEntryPoint() string {
	if len(this.EntryPointsLargeToSmall) == 0 {
		panic(fmt.Errorf("No entry points found!"))
	}
	return this.EntryPointsLargeToSmall[0].Name
}

func (this *Profile) AddStatisticalSample(stack []string, count int64, value int64) {
	entryPointName := stack[0]
	entryPoint, ok := this.EntryPoints[entryPointName]
	if !ok {
		entryPoint = NewEntryPoint(entryPointName)
		this.EntryPoints[entryPointName] = entryPoint
	}
	entryPoint.AddStatisticalSample(stack, count, value)
}

func (this *Profile) Finish() {
	this.EntryPointsLargeToSmall = make([]*EntryPoint, 0, len(this.EntryPoints))
	for _, entryPoint := range this.EntryPoints {
		entryPoint.SetMinimumCounts()
		this.EntryPointsLargeToSmall = append(this.EntryPointsLargeToSmall, entryPoint)
	}

	sort.Slice(this.EntryPointsLargeToSmall, func(i, j int) bool {
		return this.EntryPointsLargeToSmall[i].Value > this.EntryPointsLargeToSmall[j].Value
	})
}

func convertPProfToInternal(pprofProfile *pprof.Profile) *Profile {
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

	for _, sample := range pprofProfile.Sample {
		profile.AddStatisticalSample(generateFullStack(sample), sample.Value[countIndex], sample.Value[valueIndex])
	}

	profile.Finish()
	return profile
}

// Read a pprof format profile and convert to our internal format.
func ReadFromPProf(r io.Reader) (*Profile, error) {
	pprofProfile, err := pprof.Parse(r)
	if err != nil {
		fmt.Printf("Could not create profile: %v", err)
		return nil, err
	}

	profile := convertPProfToInternal(pprofProfile)
	return profile, nil
}

func generateContextString() string {
	s := strings.Builder{}
	s.WriteString("script=")
	s.WriteString(url.QueryEscape(os.Args[0]))
	for i := 1; i < len(os.Args); i++ {
		argv := url.QueryEscape(fmt.Sprintf("argv[%v]", i))
		value := url.QueryEscape(os.Args[i])
		s.WriteString(fmt.Sprintf("&%v=%v", argv, value))
	}
	return s.String()
}

// Write a parsed profile out as a Blackfire profile.
func WriteBFFormat(profile *Profile, rootNodeName string, w io.Writer) error {
	osInfo, err := osinfo.GetOSInfo()
	if err != nil {
		return err
	}

	// TODO: Profile title should be user-generated somehow
	// profileTitle := fmt.Sprintf(`{"blackfire-metadata":{"title":"%v"}}`, os.Args[0])

	headers := make(map[string]string)
	headers["Cost-Dimensions"] = "wt"
	headers["graph-root-id"] = rootNodeName
	headers["probed-os"] = osInfo.Name
	headers["probed-language"] = "go"
	headers["probed-runtime"] = runtime.Version()
	// headers["Profile-Title"] = profileTitle
	headers["Context"] = generateContextString()

	bufW := bufio.NewWriter(w)

	if _, err := bufW.WriteString("file-format: BlackfireProbe\n"); err != nil {
		return err
	}

	for k, v := range headers {
		if _, err := bufW.WriteString(fmt.Sprintf("%v: %v\n", k, v)); err != nil {
			return err
		}
	}

	if _, err := bufW.WriteString("\n"); err != nil {
		return err
	}

	entryPoint := profile.EntryPoints[rootNodeName]
	for name, edge := range entryPoint.Edges {
		if _, err := bufW.WriteString(fmt.Sprintf("%v//%v %v\n", name, edge.Count, edge.CumulativeValue/1000)); err != nil {
			return err
		}

	}

	return bufW.Flush()
}
