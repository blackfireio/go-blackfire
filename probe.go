package blackfire

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/blackfireio/go-blackfire/pprof_reader"
	"github.com/pkg/errors"
)

type profilerState int

const (
	profilerStateOff profilerState = iota
	profilerStateEnabled
	profilerStateDisabled
	profilerStateSending
)

type probe struct {
	configuration         *Configuration
	agentClient           *agentClient
	mutex                 sync.Mutex
	profileDisableTrigger chan bool
	currentTitle          string
	currentState          profilerState
	cpuProfileBuffers     []*bytes.Buffer
	memProfileBuffers     []*bytes.Buffer
	profileEndCallback    func()
	cpuSampleRate         int
	ender                 Ender
	disabledFromPanic     bool
}

var errDisabledFromPanic = errors.Errorf("Probe has been disabled due to a previous panic. Please check the logs for details.")

type Ender interface {
	End()
	EndNoWait()
}

type ender struct {
	probe *probe
}

func (e *ender) End() {
	e.probe.End()
}

func (e *ender) EndNoWait() {
	e.probe.EndNoWait()
}

func newProbe() *probe {
	p := &probe{
		configuration: &Configuration{},
	}
	p.ender = &ender{
		probe: p,
	}
	p.currentTitle = "un-named profile"
	p.startTriggerRearmLoop()
	return p
}

func (p *probe) Configure(config *Configuration) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.configuration = config
	return
}

func (p *probe) IsProfiling() bool {
	if err := p.configuration.load(); err != nil {
		return false
	}
	if !p.configuration.canProfile() {
		return false
	}
	return p.currentState == profilerStateEnabled || p.currentState == profilerStateSending
}

func (p *probe) EnableNowFor(duration time.Duration) (err error) {
	if p.disabledFromPanic {
		return errDisabledFromPanic
	}
	defer func() {
		if r := recover(); r != nil {
			err = p.handlePanic(r)
		}
	}()

	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}
	logger := p.configuration.Logger

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canEnableProfiling() {
		err = errors.Errorf("unable to enable profiling as state is %v", p.currentState)
		logger.Error().Err(err).Msgf("Blackfire: wrong profiler state")
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canEnableProfiling() {
		err = errors.Errorf("unable to enable profiling as state is %v", p.currentState)
		logger.Error().Err(err).Msgf("Blackfire: wrong profiler state")
		return
	}

	if duration == 0 || duration > p.configuration.MaxProfileDuration {
		duration = p.configuration.MaxProfileDuration
	}

	if err = p.enableProfiling(); err != nil {
		return
	}

	channel := p.profileDisableTrigger
	shouldEndProfile := false

	go func() {
		<-time.After(duration)
		channel <- shouldEndProfile
	}()

	return
}

func (p *probe) EnableNow() (err error) {
	return p.EnableNowFor(p.configuration.MaxProfileDuration)
}

func (p *probe) Enable() (err error) {
	p.configuration.onDemandOnly = true
	return p.EnableNowFor(p.configuration.MaxProfileDuration)
}

func (p *probe) Disable() (err error) {
	if p.disabledFromPanic {
		return errDisabledFromPanic
	}
	defer func() {
		if r := recover(); r != nil {
			err = p.handlePanic(r)
		}
	}()

	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}
	logger := p.configuration.Logger

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canDisableProfiling() {
		err = errors.Errorf("unable to disable profiling as state is %v", p.currentState)
		logger.Error().Err(err).Msgf("Blackfire: wrong profiler state")
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canDisableProfiling() {
		err = errors.Errorf("unable to disable profiling as state is %v", p.currentState)
		logger.Error().Err(err).Msgf("Blackfire: wrong profiler state")
		return
	}

	p.triggerStopProfiler(false)
	return
}

func (p *probe) EndNoWait() (err error) {
	if p.disabledFromPanic {
		return errDisabledFromPanic
	}
	defer func() {
		if r := recover(); r != nil {
			err = p.handlePanic(r)
		}
	}()

	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}
	logger := p.configuration.Logger

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canEndProfiling() {
		err = errors.Errorf("unable to end profiling as state is %v", p.currentState)
		logger.Error().Err(err).Msgf("Blackfire: wrong profiler state")
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canEndProfiling() {
		err = errors.Errorf("unable to end profiling as state is %v", p.currentState)
		logger.Error().Err(err).Msgf("Blackfire: wrong profiler state")
		return
	}

	p.triggerStopProfiler(true)
	return
}

