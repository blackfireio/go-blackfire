package blackfire

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/blackfireio/go-blackfire"
)

var defaultHostAndPort string = ":6020"
var defaultDuration time.Duration = time.Second * 10

var isServing *uint32 = new(uint32)

var server *http.Server

func acquireServingLock() bool {
	return atomic.CompareAndSwapUint32(isServing, 0, 1)
}

func releaseServingLock() {
	atomic.StoreUint32(isServing, 0)
}

func runProfiler(w http.ResponseWriter, r *http.Request) {
	profileDuration := defaultDuration
	if values, ok := r.URL.Query()["duration"]; ok {
		if len(values) > 0 {
			if value, err := strconv.ParseFloat(values[0], 64); err == nil {
				profileDuration = time.Duration(value * float64(time.Second))
			}
		}
	}

	go func() {
		log.Printf("Blackfire: Triggered by http. Profiling for %v seconds\n", float64(profileDuration)/1000000000)
		if err := blackfire.ProfileFor(profileDuration); err != nil {
			log.Printf("Blackfire: Error profiling: %v\n", err)
		} else {
			log.Printf("Blackfire: Profiling complete\n")
		}
	}()
}

func StartServer(hostAndPort string) error {
	if !acquireServingLock() {
		return fmt.Errorf("Already serving HTTP")
	}

	if server != nil {
		return fmt.Errorf("Already serving HTTP")
	}

	if hostAndPort == "" {
		hostAndPort = defaultHostAndPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/profile", runProfiler)

	server = new(http.Server)
	server.Addr = hostAndPort
	server.Handler = mux
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	releaseServingLock()
	return nil
}

func StopServer() (err error) {
	if !acquireServingLock() {
		return
	}

	serverRef := server
	server = nil
	if serverRef != nil {
		err = serverRef.Close()
	}

	releaseServingLock()
	return
}
