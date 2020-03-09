package blackfire

import (
	"errors"
	"time"
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
func Configure(manualConfig *BlackfireConfiguration, iniFilePath string) error {
	return globalProbe.Configure(manualConfig, iniFilePath)
}

// IsProfiling checks if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	return globalProbe.IsProfiling()
}

// ProfileWithCallback profiles the current process for the specified duration.
// It also connects to the agent and upload the generated profile.
// and calls the callback in a goroutine (if not null).
func ProfileWithCallback(duration time.Duration, callback func()) error {
	return globalProbe.ProfileWithCallback(duration, callback)
}

// ProfileFor profiles the current process for the specified duration, then
// connects to the agent and uploads the generated profile.
func ProfileFor(duration time.Duration) error {
	return globalProbe.ProfileFor(duration)
}

// Enable starts profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func Enable() error {
	return globalProbe.Enable()
}

// Disable stops profiling.
func Disable() error {
	return globalProbe.Disable()
}

// End stops profiling, then uploads the result to the agent in a separate
// goroutine. You must ensure that the program does not exit before uploading
// is complete. If you can't make such a guarantee, use EndAndWait() instead.
func End() error {
	return globalProbe.End()
}

// EndAndWait ends the current profile, then blocks until the result is uploaded
// to the agent.
func EndAndWait() error {
	return globalProbe.EndAndWait()
}

// ProfileOnDemandOnly completely disables the profiler unless the BLACKFIRE_QUERY
// env variable is set. When the profiler is disabled, all API calls become no-ops.
//
// Only call this function before all other API calls. Calling it after another
// API call in this module will lead to undefined behavior.
func ProfileOnDemandOnly() {
	globalProbe.ProfileOnDemandOnly()
}
