package blackfire

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/blackfireio/go-blackfire/pprof_reader"
)

type profilerState int

const (
	profilerStateIdle profilerState = iota
	profilerStateProfiling
	profilerStateSending
)

var (
	blackfireConfig             BlackfireConfiguration
	agentClient                 *AgentClient
	profilerMutex               sync.Mutex
	triggerDisableProfilingChan chan bool
	currentState                profilerState
	endOnNextProfile            bool
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
	return currentState != profilerStateIdle
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

	if duration > blackfireConfig.MaxProfileDuration {
		duration = blackfireConfig.MaxProfileDuration
	}

	if err := startProfiling(); err != nil {
		return err
	}

	channel := make(chan bool)

	go func() {
		<-channel
		onProfileDisableTriggered(callback)
	}()

	go func() {
		<-time.After(duration)
		channel <- true
	}()

	triggerDisableProfilingChan = channel

	return nil
}

// Profile the current process for the specified duration, then
// connect to the agent and upload the generated profile.
func ProfileFor(duration time.Duration) error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	return ProfileWithCallback(duration, nil)
}

// Enable starts profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func Enable() error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	return ProfileFor(blackfireConfig.MaxProfileDuration)
}

// Disable stops profiling.
func Disable() error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if currentState != profilerStateProfiling {
		// Keep it idempotent
		return nil
	}

	triggerProfilerDisable()
	return nil
}

// End stops profiling and upload the result to the agent.
func End() error {
	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	switch currentState {
	case profilerStateProfiling:
		endOnNextProfile = true
		triggerProfilerDisable()
	case profilerStateIdle:
		endOnNextProfile = true
		go func() {
			onProfileDisableTriggered(nil)
		}()
	}

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

func startProfiling() error {
	Log.Debug().Msgf("Blackfire: Start profiling")
	if currentState != profilerStateIdle {
		return ProfilerErrorAlreadyProfiling
	}

	if err := assertConfigurationIsValid(); err != nil {
		return err
	}

	addNewProfileBufferSet()

	if err := pprof.StartCPUProfile(currentCPUBuffer()); err != nil {
		return err
	}

	endOnNextProfile = false
	currentState = profilerStateProfiling
	return nil
}

func stopProfiling() error {
	Log.Debug().Msgf("Blackfire: Stop profiling")
	if currentState != profilerStateProfiling {
		return nil
	}

	defer func() {
		currentState = profilerStateIdle
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
	if err := stopProfiling(); err != nil {
		return err
	}

	if currentState != profilerStateIdle {
		return nil
	}

	if err := prepareAgentClient(); err != nil {
		return err
	}

	currentState = profilerStateSending
	defer func() {
		currentState = profilerStateIdle
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

	var fp *os.File
	fp, err = os.Create("mem.pprof")
	pprof.WriteHeapProfile(fp)

	return err
}

func triggerProfilerDisable() {
	channel := triggerDisableProfilingChan
	triggerDisableProfilingChan = make(chan bool)
	channel <- true
}

func onProfileDisableTriggered(callback func()) {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if endOnNextProfile {
		if err := endProfile(); err != nil {
			Log.Error().Msgf("Blackfire (end profile): %v", err)
		}
	} else {
		if err := stopProfiling(); err != nil {
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
