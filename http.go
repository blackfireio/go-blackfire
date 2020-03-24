package blackfire

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// NewServeMux returns an http.ServerMux that allows to manage profiling from HTTP
func NewServeMux(prefix string) (mux *http.ServeMux, err error) {
	if err = globalProbe.configuration.load(); err != nil {
		return
	}
	prefix = strings.Trim(prefix, "/")
	mux = http.NewServeMux()
	mux.HandleFunc("/"+prefix+"/enable", EnableHandler)
	mux.HandleFunc("/"+prefix+"/disable", DisableHandler)
	mux.HandleFunc("/"+prefix+"/end", EndHandler)

	return
}

// EnableHandler starts profiling via HTTP
func EnableHandler(w http.ResponseWriter, r *http.Request) {
	durationInSeconds, durationWasSpecified, err := parseFloat(r, "duration")

	if err != nil {
		Log.Error().Msgf("Blackfire (HTTP): %v", err)
		w.WriteHeader(400)
		return
	}

	if durationWasSpecified {
		duration := time.Duration(durationInSeconds * float64(time.Second))
		Log.Info().Msgf("Blackfire (HTTP): Profiling for %f seconds", float64(duration)/1000000000)
		if err := globalProbe.ProfileWithCallback(duration, func() {
			Log.Info().Msgf("Blackfire (HTTP): Profile complete")
		}); err != nil {
			Log.Error().Msgf("Blackfire (HTTP) (enable): %v", err)
		}
	} else {
		Log.Info().Msgf("Blackfire (HTTP): Enable profiling")
		if err := globalProbe.Enable(); err != nil {
			Log.Error().Msgf("Blackfire (HTTP) (enable): %v", err)
		}
	}
}

// DisableHandler stops profiling via HTTP
func DisableHandler(w http.ResponseWriter, r *http.Request) {
	Log.Info().Msgf("Blackfire (HTTP): Disable profiling")
	globalProbe.Disable()
}

// EndHandler stops profiling via HTTP and send the profile to the agent
func EndHandler(w http.ResponseWriter, r *http.Request) {
	Log.Info().Msgf("Blackfire (HTTP): End profiling")
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
