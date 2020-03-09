package blackfire

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// NewServeMux returns an http.ServerMux that allows to manage profiling from HTTP
func NewServeMux(prefix string) *http.ServeMux {
	prefix = strings.Trim(prefix, "/")
	mux := http.NewServeMux()
	mux.HandleFunc("/"+prefix+"/enable", EnableHandler)
	mux.HandleFunc("/"+prefix+"/disable", DisableHandler)
	mux.HandleFunc("/"+prefix+"/end", EndHandler)

	return mux
}

// EnableHandler starts profiling via HTTP
func EnableHandler(w http.ResponseWriter, r *http.Request) {
	durationInSeconds, durationWasSpecified, err := parseFloat(r, "duration")

	if err != nil {
		Log.Error().Msgf("Blackfire (HTTP): %v\n", err)
		w.WriteHeader(400)
		return
	}

	if durationWasSpecified {
		duration := time.Duration(durationInSeconds * float64(time.Second))
		Log.Info().Msgf("Blackfire (HTTP): Profiling for %v seconds\n", float64(duration)/1000000000)
		if err := globalProbe.ProfileWithCallback(duration, func() {
			Log.Info().Msgf("Blackfire (HTTP): Profile complete\n")
		}); err != nil {
			Log.Error().Msgf("Blackfire (HTTP) (enable): %v\n", err)
		}
	} else {
		Log.Info().Msgf("Blackfire (HTTP): Enable profiling\n")
		if err := globalProbe.Enable(); err != nil {
			Log.Error().Msgf("Blackfire (HTTP) (enable): %v\n", err)
		}
	}
}

// DisableHandler stops profiling via HTTP
func DisableHandler(w http.ResponseWriter, r *http.Request) {
	Log.Info().Msgf("Blackfire (HTTP): Disable profiling\n")
	globalProbe.Disable()
}

// EndHandler stops profiling via HTTP and send the profile to the agent
func EndHandler(w http.ResponseWriter, r *http.Request) {
	Log.Info().Msgf("Blackfire (HTTP): End profiling\n")
	globalProbe.End()
}

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
