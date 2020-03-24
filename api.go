package blackfire

import (
	"errors"
	"time"
)

var ProfilerErrorAlreadyProfiling = errors.New("A Blackfire profile is currently in progress. Please wait for it to finish.")

// Configure explicitely configures the probe. This should be done before any other API calls.
//
// Configuration is initialized in a set order, with later steps overriding
// earlier steps:
//
// * Defaults
// * INI file
// * Explicit configuration in Go code
// * Environment variables
//
// config will be ignored if nil.
func Configure(config *BlackfireConfiguration) error {
	return globalProbe.Configure(config)
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

// GenerateSubProfileQuery generates a Blackfire query
// to attach a subprofile with the current one as a parent
func GenerateSubProfileQuery() (string, error) {
	return globalProbe.GenerateSubProfileQuery()
}
