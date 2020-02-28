package blackfire

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/blackfireio/go-blackfire/pprof_reader"
)

type profilerState int

const (
	profilerStateOff profilerState = iota
	profilerStateEnabled
	profilerStateDisabled
	profilerStateSending
)

var (
	blackfireConfig             BlackfireConfiguration
	agentClient                 *AgentClient
	profilerMutex               sync.Mutex
	triggerDisableProfilingChan chan bool
	currentState                profilerState
	cpuProfileBuffers           []*bytes.Buffer
	memProfileBuffers           []*bytes.Buffer
)

var ProfilerErrorAlreadyProfiling = errors.New("A Blackfire profile is currently in progress. Please wait for it to finish.")

// Configure configures the probe (optional). This should be done before any other API calls.
// If this function isn't called, the probe will get its configuration from
// the ENV variables and the default blackfire.ini file location.
//
// Configuration is initialized in a set order, with later steps overriding
// earlier steps. Missing or zero values in manualConfig will not be applied.
// See: Zero values https://tour.golang.org/basics/12
//
// Initialization order:
// * Defaults
// * INI file
// * Environment variables
// * Manual configuration
//
// manualConfig will be ignored if nil.
// iniFilePath will be ignored if "".
func Configure(manualConfig *BlackfireConfiguration, iniFilePath string) (err error) {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()
	blackfireConfig.configure(manualConfig, iniFilePath)
	return
}

// IsProfiling checks if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	return currentState == profilerStateEnabled || currentState == profilerStateSending
}

// ProfileWithCallback profiles the current process for the specified duration.
// It also connects to the agent and upload the generated profile.
// and calls the callback in a goroutine (if not null).
func ProfileWithCallback(duration time.Duration, callback func()) error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	// Note: The check for "can enable" is inside enableProfiling()
	// It must reside there because it returns an error if we're currently
	// profiling, or sending the profile to the agent.

	if duration > blackfireConfig.MaxProfileDuration {
		duration = blackfireConfig.MaxProfileDuration
	}

	if err := enableProfiling(); err != nil {
		return err
	}

	channel := make(chan bool)

	go func() {
		shouldEndProfile := <-channel
		onProfileDisableTriggered(shouldEndProfile, callback)
	}()

	go func() {
		<-time.After(duration)
		channel <- true
	}()

	triggerDisableProfilingChan = channel

	return nil
}

// ProfileFor profiles the current process for the specified duration, then
// connects to the agent and uploads the generated profile.
func ProfileFor(duration time.Duration) error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	// Note: The check for "can enable" is inside ProfileWithCallback()

	return ProfileWithCallback(duration, nil)
}

// Enable starts profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func Enable() error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	// Note: The check for "can enable" is inside ProfileFor()

	return ProfileFor(blackfireConfig.MaxProfileDuration)
}

// Disable stops profiling.
func Disable() error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !canDisableProfiling() {
		return nil
	}

	triggerStopProfiler(false)
	return nil
}

// End stops profiling, then uploads the result to the agent in a separate
// goroutine. You must ensure that the program does not exit before uploading
// is complete. If you can't make such a guarantee, use EndAndWait() instead.
func End() error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !canEndProfiling() {
		return nil
	}

	triggerStopProfiler(true)
	return nil
}

// EndAndWait ends the current profile, then blocks until the result is uploaded
// to the agent.
func EndAndWait() error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !canEndProfiling() {
		return nil
	}

	Log.Debug().Msgf("Ending the current profile and blocking until it's uploaded")
	endProfile()
	Log.Debug().Msgf("Profile uploaded. Unblocking.")
	return nil
}

func init() {
	// Attempt a default configuration. Any errors encountered will be stored
	// and listed whenever the user makes an API call. If the user calls
	// Configure(), the errors list will get replaced.
	blackfireConfig.configure(nil, "")
}

func addNewProfileBufferSet() {
	cpuProfileBuffers = append(cpuProfileBuffers, &bytes.Buffer{})
	memProfileBuffers = append(memProfileBuffers, &bytes.Buffer{})
}

func resetProfileBufferSet() {
	cpuProfileBuffers = cpuProfileBuffers[:0]
	memProfileBuffers = memProfileBuffers[:0]
}

