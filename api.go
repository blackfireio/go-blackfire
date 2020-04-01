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
func Configure(config *Configuration) {
	globalProbe.Configure(config)
}

// IsProfiling checks if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	return globalProbe.IsProfiling()
}

// ProfileWithCallback profiles the current process for the specified duration.
// It also connects to the agent and upload the generated profile.
// and calls the callback in a goroutine (if not null).
func ProfileWithCallback(duration time.Duration, callback func()) bool {
	return globalProbe.ProfileWithCallback(duration, callback) == nil
}

// ProfileFor profiles the current process for the specified duration, then
// connects to the agent and uploads the generated profile.
func ProfileFor(duration time.Duration) bool {
	return globalProbe.ProfileFor(duration) == nil
}

// Enable starts profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func Enable() bool {
	return globalProbe.Enable() == nil
}

// Disable stops profiling.
func Disable() bool {
	return globalProbe.Disable() == nil
}

// End stops profiling, then uploads the result to the agent in a separate
// goroutine. You must ensure that the program does not exit before uploading
// is complete. If you can't make such a guarantee, use EndAndWait() instead.
func End() bool {
	return globalProbe.End() == nil
}

// EndAndWait ends the current profile, then blocks until the result is uploaded
// to the agent.
func EndAndWait() bool {
	return globalProbe.EndAndWait() == nil
}

// GenerateSubProfileQuery generates a Blackfire query
// to attach a subprofile with the current one as a parent
func GenerateSubProfileQuery() (string, error) {
	return globalProbe.GenerateSubProfileQuery()
}
