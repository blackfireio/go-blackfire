package blackfire

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/blackfireio/go-blackfire/pprof_reader"
	"github.com/pkg/errors"
)

// globalProbe is the access point for all probe functionality. The API, signal,
// and HTTP interfaces perform all operations by proxying to globalProbe. This
// ensures that mutexes and other guards are respected, and no interface can
// trigger functionality that others can't, or in a way that others can't.
var globalProbe = newProbe()

type profilerState int

const (
	profilerStateOff profilerState = iota
	profilerStateEnabled
	profilerStateDisabled
	profilerStateSending
)

type probe struct {
	configuration         *BlackfireConfiguration
	agentClient           *agentClient
	mutex                 sync.Mutex
	profileDisableTrigger chan bool
	currentState          profilerState
	cpuProfileBuffers     []*bytes.Buffer
	memProfileBuffers     []*bytes.Buffer
	profileEndCallback    func()
	cpuSampleRate         int
}

func newProbe() *probe {
	p := &probe{
		configuration: &BlackfireConfiguration{},
	}
	p.startTriggerRearmLoop()
	return p
}

func (p *probe) Configure(config *BlackfireConfiguration) (err error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.configuration = config
	return
}

func (p *probe) IsProfiling() bool {
	if !p.configuration.canProfile() {
		return false
	}
	return p.currentState == profilerStateEnabled || p.currentState == profilerStateSending
}

func (p *probe) ProfileWithCallback(duration time.Duration, callback func()) (err error) {
	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canEnableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to enableProfiling(), but state = %v", p.currentState)
		if p.IsProfiling() {
			return ProfilerErrorAlreadyProfiling
		}
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canEnableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to enableProfiling(), but state = %v", p.currentState)
		if p.IsProfiling() {
			return ProfilerErrorAlreadyProfiling
		}
		return
	}

	if duration == 0 || duration > p.configuration.MaxProfileDuration {
		duration = p.configuration.MaxProfileDuration
	}

	if err = p.enableProfiling(); err != nil {
		return
	}

	p.profileEndCallback = callback
	channel := p.profileDisableTrigger
	shouldEndProfile := false

	go func() {
		<-time.After(duration)
		channel <- shouldEndProfile
	}()

	return
}

func (p *probe) ProfileFor(duration time.Duration) (err error) {
	return p.ProfileWithCallback(duration, nil)
}

func (p *probe) Enable() (err error) {
	return p.ProfileFor(p.configuration.MaxProfileDuration)
}

func (p *probe) Disable() (err error) {
	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canDisableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to Disable(), but state = %v", p.currentState)
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canDisableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to Disable(), but state = %v", p.currentState)
		return
	}

	p.triggerStopProfiler(false)
	return
}

func (p *probe) End() (err error) {
	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to End(), but state = %v", p.currentState)
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to End(), but state = %v", p.currentState)
		return
	}

	p.triggerStopProfiler(true)
	return
}

func (p *probe) EndAndWait() (err error) {
	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to EndAndWait(), but state = %v", p.currentState)
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to EndAndWait(), but state = %v", p.currentState)
		return
	}

	Log.Debug().Msgf("Blackfire: Ending the current profile and blocking until it's uploaded")
	p.endProfile()
	Log.Debug().Msgf("Blackfire: Profile uploaded. Unblocking.")
	return
}

func (p *probe) GenerateSubProfileQuery() (string, error) {
	// FIXME: the query is empty when not ran by blackfire run
	// as for now, the query is retrieved as late as possible
	parts := strings.Split(p.configuration.BlackfireQuery, "signature=")
	if len(parts) < 2 {
		return "", errors.New("Blackfire: Unable to generate a sub-profile query")
	}
	challenge := strings.TrimRight(parts[0], "&")
	parts = strings.Split(parts[1], "&")
	signature := parts[0]
	args, err := url.ParseQuery(parts[1])
	if err != nil {
		return "", errors.Wrapf(err, "Blackfire: Unable to generate a sub-profile query")
	}
	args.Del("aggreg_samples")

	parent := ""
	parts = strings.Split(args.Get("sub_profile"), ":")
	if len(parts) > 1 {
		parent = parts[1]
	}
	token := make([]byte, 7)
	rand.Read(token)
	id := base64.StdEncoding.EncodeToString(token)
	id = strings.TrimRight(id, "=")
	id = strings.ReplaceAll(id, "+", "A")
	id = strings.ReplaceAll(id, "/", "B")
	args.Set("sub_profile", parent+":"+id[0:9])
	return challenge + "&signature=" + signature + "&" + args.Encode(), nil
}

func (p *probe) startTriggerRearmLoop() {
	go func() {
		for {
			// Use a large queue for the rare edge case where many goroutines
			// try to trigger the same channel before it gets rebuilt.
			p.profileDisableTrigger = make(chan bool, 100)
			shouldEndProfile := <-p.profileDisableTrigger
			p.onProfileDisableTriggered(shouldEndProfile, p.profileEndCallback)

		}
	}()
}

func (p *probe) addNewProfileBufferSet() {
	p.cpuProfileBuffers = append(p.cpuProfileBuffers, &bytes.Buffer{})
	p.memProfileBuffers = append(p.memProfileBuffers, &bytes.Buffer{})
}