func currentCPUBuffer() *bytes.Buffer {
	return cpuProfileBuffers[len(cpuProfileBuffers)-1]
}

func currentMemBuffer() *bytes.Buffer {
	return memProfileBuffers[len(memProfileBuffers)-1]
}

func prepareAgentClient() (err error) {
	if agentClient != nil {
		return
	}

	if blackfireConfig.BlackfireQuery != "" {
		agentClient, err = NewAgentClient(blackfireConfig.AgentSocket, blackfireConfig.BlackfireQuery)
	} else {
		agentClient, err = NewAgentClientWithSigningRequest(blackfireConfig.AgentSocket, blackfireConfig.HTTPEndpoint, blackfireConfig.ClientId, blackfireConfig.ClientToken)
	}

	return
}

func canEnableProfiling() bool {
	switch currentState {
	case profilerStateOff, profilerStateDisabled:
		return true
	case profilerStateEnabled, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Unhandled state: %v", currentState))
	}
}

func canDisableProfiling() bool {
	switch currentState {
	case profilerStateEnabled:
		return true
	case profilerStateOff, profilerStateDisabled, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Unhandled state: %v", currentState))
	}
}

func canEndProfiling() bool {
	switch currentState {
	case profilerStateEnabled, profilerStateDisabled:
		return true
	case profilerStateOff, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Unhandled state: %v", currentState))
	}
}

func enableProfiling() error {
	Log.Debug().Msgf("Blackfire: Start profiling")

	if !canEnableProfiling() {
		if IsProfiling() {
			return ProfilerErrorAlreadyProfiling
		}
		return nil
	}

	addNewProfileBufferSet()

	if err := pprof.StartCPUProfile(currentCPUBuffer()); err != nil {
		return err
	}

	currentState = profilerStateEnabled
	return nil
}

func disableProfiling() error {
	Log.Debug().Msgf("Blackfire: Stop profiling")
	if !canDisableProfiling() {
		return nil
	}

	defer func() {
		currentState = profilerStateDisabled
	}()

	pprof.StopCPUProfile()

	memWriter := bufio.NewWriter(currentMemBuffer())
	if err := pprof.WriteHeapProfile(memWriter); err != nil {
		return err
	}
	if err := memWriter.Flush(); err != nil {
		return err
	}

	return nil
}

func endProfile() error {
	Log.Debug().Msgf("Blackfire: End profile")
	if !canEndProfiling() {
		return nil
	}

	if err := disableProfiling(); err != nil {
		return err
	}

	if err := prepareAgentClient(); err != nil {
		return err
	}

	currentState = profilerStateSending
	defer func() {
		currentState = profilerStateOff
	}()

	profile, err := pprof_reader.ReadFromPProf(cpuProfileBuffers, memProfileBuffers)
	if err != nil {
		return err
	}
	resetProfileBufferSet()

	if profile == nil {
		return fmt.Errorf("Profile was not created")
	}

	if !profile.HasData() {
		return nil
	}

	profileBuffer := new(bytes.Buffer)
	if err := pprof_reader.WriteBFFormat(profile, profile.BiggestImpactEntryPoint(), profileBuffer); err != nil {
		return err
	}

	if err := agentClient.SendProfile(profileBuffer.Bytes()); err != nil {
		return err
	}

	return err
}

func triggerStopProfiler(shouldEndProfile bool) {
	channel := triggerDisableProfilingChan
	triggerDisableProfilingChan = make(chan bool)
	channel <- shouldEndProfile
}

func onProfileDisableTriggered(shouldEndProfile bool, callback func()) {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if shouldEndProfile {
		if err := endProfile(); err != nil {
			Log.Error().Msgf("Blackfire (end profile): %v", err)
		}
	} else {
		if err := disableProfiling(); err != nil {
			Log.Error().Msgf("Blackfire (stop profiling): %v", err)
		}
	}

	if callback != nil {
		go callback()
	}
}

func assertConfigurationIsValid() error {
	if !blackfireConfig.isValid {
		return fmt.Errorf("The Blackfire profiler has an invalid configuration. "+
			"Please check your settings. You may need to call blackfire.Configure(). "+
			"Configuration errors = %v", blackfireConfig.validationErrors)
	}
	return nil
}
