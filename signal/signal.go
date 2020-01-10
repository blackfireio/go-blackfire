package signal

import (
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/blackfireio/go-blackfire"
)

func callOnSignal(sig os.Signal, function func()) {
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

// Set up a trigger to start profiling when the specified signal is received.
// The profiler will profile for the specified duration and then upload the
// result to the agent.
func StartOnSignal(sig os.Signal, duration time.Duration) (err error) {
	if err = blackfire.AssertCanProfile(); err != nil {
		return
	}

	log.Printf("Blackfire (signal): Signal [%v] triggers profiling for %v seconds\n", sig, float64(duration)/1000000000)

	callOnSignal(sig, func() {
		log.Printf("Blackfire (%v): Profiling for %v seconds\n", sig, float64(duration)/1000000000)
		if err := blackfire.ProfileWithCallback(duration, func() {
			log.Printf("Blackfire (%v): Profile complete\n", sig)
		}); err != nil {
			log.Printf("Blackfire Error (profileFor): %v\n", err)
		}
	})

	return
}

// Set up a trigger to stop profiling when the specified signal is received.
func StopOnSignal(sig os.Signal) (err error) {
	if err = blackfire.AssertCanProfile(); err != nil {
		return
	}

	log.Printf("Blackfire (signal): Signal [%v] stops profiling\n", sig)

	callOnSignal(sig, func() {
		log.Printf("Blackfire (%v): Stop profiling\n", sig)
		if err := blackfire.StopProfiling(); err != nil {
			log.Printf("Blackfire Error (StopOnSignal): %v\n", err)
		}
	})
	return
}
