package blackfire

import (
	"bytes"
	"errors"
	"fmt"
	"log"
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
	blackfireConfig             *BlackfireConfiguration
	agentClient                 *AgentClient
	isConfigured                bool
	profilerMutex               sync.Mutex
	triggerDisableProfilingChan chan bool
	currentState                profilerState
	endOnNextProfile            bool
	cpuProfileBuffer            bytes.Buffer
)

func startProfiling() error {
	if currentState != profilerStateIdle {
		return ProfilerErrorAlreadyProfiling
	}

	if err := AssertCanProfile(); err != nil {
		return err
	}

	if err := pprof.StartCPUProfile(&cpuProfileBuffer); err != nil {
		return err
	}

	endOnNextProfile = false
	currentState = profilerStateProfiling
	return nil
}

func stopProfiling() {
	if currentState != profilerStateProfiling {
		return
	}

	pprof.StopCPUProfile()
	currentState = profilerStateIdle
	return
}

func endProfile() error {
	if currentState == profilerStateSending {
		stopProfiling()
	}

	if currentState != profilerStateIdle {
		return nil
	}

	currentState = profilerStateSending
	defer func() {
		currentState = profilerStateIdle
	}()

	profile, err := pprof_reader.ReadFromPProf(&cpuProfileBuffer)
	if err != nil {
		return err
	}

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

	return nil
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
			log.Printf("Error: blackfire.endProfile: %v", err)
		}
	} else {
		stopProfiling()
	}

	if callback != nil {
		go callback()
	}
}

// ----------
// Public API
// ----------

var ProfilerErrorNotConfigured = errors.New("Blackfire: The profiler has not been configured. Please call blackfire.Configure() before calling other functions.")
var ProfilerErrorAlreadyProfiling = errors.New("Blackfire: A Blackfire profile is currently in progress. Please wait for it to finish.")

func AssertCanProfile() error {
	if !isConfigured {
		return ProfilerErrorNotConfigured
	}
	return nil
}

// Configure the probe. This must be called before any other functions.
// Configuration is initialized in a set order, with later steps overriding
// earlier steps. Missing or zero values will not be applied.
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
	blackfireConfig = NewBlackfireConfiguration(manualConfig, iniFilePath)
	if blackfireConfig.BlackfireQuery == "" {
		if blackfireConfig.ClientId == "" || blackfireConfig.ClientToken == "" {
			return fmt.Errorf("Error: Blackfire: No Blackfire query or client ID/token found. Please add one of these to your configuration.")
		}
		agentClient, err = NewAgentClientWithSigningRequest(blackfireConfig.AgentSocket, blackfireConfig.HTTPEndpoint, blackfireConfig.ClientId, blackfireConfig.ClientToken)
	} else {
		agentClient, err = NewAgentClient(blackfireConfig.AgentSocket, blackfireConfig.BlackfireQuery)
	}
	if err == nil {
		isConfigured = true
	}
	return
}

// Check if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	return currentState != profilerStateIdle
}

// Does the following:
// - Profile the current process for the specified duration.
// - Connect to the agent and upload the generated profile.
// - Call the callback in a goroutine (if not null).
func ProfileWithCallback(duration time.Duration, callback func()) error {
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
	return ProfileWithCallback(duration, nil)
}

// Start profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func Enable() error {
	return ProfileFor(blackfireConfig.MaxProfileDuration)
}

// Stop profiling and upload the result to the agent.
func Disable() {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if currentState != profilerStateProfiling {
		return
	}

	triggerProfilerDisable()
	return
}

// Stop profiling and upload the result to the agent.
func End() {
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

	return
}
