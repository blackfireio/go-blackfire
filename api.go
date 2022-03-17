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

// EnableNowFor profiles the current process for the specified duration, then
// connects to the agent and uploads the generated profile.
func EnableNowFor(duration time.Duration) Ender {
	globalProbe.EnableNowFor(duration)
	return globalProbe.ender
}

// EnableNow starts profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func EnableNow() Ender {
	globalProbe.EnableNow()
	return globalProbe.ender
}

// Enable() only profiles when trigerred from an external event (like using blackfire run).
func Enable() Ender {
	globalProbe.Enable()
	return globalProbe.ender
}

// Disable stops profiling.
func Disable() {
	globalProbe.Disable()
}

// End ends the current profile, then blocks until the result is uploaded
// to the agent.
func End() {
	globalProbe.End()
}

// EndNoWait stops profiling, then uploads the result to the agent in a separate
// goroutine. You must ensure that the program does not exit before uploading
// is complete. If you can't make such a guarantee, use End() instead.
func EndNoWait() {
	globalProbe.EndNoWait()
}

// GenerateSubProfileQuery generates a Blackfire query
// to attach a subprofile with the current one as a parent
func GenerateSubProfileQuery() (string, error) {
	return globalProbe.GenerateSubProfileQuery()
}

// SetCurrentTitle Sets the title to use for following profiles
func SetCurrentTitle(title string) {
	globalProbe.SetCurrentTitle(title)
}

// globalProbe is the access point for all probe functionality. The API, signal,
// and HTTP interfaces perform all operations by proxying to globalProbe. This
// ensures that mutexes and other guards are respected, and no interface can
// trigger functionality that others can't, or in a way that others can't.
var globalProbe = newProbe()