func (p *probe) End() (err error) {
	if p.disabledFromPanic {
		return errDisabledFromPanic
	}
	defer func() {
		if r := recover(); r != nil {
			err = p.handlePanic(r)
		}
	}()

	if err = p.configuration.load(); err != nil {
		return
	}
	if !p.configuration.canProfile() {
		return
	}
	logger := p.configuration.Logger

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !p.canEndProfiling() {
		err = errors.Errorf("unable to end profiling and wait as state is %v", p.currentState)
		logger.Error().Err(err).Msgf("Blackfire: wrong profiler state")
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.canEndProfiling() {
		err = errors.Errorf("unable to end profiling and wait as state is %v", p.currentState)
		logger.Error().Err(err).Msg("Blackfire: wrong profiler state")
		return
	}

	logger.Debug().Msg("Blackfire: Ending the current profile and blocking until it's uploaded")
	if err = p.endProfile(); err != nil {
		logger.Error().Msgf("Blackfire (end profile): %v", err)
		return
	}
	logger.Debug().Msg("Blackfire: Profile uploaded. Unblocking.")
	return
}

func (p *probe) GenerateSubProfileQuery() (s string, err error) {
	if p.disabledFromPanic {
		err = errDisabledFromPanic
		return
	}
	defer func() {
		if r := recover(); r != nil {
			err = p.handlePanic(r)
		}
	}()

	if err := p.prepareAgentClient(); err != nil {
		return "", err
	}
	currentQuery, err := p.agentClient.CurrentBlackfireQuery()
	if err != nil {
		return "", err
	}
	parts := strings.Split(currentQuery, "signature=")
	if len(parts) < 2 {
		return "", errors.New("Blackfire: Unable to generate a sub-profile query")
	}
	challenge := strings.TrimRight(parts[0], "&")
	parts = strings.Split(parts[1], "&")
	signature := parts[0]
	args := make(url.Values)
	if len(parts) > 1 {
		args, err = url.ParseQuery(parts[1])
		if err != nil {
			return "", errors.Wrapf(err, "Blackfire: Unable to generate a sub-profile query")
		}
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

func (p *probe) SetCurrentTitle(title string) {
	p.currentTitle = title
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
	if p.agentClient != nil {
		return nil
	}
	p.agentClient, err = NewAgentClient(p.configuration)
	return err
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
	logger := p.configuration.Logger
	logger.Debug().Msgf("Blackfire: Start profiling")

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
	logger := p.configuration.Logger
	logger.Debug().Msgf("Blackfire: Stop profiling")
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
	logger := p.configuration.Logger
	logger.Debug().Msgf("Blackfire: End profile")
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

	if p.configuration.PProfDumpDir != "" {
		logger.Debug().Msgf("Dumping pprof profiles to %v", p.configuration.PProfDumpDir)
		pprof_reader.DumpProfiles(p.cpuProfileBuffers, p.memProfileBuffers, p.configuration.PProfDumpDir)
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
		logger.Debug().Msgf("Blackfire: No samples recorded")
		return nil
	}

	if err := p.agentClient.SendProfile(profile, p.currentTitle); err != nil {
		return err
	}

	return err
}

func (p *probe) triggerStopProfiler(shouldEndProfile bool) {
	p.profileDisableTrigger <- shouldEndProfile
}

func (p *probe) onProfileDisableTriggered(shouldEndProfile bool, callback func()) {
	logger := p.configuration.Logger
	logger.Debug().Msgf("Blackfire: Received profile disable trigger. shouldEndProfile = %t, callback = %p", shouldEndProfile, callback)
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if shouldEndProfile {
		if err := p.endProfile(); err != nil {
			logger.Error().Msgf("Blackfire (end profile): %v", err)
		}
	} else {
		if err := p.disableProfiling(); err != nil {
			logger.Error().Msgf("Blackfire (stop profiling): %v", err)
		}
	}

	if callback != nil {
		go callback()
	}
}

func (p *probe) handlePanic(r interface{}) error {
	p.disabledFromPanic = true
	p.configuration.Logger.Error().Msgf("Unexpected panic %v. Probe has been disabled.", r)
	p.configuration.Logger.Error().Msg(string(debug.Stack()))
	return fmt.Errorf("Unexpected panic %v. Probe has been disabled.", r)
}
