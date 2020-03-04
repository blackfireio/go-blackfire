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
	allowProfiling              = true
	blackfireConfig             BlackfireConfiguration
	agentClient                 *AgentClient
	profilerMutex               sync.Mutex
	triggerDisableProfilingChan chan bool
	currentState                profilerState
	cpuProfileBuffers           []*bytes.Buffer
	memProfileBuffers           []*bytes.Buffer
	profileEndCallback          func()
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
	if !allowProfiling {
		return
	}
	profilerMutex.Lock()
	defer profilerMutex.Unlock()
	blackfireConfig.configure(manualConfig, iniFilePath)
	return
}

// IsProfiling checks if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	if !allowProfiling {
		return false
	}
	return currentState == profilerStateEnabled || currentState == profilerStateSending
}

// ProfileWithCallback profiles the current process for the specified duration.
// It also connects to the agent and upload the generated profile.
// and calls the callback in a goroutine (if not null).
func ProfileWithCallback(duration time.Duration, callback func()) (err error) {
	if !allowProfiling {
		return
	}
	if err = assertConfigurationIsValid(); err != nil {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !canEnableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to enableProfiling(), but state = %v", currentState)
		if IsProfiling() {
			return ProfilerErrorAlreadyProfiling
		}
		return
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !canEnableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to enableProfiling(), but state = %v", currentState)
		if IsProfiling() {
			return ProfilerErrorAlreadyProfiling
		}
		return
	}

	if duration > blackfireConfig.MaxProfileDuration {
		duration = blackfireConfig.MaxProfileDuration
	}

	if err = enableProfiling(); err != nil {
		return
	}

	profileEndCallback = callback
	channel := triggerDisableProfilingChan
	shouldEndProfile := false

	go func() {
		<-time.After(duration)
		channel <- shouldEndProfile
	}()

	return
}

// ProfileFor profiles the current process for the specified duration, then
// connects to the agent and uploads the generated profile.
func ProfileFor(duration time.Duration) (err error) {
	return ProfileWithCallback(duration, nil)
}

// Enable starts profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func Enable() (err error) {
	return ProfileFor(blackfireConfig.MaxProfileDuration)
}

// Disable stops profiling.
func Disable() (err error) {
	if !allowProfiling {
		return
	}
	if err = assertConfigurationIsValid(); err != nil {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !canDisableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to Disable(), but state = %v", currentState)
		return
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !canDisableProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to Disable(), but state = %v", currentState)
		return
	}

	triggerStopProfiler(false)
	return
}

// End stops profiling, then uploads the result to the agent in a separate
// goroutine. You must ensure that the program does not exit before uploading
// is complete. If you can't make such a guarantee, use EndAndWait() instead.
func End() (err error) {
	if !allowProfiling {
		return
	}
	if err = assertConfigurationIsValid(); err != nil {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to End(), but state = %v", currentState)
		return
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to End(), but state = %v", currentState)
		return
	}

	triggerStopProfiler(true)
	return
}

// EndAndWait ends the current profile, then blocks until the result is uploaded
// to the agent.
func EndAndWait() (err error) {
	if !allowProfiling {
		return
	}
	if err = assertConfigurationIsValid(); err != nil {
		return
	}

	// Note: We do this once on each side of the mutex to be 100% sure that it's
	// impossible for deferred/idempotent calls to deadlock, here and forever.
	if !canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to EndAndWait(), but state = %v", currentState)
		return
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !canEndProfiling() {
		Log.Debug().Msgf("Blackfire: Tried to EndAndWait(), but state = %v", currentState)
		return
	}

	Log.Debug().Msgf("Blackfire: Ending the current profile and blocking until it's uploaded")
	endProfile()
	Log.Debug().Msgf("Blackfire: Profile uploaded. Unblocking.")
	return
}

// ProfileOnDemandOnly completely disables the profiler unless the BLACKFIRE_QUERY
// env variable is set. When the profiler is disabled, all API calls become no-ops.
//
// Only call this function before all other API calls. Calling it after another
// API call in this module will lead to undefined behavior.
func ProfileOnDemandOnly() {
	allowProfiling = isBlackfireQueryEnvSet()
}

func init() {
	// Attempt a default configuration. Any errors encountered will be stored
	// and listed whenever the user makes an API call. If the user calls
	// Configure(), the errors list will get replaced.
	blackfireConfig.configure(nil, "")

	startTriggerRearmLoop()
}

func startTriggerRearmLoop() {
	go func() {
		for {
			triggerDisableProfilingChan = newDisableProfilerTriggerChan()
			shouldEndProfile := <-triggerDisableProfilingChan
			onProfileDisableTriggered(shouldEndProfile, profileEndCallback)

		}
	}()
}

func newDisableProfilerTriggerChan() chan bool {
	// Use a large queue for the rare edge case where many goroutines try
	// to trigger the same channel before it gets rebuilt.
	return make(chan bool, 100)
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
		panic(fmt.Errorf("Blackfire: Unhandled state: %v", currentState))
	}
}

func canDisableProfiling() bool {
	switch currentState {
	case profilerStateEnabled:
		return true
	case profilerStateOff, profilerStateDisabled, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Blackfire: Unhandled state: %v", currentState))
	}
}

func canEndProfiling() bool {
	switch currentState {
	case profilerStateEnabled, profilerStateDisabled:
		return true
	case profilerStateOff, profilerStateSending:
		return false
	default:
		panic(fmt.Errorf("Blackfire: Unhandled state: %v", currentState))
	}
}

func enableProfiling() error {
	Log.Debug().Msgf("Blackfire: Start profiling")

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
	triggerDisableProfilingChan <- shouldEndProfile
}

func onProfileDisableTriggered(shouldEndProfile bool, callback func()) {
	Log.Debug().Msgf("Blackfire: Received profile disable trigger. shouldEndProfile = %v, callback = %p", shouldEndProfile, callback)
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
