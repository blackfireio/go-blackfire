package blackfire

import (
	"os"
	"os/signal"
	"time"
)

// EnableOnSignal sets up a trigger to enable profiling when the specified signal is received.
// The profiler will profile for the specified duration.
func EnableOnSignal(sig os.Signal, duration time.Duration) (err error) {
	if err = globalProbe.configuration.load(); err != nil {
		return
	}
	if !globalProbe.configuration.canProfile() {
		return
	}

	logger := globalProbe.configuration.Logger
	logger.Info().Msgf("Blackfire (signal): Signal [%s] triggers profiling for %.0f seconds", sig, float64(duration)/1000000000)

	callFuncOnSignal(sig, func() {
		logger.Info().Msgf("Blackfire (%s): Profiling for %.0f seconds", sig, float64(duration)/1000000000)
		if err := globalProbe.EnableNowFor(duration); err != nil {
			logger.Error().Msgf("Blackfire (EnableOnSignal): %v", err)
		}
	})

	return
}

// DisableOnSignal sets up a trigger to disable profiling when the specified signal is received.
func DisableOnSignal(sig os.Signal) (err error) {
	if err = globalProbe.configuration.load(); err != nil {
		return
	}
	if !globalProbe.configuration.canProfile() {
		return
	}

	logger := globalProbe.configuration.Logger
	logger.Info().Msgf("Blackfire (signal): Signal [%s] stops profiling", sig)

	callFuncOnSignal(sig, func() {
		logger.Info().Msgf("Blackfire (%s): Disable profiling", sig)
		if err := globalProbe.Disable(); err != nil {
			logger.Error().Msgf("Blackfire (DisableOnSignal): %v", err)
		}
	})
	return
}

// EndOnSignal sets up a trigger to end the current profile and upload to Blackfire when the
// specified signal is received.
func EndOnSignal(sig os.Signal) (err error) {
	if err = globalProbe.configuration.load(); err != nil {
		return
	}
	if !globalProbe.configuration.canProfile() {
		return
	}

	logger := globalProbe.configuration.Logger
	logger.Info().Msgf("Blackfire (signal): Signal [%s] ends the current profile", sig)

	callFuncOnSignal(sig, func() {
		logger.Info().Msgf("Blackfire (%s): End profile", sig)
		if err := globalProbe.EndNoWait(); err != nil {
			logger.Error().Msgf("Blackfire (EndOnSignal): %v", err)
		}
	})
	return
}

func callFuncOnSignal(sig os.Signal, function func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, sig)
	go func() {
		for {
			<-sigs
			function()
		}
	}()
}