func (p *probe) resetProfileBufferSet() {
	p.cpuProfileBuffers = p.cpuProfileBuffers[:0]
	p.memProfileBuffers = p.memProfileBuffers[:0]
}

func (p *probe) currentCPUBuffer() *bytes.Buffer {
	return p.cpuProfileBuffers[len(p.cpuProfileBuffers)-1]
}

func (p *probe) currentMemBuffer() *bytes.Buffer {
	return p.memProfileBuffers[len(p.memProfileBuffers)-1]
}

func (p *probe) prepareAgentClient() (err error) {
	if p.agentClient == nil {
		p.agentClient, err = NewAgentClient(p.configuration)
		if err != nil {
			return err
		}
	}

	return p.agentClient.StartNewRequest()
}

func (p *probe) canEnableProfiling() bool {
	switch p.currentState {
	case profilerStateOff, profilerStateDisabled:
		return true
	case profilerStateEnabled, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Blackfire: Unhandled state: %v", p.currentState))
	}
}

func (p *probe) canDisableProfiling() bool {
	switch p.currentState {
	case profilerStateEnabled:
		return true
	case profilerStateOff, profilerStateDisabled, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Blackfire: Unhandled state: %v", p.currentState))
	}
}

func (p *probe) canEndProfiling() bool {
	switch p.currentState {
	case profilerStateEnabled, profilerStateDisabled:
		return true
	case profilerStateOff, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Blackfire: Unhandled state: %v", p.currentState))
	}
}

func (p *probe) enableProfiling() error {
	Log.Debug().Msgf("Blackfire: Start profiling")

	p.addNewProfileBufferSet()

	if p.cpuSampleRate == 0 {
		p.cpuSampleRate = p.configuration.DefaultCPUSampleRateHz
	}

	// We call SetCPUProfileRate before StartCPUProfile in order to lock in our
	// desired sample rate. When SetCPUProfileRate is called with a non-zero
	// value, profiling is considered "ON". Any attempt to change the sample
	// rate without first setting it back to 0 will fail. However, since
	// SetCPUProfileRate has no return value, there's no way to check for this
	// failure (Note: it will print "runtime: cannot set cpu profile rate until
	// previous profile has finished" to stderr). Since StartCPUProfile can't
	// know if its call to SetCPUProfileRate failed, it will just carry on with
	// the profiling (at our selected rate).
	runtime.SetCPUProfileRate(0)
	if p.cpuSampleRate != golangDefaultCPUSampleRate {
		// Only pre-set if it's different from what StartCPUProfile would set.
		// This avoids the unsightly error message whenever possible.
		runtime.SetCPUProfileRate(p.cpuSampleRate)
	}
	if err := pprof.StartCPUProfile(p.currentCPUBuffer()); err != nil {
		return err
	}

	p.currentState = profilerStateEnabled
	return nil
}

func (p *probe) disableProfiling() error {
	Log.Debug().Msgf("Blackfire: Stop profiling")
	if !p.canDisableProfiling() {
		return nil
	}

	defer func() {
		p.currentState = profilerStateDisabled
	}()

	pprof.StopCPUProfile()

	memWriter := bufio.NewWriter(p.currentMemBuffer())
	if err := pprof.WriteHeapProfile(memWriter); err != nil {
		return err
	}
	if err := memWriter.Flush(); err != nil {
		return err
	}

	return nil
}

func (p *probe) endProfile() error {
	Log.Debug().Msgf("Blackfire: End profile")
	if !p.canEndProfiling() {
		return nil
	}

	if err := p.disableProfiling(); err != nil {
		return err
	}

	if err := p.prepareAgentClient(); err != nil {
		return err
	}

	p.currentState = profilerStateSending
	defer func() {
		p.currentState = profilerStateOff
	}()

	if p.configuration.ShouldDumpProfiles {
		Log.Debug().Msgf("Dumping pprof profiles to current dir")
		pprof_reader.DumpProfiles(p.cpuProfileBuffers, p.memProfileBuffers)
	}

	profile, err := pprof_reader.ReadFromPProf(p.cpuProfileBuffers, p.memProfileBuffers)
	if err != nil {
		return err
	}
	p.resetProfileBufferSet()

	if profile == nil {
		return fmt.Errorf("Profile was not created")
	}

	if !profile.HasData() {
		return nil
	}

	profileBuffer := new(bytes.Buffer)
	profile.CpuSampleRate = p.configuration.DefaultCPUSampleRateHz
	if err := pprof_reader.WriteBFFormat(profile, profileBuffer); err != nil {
		return err
	}

	if err := p.agentClient.SendProfile(profileBuffer.Bytes()); err != nil {
		return err
	}

	return err
}

func (p *probe) triggerStopProfiler(shouldEndProfile bool) {
	p.profileDisableTrigger <- shouldEndProfile
}

func (p *probe) onProfileDisableTriggered(shouldEndProfile bool, callback func()) {
	Log.Debug().Msgf("Blackfire: Received profile disable trigger. shouldEndProfile = %t, callback = %p", shouldEndProfile, callback)
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if shouldEndProfile {
		if err := p.endProfile(); err != nil {
			Log.Error().Msgf("Blackfire (end profile): %v", err)
		}
	} else {
		if err := p.disableProfiling(); err != nil {
			Log.Error().Msgf("Blackfire (stop profiling): %v", err)
		}
	}

	if callback != nil {
		go callback()
	}
}
