package blackfire

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/blackfireio/go-blackfire"
)

var defaultHostAndPort string = ":6020"
var httpMutex sync.Mutex
var server *http.Server

func parseFloat(r *http.Request, paramName string) (value float64, isFound bool, err error) {
	if values, ok := r.URL.Query()[paramName]; ok {
		if len(values) > 0 {
			if value, err = strconv.ParseFloat(values[0], 64); err == nil {
				isFound = true
			}
		}
	}
	return
}

func startProfiler(w http.ResponseWriter, r *http.Request) {
	durationInSeconds, durationWasSpecified, err := parseFloat(r, "duration")

	if err != nil {
		log.Printf("Blackfire (HTTP): Error: %v\n", err)
		w.WriteHeader(400)
		return
	}

	if durationWasSpecified {
		duration := time.Duration(durationInSeconds * float64(time.Second))
		log.Printf("Blackfire (HTTP): Profiling for %v seconds\n", float64(duration)/1000000000)
		if err := blackfire.ProfileWithCallback(duration, func() {
			log.Printf("Blackfire (HTTP): Profile complete\n")
		}); err != nil {
			log.Printf("Blackfire Error (startProfiler): %v\n", err)
		}
	} else {
		log.Printf("Blackfire (HTTP): Start profiling\n")
		if err := blackfire.StartProfiling(); err != nil {
			log.Printf("Blackfire Error (startProfiler): %v\n", err)
		}
	}
}

func stopProfiler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Blackfire (HTTP): Stop profiling\n")
	if err := blackfire.StopProfiling(); err != nil {
		log.Printf("Blackfire Error (stopProfiler): %v\n", err)
	}
}

// ----------
// Public API
// ----------

// Start the HTTP server on the specified host and port.
//
// The following HTTP paths will be available:
// - /start : Run the profiler for either 30 seconds, or the value of the "duration" parameter (parsed as a float).
// - /stop : Stop the profiler (if running).
//
// Supplying a hostAndPort value of "" will choose the default of ":6020"
func StartServer(hostAndPort string) error {
	httpMutex.Lock()
	defer httpMutex.Unlock()

	if err := blackfire.AssertCanProfile(); err != nil {
		return err
	}

	if server != nil {
		return fmt.Errorf("Already serving HTTP")
	}

	if hostAndPort == "" {
		hostAndPort = defaultHostAndPort
	}

	log.Printf("Blackfire (HTTP): Listening on [%v]. Paths are /start and /stop\n", hostAndPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/start", startProfiler)
	mux.HandleFunc("/stop", stopProfiler)

	server = new(http.Server)
	server.Addr = hostAndPort
	server.Handler = mux
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Blackfire Error (StartServer): %v\n", err)
		}
	}()

	return nil
}

func StopServer() error {
	httpMutex.Lock()
	defer httpMutex.Unlock()

	if server == nil {
		return nil
	}

	serverRef := server
	server = nil

	return serverRef.Close()
}
