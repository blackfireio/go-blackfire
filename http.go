package blackfire

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/blackfireio/go-blackfire/statik"
	"github.com/rakyll/statik/fs"
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
	mux.HandleFunc("/"+prefix+"/dashboard", DashboardHandler)
	mux.HandleFunc("/"+prefix+"/dashboard_api", DashboardApiHandler)
	mux.HandleFunc("/"+prefix+"/enable", EnableHandler)
	mux.HandleFunc("/"+prefix+"/disable", DisableHandler)
	mux.HandleFunc("/"+prefix+"/end", EndHandler)

	return
}

// DashboardHandler displays the current status of the profiler
func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	logger := globalProbe.configuration.Logger
	statikFS, err := fs.New()
	if err != nil {
		logger.Error().Msgf("Blackfire (HTTP): %s", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	f, err := statikFS.Open("/index.html")
	if err != nil {
		logger.Error().Msgf("Blackfire (HTTP): %s", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	defer f.Close()
	contents, err := ioutil.ReadAll(f)
	if err != nil {
		logger.Error().Msgf("Blackfire (HTTP): %s", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(contents)
}

func DashboardApiHandler(w http.ResponseWriter, r *http.Request) {
	writeJsonStatus(w)
}

// EnableHandler starts profiling via HTTP
func EnableHandler(w http.ResponseWriter, r *http.Request) {
	logger := globalProbe.configuration.Logger
	if title, found := parseString(r, "title"); found {
		globalProbe.SetCurrentTitle(title)
	}
	durationInSeconds, err := parseFloat(r, "duration")
	if err != nil {
		writeJsonError(w, &problem{Status: 400, Title: "Wrong duration", Detail: err.Error()})
		return
	}

	duration := time.Duration(durationInSeconds * float64(time.Second))
	if durationInSeconds > 0 {
		logger.Info().Msgf("Blackfire (HTTP): Profiling for %f seconds", float64(duration)/1000000000)
	} else {
		logger.Info().Msgf("Blackfire (HTTP): Enable profiling")
	}
	err = globalProbe.EnableNowFor(duration)
	if err != nil {
		writeJsonError(w, &problem{Status: 500, Title: "Enable error", Detail: err.Error()})
	} else {
		writeJsonStatus(w)
	}
}

// DisableHandler stops profiling via HTTP
func DisableHandler(w http.ResponseWriter, r *http.Request) {
	logger := globalProbe.configuration.Logger
	logger.Info().Msgf("Blackfire (HTTP): Disable profiling")
	if err := globalProbe.Disable(); err != nil {
		writeJsonError(w, &problem{Status: 500, Title: "Disable error", Detail: err.Error()})
	} else {
		writeJsonStatus(w)
	}
}

// EndHandler stops profiling via HTTP and send the profile to the agent
func EndHandler(w http.ResponseWriter, r *http.Request) {
	logger := globalProbe.configuration.Logger
	logger.Info().Msgf("Blackfire (HTTP): End profiling")
	if err := globalProbe.End(); err != nil {
		writeJsonError(w, &problem{Status: 500, Title: "End error", Detail: err.Error()})
	} else {
		writeJsonStatus(w)
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

func parseString(r *http.Request, paramName string) (value string, found bool) {
	value = ""
	if values, ok := r.URL.Query()[paramName]; ok {
		if len(values) > 0 {
			found = true
			value = values[0]
		}
	}
	return
}

func writeJsonError(w http.ResponseWriter, problem *problem) {
	logger := globalProbe.configuration.Logger
	logger.Error().Msgf("Blackfire (HTTP): %s: %s", problem.Title, problem.Detail)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	data, _ := json.Marshal(problem)
	w.Write(data)
}

func writeJsonStatus(w http.ResponseWriter) {
	profiling := "false"
	if globalProbe.currentState == profilerStateEnabled {
		profiling = "true"
	}
	profiles := []string{}
	if globalProbe.agentClient != nil {
		for _, profile := range globalProbe.agentClient.LastProfiles() {
			profiles = append(profiles, fmt.Sprintf(`{
	"UUID": "%s",
	"url": "%s",
	"name": "%s",
	"status": "%s",
	"created_at": "%s"
}`, profile.UUID, profile.URL, profile.Title, profile.Status.Name, profile.CreatedAt.Format(time.RFC3339)))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{
	"profiling": {
		"enabled": %s,
		"sample_rate": %d
	},
	"profiles": {
		"_embedded": [
			%s
		]
	}
}`, profiling, globalProbe.configuration.DefaultCPUSampleRateHz, strings.Join(profiles, ","))))
}
