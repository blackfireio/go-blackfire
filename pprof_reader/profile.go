package pprof_reader

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	pprof "github.com/blackfireio/go-blackfire/pprof_reader/internal/profile"
)

type Function struct {
	Name string

	// Memory usage is aggregated into one overall cost per function (stored as
	// MemoryCost here), so we must keep track of the number of times a function
	// is referenced in a profile, and then "distribute" the cost based on the
	// number of times it is referenced across the sample call stacks of a
	// profile. This value is calculated and cached in DistributedMemoryCost
	MemoryCost            uint64
	DistributedMemoryCost uint64
	ReferenceCount        int
}

func (f *Function) AddReferences(count int) {
	f.ReferenceCount += count
	f.DistributedMemoryCost = f.MemoryCost / uint64(f.ReferenceCount)
}

func (f *Function) String() string {
	return f.Name
}

type Sample struct {
	Count    int
	CPUTime  uint64
	MemUsage uint64
	Stack    []*Function
}

func newSample(count int, cpuTime uint64, stack []*Function) *Sample {
	return &Sample{
		Count:   count,
		CPUTime: cpuTime,
		Stack:   stack,
	}
}

func (s *Sample) CloneWithStack(stack []*Function) *Sample {
	return &Sample{
		Count:    s.Count,
		CPUTime:  s.CPUTime,
		MemUsage: s.MemUsage,
		Stack:    stack,
	}
}

// Profle contains a set of entry points, which collectively contain all sampled data
type Profile struct {
	CpuSampleRateHz int
	USecPerSample   uint64
	Samples         []*Sample
	// Note: Matching by ID didn't work since there seems to be some duplication
	// in the pprof data. We match by name instead since it's guaranteed unique.
	Functions map[string]*Function
}

func NewProfile() *Profile {
	return &Profile{
		Functions: make(map[string]*Function),
	}
}

func (p *Profile) CloneWithSamples(samples []*Sample) *Profile {
	return &Profile{
		CpuSampleRateHz: p.CpuSampleRateHz,
		USecPerSample:   p.USecPerSample,
		Samples:         samples,
		Functions:       p.Functions,
	}
}

func (p *Profile) getMatchingFunction(pf *pprof.Function) *Function {
	f, ok := p.Functions[pf.Name]
	if !ok {
		f = &Function{
			Name: pf.Name,
		}
		p.Functions[pf.Name] = f
	}

	return f
}

func (p *Profile) setCPUSampleRate(hz int) {
	p.CpuSampleRateHz = hz
	p.USecPerSample = uint64(1000000 / float64(p.CpuSampleRateHz))
}

func (p *Profile) HasData() bool {
	return len(p.Samples) > 0
}

// Read a pprof format profile and convert to our internal format.
func ReadFromPProf(cpuBuffers, memBuffers []*bytes.Buffer) (*Profile, error) {
	profile := NewProfile()

	for _, buffer := range memBuffers {
		if p, err := pprof.Parse(buffer); err != nil {
			return nil, err
		} else {
			profile.addMemorySamples(p)
		}
	}

	for _, buffer := range cpuBuffers {
		if p, err := pprof.Parse(buffer); err != nil {
			return nil, err
		} else {
			profile.USecPerSample = uint64(p.Period) / 1000
			profile.CpuSampleRateHz = int(1000000 / profile.USecPerSample)
			profile.addCPUSamples(p)
		}
	}

	profile.postProcessSamples()
	return profile, nil
}

func (p *Profile) addMemorySamples(pp *pprof.Profile) {
	const valueIndex = 3
	for _, sample := range pp.Sample {
		memUsage := sample.Value[valueIndex]
		if memUsage > 0 {
			loc := sample.Location[0]
			line := loc.Line[0]
			f := p.getMatchingFunction(line.Function)
			f.MemoryCost += uint64(memUsage)
		}
	}
}

func (p *Profile) addCPUSamples(pp *pprof.Profile) {
	// All pprof profiles have count in index 0, and whatever value in index 1.
	// I haven't encountered a profile with sample value index > 1, and in fact
	// it cannot happen the way runtime.pprof does profiling atm.
	const countIndex = 0
	const valueIndex = 1

	for _, sample := range pp.Sample {
		callCount := sample.Value[countIndex]
		if callCount < 1 {
			callCount = 1
		}
		cpuTime := uint64(sample.Value[valueIndex]) / 1000 // Convert ns to us

		// A sample contains a stack trace, which is made of locations.
		// A location has one or more lines (>1 if functions are inlined).
		// Each line points to a function.
		stack := make([]*Function, 0, 10)

		// PProf stack data is stored leaf-first. We need it to be root-first.
		for i := len(sample.Location) - 1; i >= 0; i-- {
			location := sample.Location[i]
			for j := len(location.Line) - 1; j >= 0; j-- {
				line := location.Line[j]
				f := p.getMatchingFunction(line.Function)
				f.AddReferences(int(callCount))
				stack = append(stack, f)
			}
		}

		p.Samples = append(p.Samples, newSample(int(callCount), cpuTime, stack))
	}
}

func (p *Profile) postProcessSamples() {
	for _, sample := range p.Samples {
		decycleStack(sample.Stack)
		memUsage := uint64(0)
		for _, f := range sample.Stack {
			memUsage += f.DistributedMemoryCost
		}
		sample.MemUsage = memUsage
	}
}

// Decycle a sample's call stack.
// If the same function is encountered multiple times in a goroutine stack,
// create duplicates with @1, @2, etc appended to the name so that they show
// up as different names in the BF visualizer.
func decycleStack(stack []*Function) {
	seen := make(map[string]int)
	for i, f := range stack {
		if dupCount, ok := seen[f.Name]; ok {
			stack[i] = &Function{
				Name:                  fmt.Sprintf("%s@%d", f.Name, dupCount),
				MemoryCost:            f.MemoryCost,
				DistributedMemoryCost: f.DistributedMemoryCost,
				ReferenceCount:        f.ReferenceCount,
			}
			seen[f.Name] = dupCount + 1
		} else {
			seen[f.Name] = 1
		}
	}
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
