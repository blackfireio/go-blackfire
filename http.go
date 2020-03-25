package blackfire

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type problem struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

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
	durationInSeconds, err := parseFloat(r, "duration")
	if err != nil {
		writeJsonError(w, &problem{Status: 400, Title: "Wrong duration", Detail: err.Error()})
		return
	}

	duration := time.Duration(durationInSeconds * float64(time.Second))
	if durationInSeconds > 0 {
		Log.Info().Msgf("Blackfire (HTTP): Profiling for %f seconds", float64(duration)/1000000000)
	} else {
		Log.Info().Msgf("Blackfire (HTTP): Enable profiling")
	}
	err = globalProbe.ProfileWithCallback(duration, func() {
		Log.Info().Msgf("Blackfire (HTTP): Profile complete")
	})
	if err != nil {
		writeJsonError(w, &problem{Status: 500, Title: "Enable error", Detail: err.Error()})
	} else {
		writeJsonSuccess(w)
	}
}

// DisableHandler stops profiling via HTTP
func DisableHandler(w http.ResponseWriter, r *http.Request) {
	Log.Info().Msgf("Blackfire (HTTP): Disable profiling")
	if err := globalProbe.Disable(); err != nil {
		writeJsonError(w, &problem{Status: 500, Title: "Disable error", Detail: err.Error()})
	} else {
		writeJsonSuccess(w)
	}
}

// EndHandler stops profiling via HTTP and send the profile to the agent
func EndHandler(w http.ResponseWriter, r *http.Request) {
	Log.Info().Msgf("Blackfire (HTTP): End profiling")
	if err := globalProbe.EndAndWait(); err != nil {
		writeJsonError(w, &problem{Status: 500, Title: "End error", Detail: err.Error()})
	} else {
		writeJsonSuccess(w)
	}
}

func parseFloat(r *http.Request, paramName string) (value float64, err error) {
	value = 0
	if values, ok := r.URL.Query()[paramName]; ok {
		if len(values) > 0 {
			value, err = strconv.ParseFloat(values[0], 64)
		}
	}
	return
}

func writeJsonError(w http.ResponseWriter, problem *problem) {
	Log.Error().Msgf("Blackfire (HTTP): %s: %s", problem.Title, problem.Detail)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	data, _ := json.Marshal(problem)
	w.Write(data)
}

func writeJsonSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}
