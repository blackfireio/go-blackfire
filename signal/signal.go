package signal

import (
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/blackfireio/go-blackfire"
)

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

// ----------
// Public API
// ----------

// Set up a trigger to enable profiling when the specified signal is received.
// The profiler will profile for the specified duration.
func EnableOnSignal(sig os.Signal, duration time.Duration) (err error) {
	if err = blackfire.AssertCanProfile(); err != nil {
		return
	}

	log.Printf("Blackfire (signal): Signal [%v] triggers profiling for %v seconds\n", sig, float64(duration)/1000000000)

	callFuncOnSignal(sig, func() {
		log.Printf("Blackfire (%v): Profiling for %v seconds\n", sig, float64(duration)/1000000000)
		if err := blackfire.ProfileWithCallback(duration, func() {
			log.Printf("Blackfire (%v): Profile complete\n", sig)
		}); err != nil {
			log.Printf("Blackfire Error (EnableOnSignal): %v\n", err)
		}
	})

	return
}

// Set up a trigger to disable profiling when the specified signal is received.
func DisableOnSignal(sig os.Signal) (err error) {
	if err = blackfire.AssertCanProfile(); err != nil {
		return
	}

	log.Printf("Blackfire (signal): Signal [%v] stops profiling\n", sig)

	callFuncOnSignal(sig, func() {
		log.Printf("Blackfire (%v): Disable profiling\n", sig)
		blackfire.Disable()
	})
	return
}

// Set up a trigger to end the current profile and upload to Blackfire when the
// specified signal is received.
func EndOnSignal(sig os.Signal) (err error) {
	if err = blackfire.AssertCanProfile(); err != nil {
		return
	}

	log.Printf("Blackfire (signal): Signal [%v] ends the current profile\n", sig)

	callFuncOnSignal(sig, func() {
		log.Printf("Blackfire (%v): End profile\n", sig)
		blackfire.End()
	})
	return
}
