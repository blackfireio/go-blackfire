package blackfire

import (
	"fmt"
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

func enable(w http.ResponseWriter, r *http.Request) {
	durationInSeconds, durationWasSpecified, err := parseFloat(r, "duration")

	if err != nil {
		blackfire.Log.Error().Msgf("Blackfire (HTTP): %v\n", err)
		w.WriteHeader(400)
		return
	}

	if durationWasSpecified {
		duration := time.Duration(durationInSeconds * float64(time.Second))
		blackfire.Log.Info().Msgf("Blackfire (HTTP): Profiling for %v seconds\n", float64(duration)/1000000000)
		if err := blackfire.ProfileWithCallback(duration, func() {
			blackfire.Log.Info().Msgf("Blackfire (HTTP): Profile complete\n")
		}); err != nil {
			blackfire.Log.Error().Msgf("Blackfire (HTTP) (enable): %v\n", err)
		}
	} else {
		blackfire.Log.Info().Msgf("Blackfire (HTTP): Enable profiling\n")
		if err := blackfire.Enable(); err != nil {
			blackfire.Log.Error().Msgf("Blackfire (HTTP) (enable): %v\n", err)
		}
	}
}

func disable(w http.ResponseWriter, r *http.Request) {
	blackfire.Log.Info().Msgf("Blackfire (HTTP): Disable profiling\n")
	blackfire.Disable()
}

func end(w http.ResponseWriter, r *http.Request) {
	blackfire.Log.Info().Msgf("Blackfire (HTTP): End profiling\n")
	blackfire.End()
}

// ----------
// Public API
// ----------

// Start the HTTP server on the specified host and port.
//
// The following HTTP paths will be available:
// - /enable : Run the profiler for either 30 seconds, or the value of the "duration" parameter (parsed as a float).
// - /disable : Stop the profiler (if running).
// - /end : End the current profile and upload to Blackfire
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

	blackfire.Log.Info().Msgf("Blackfire (HTTP): Listening on [%v]. Paths are /start and /stop\n", hostAndPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/enable", enable)
	mux.HandleFunc("/disable", disable)
	mux.HandleFunc("/end", end)

	server = new(http.Server)
	server.Addr = hostAndPort
	server.Handler = mux
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			blackfire.Log.Error().Msgf("Blackfire (StartServer): %v\n", err)
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
