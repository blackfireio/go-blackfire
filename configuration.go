package blackfire

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-ini/ini"
)

// This must match the value of `hz` in StartCPUProfile in runtime/pprof/pprof.go
// It's always been 100hz since the beginning, so it should be safe.
const golangDefaultCPUSampleRate = 100

type Configuration struct {
	// The configuration path to the Blackfire CLI ini file
	// Defaults to ~/.blackfire.ini
	ConfigFile string

	// Disables the profiler unless the BLACKFIRE_QUERY env variable is set.
	// When the profiler is disabled, all API calls become no-ops.
	OnDemandOnly bool

	// Time before dropping an unresponsive agent connection (default 250ms)
	AgentTimeout time.Duration
	// The socket to use when connecting to the Blackfire agent (default depends on OS)
	AgentSocket string
	// The Blackfire query string to be sent with any profiles. This is either
	// provided by the `blackfire run` command in an ENV variable, or acquired
	// via a signing request to Blackfire. You won't need to set this manually.
	BlackfireQuery string
	// Client ID to authenticate with the Blackfire API
	ClientID string
	// Client token to authenticate with the Blackfire API
	ClientToken string
	// The Blackfire API endpoint the profile data will be sent to (default https://blackfire.io)
	HTTPEndpoint *url.URL
	// Path to the log file, supports stderr and stdout as well (default stderr)
	LogFile string
	// Log verbosity 4: debug, 3: info, 2: warning, 1: error (default 1)
	LogLevel int
	// The maximum duration of a profile. A profile operation can never exceed
	// this duration (default 10 minutes).
	// This guards against runaway profile operations.
	MaxProfileDuration time.Duration
	// Default rate at which the CPU samples are taken. Values > 500 will likely
	// exceed the abilities of most environments.
	// See https://golang.org/src/runtime/pprof/pprof.go#L727
	DefaultCPUSampleRateHz int
	// If true, dump the original pprof profiles to the current directory whenever
	// a profile ends.
	ShouldDumpProfiles bool

	loader sync.Once
}

func (c *Configuration) canProfile() bool {
	if c.BlackfireQuery == "" && c.OnDemandOnly {
		return false
	}
	return true
}

func (c *Configuration) setEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	c.HTTPEndpoint = u
	return nil
}

func (c *Configuration) getDefaultIniPath() string {
	getIniPath := func(dir string) string {
		if dir == "" {
			return ""
		}
		fileName := ".blackfire.ini"
		filePath := path.Join(path.Clean(dir), fileName)
		_, err := os.Stat(filePath)
		Log.Debug().Msgf("Blackfire: Does configuration file exist at %s: %t", filePath, err == nil)
		if err != nil {
			return ""
		}
		return filePath
	}

	if iniPath := getIniPath(readEnvVar("BLACKFIRE_HOME")); iniPath != "" {
		return iniPath
	}

	if runtime.GOOS == "linux" {
		if iniPath := getIniPath(os.Getenv("XDG_CONFIG_HOME")); iniPath != "" {
			return iniPath
		}
	}

	if iniPath := getIniPath(os.Getenv("HOME")); iniPath != "" {
		return iniPath
	}

	if runtime.GOOS == "windows" {
		homedrive := os.Getenv("HOMEDRIVE")
		homepath := os.Getenv("HOMEPATH")
		if homedrive != "" && homepath != "" {
			dir := path.Join(path.Dir(homedrive), homepath)
			if iniPath := getIniPath(dir); iniPath != "" {
				return iniPath
			}
		}
	}

	return ""
}

func (c *Configuration) configureLogging() error {
	if v := readEnvVar("BLACKFIRE_LOG_LEVEL"); v != "" {
		level, err := strconv.Atoi(v)
		if err != nil {
			level = 1
		}
		c.LogLevel = level
	}
	if c.LogLevel == 0 {
		c.LogLevel = 1
	}
	setLogLevel(c.LogLevel)

	if v := readEnvVar("BLACKFIRE_LOG_FILE"); v != "" {
		c.LogFile = v
	}
	if c.LogFile == "" {
		c.LogFile = "stderr"
	}
	return setLogFile(c.LogFile)
}

func (c *Configuration) configureFromDefaults() {
	if c.AgentSocket == "" {
		switch runtime.GOOS {
		case "windows":
			c.AgentSocket = "tcp://127.0.0.1:8307"
		case "darwin":
			c.AgentSocket = "unix:///usr/local/var/run/blackfire-agent.sock"
		default:
			c.AgentSocket = "unix:///var/run/blackfire/agent.sock"
		}
	}

	if c.HTTPEndpoint == nil {
		c.setEndpoint("https://blackfire.io")
	}
	if c.AgentTimeout == 0 {
		c.AgentTimeout = time.Millisecond * 250
	}
	if c.MaxProfileDuration == 0 {
		c.MaxProfileDuration = time.Minute * 10
	}
	if c.DefaultCPUSampleRateHz == 0 {
		c.DefaultCPUSampleRateHz = golangDefaultCPUSampleRate
	}
}

func (c *Configuration) configureFromIniFile() {
	path := c.ConfigFile
	if path == "" {
		if path = c.getDefaultIniPath(); path == "" {
			return
		}
	}

	iniConfig, err := ini.Load(path)
	if err != nil {
		Log.Error().Msgf("Blackfire: Could not load Blackfire config file %s: %v", path, err)
		return
	}

	section := iniConfig.Section("blackfire")
	if section.HasKey("client-id") && c.ClientID == "" {
		c.ClientID = getStringFromIniSection(section, "client-id")
	}

	if section.HasKey("client-token") && c.ClientToken == "" {
		c.ClientToken = getStringFromIniSection(section, "client-token")
	}

	if section.HasKey("endpoint") && c.HTTPEndpoint == nil {
		endpoint := getStringFromIniSection(section, "endpoint")
		if err := c.setEndpoint(endpoint); err != nil {
			Log.Error().Msgf("Blackfire: Unable to set from ini file %s, endpoint %s: %v", path, endpoint, err)
		}
	}

	if section.HasKey("timeout") && c.AgentTimeout == 0 {
		timeout := getStringFromIniSection(section, "timeout")
		var err error
		if c.AgentTimeout, err = parseSeconds(timeout); err != nil {
			Log.Error().Msgf("Blackfire: Unable to set from ini file %s, timeout %s: %v", path, timeout, err)
		}
	}
}

func (c *Configuration) configureFromEnv() {
	if v := readEnvVar("BLACKFIRE_AGENT_SOCKET"); v != "" {
		c.AgentSocket = v
	}

	if v := readEnvVar("BLACKFIRE_QUERY"); v != "" {
		c.BlackfireQuery = v
		os.Unsetenv("BLACKFIRE_QUERY")
	}

	if v := readEnvVar("BLACKFIRE_CLIENT_ID"); v != "" {
		c.ClientID = v
	}

	if v := readEnvVar("BLACKFIRE_CLIENT_TOKEN"); v != "" {
		c.ClientToken = v
	}

	if v := readEnvVar("BLACKFIRE_ENDPOINT"); v != "" {
		if err := c.setEndpoint(v); err != nil {
			Log.Error().Msgf("Blackfire: Unable to set from env var BLACKFIRE_ENDPOINT %s: %v", v, err)
		}
	}

	if v := readEnvVar("BLACKFIRE_DUMP_PPROF"); v == "true" {
		c.ShouldDumpProfiles = true
	}
}

func (c *Configuration) load() (err error) {
	errs := []error{}
	c.loader.Do(func() {
		if err := c.configureLogging(); err != nil {
			errs = append(errs, err)
		}
		c.configureFromEnv()
		c.configureFromIniFile()
		c.configureFromDefaults()
		errs = append(errs, c.validate()...)
	})
	if len(errs) > 0 {
		return fmt.Errorf("Blackfire: Invalid configuration: %v", errs)
	}
	return nil
}

func (c *Configuration) validate() []error {
	errors := []error{}

	if c.AgentTimeout <= 0 {
		errors = append(errors, fmt.Errorf("Agent timeout cannot be less than 1"))
	}

	if c.AgentSocket == "" {
		errors = append(errors, fmt.Errorf("Agent socket cannot be empty"))
	}

	if c.BlackfireQuery == "" {
		if c.ClientID == "" || c.ClientToken == "" {
			errors = append(errors, fmt.Errorf("Either Blackfire query must be set, or client ID and client token must be set"))
		}
	}

	if c.HTTPEndpoint == nil {
		errors = append(errors, fmt.Errorf("HTTP endpoint cannot be empty"))
	}

	if c.MaxProfileDuration < 1 {
		errors = append(errors, fmt.Errorf("Max profile duration cannot be less than 1"))
	}

	return errors
}

func readEnvVar(name string) string {
	if v := os.Getenv(name); v != "" {
		Log.Debug().Msgf("Blackfire: Read ENV var %s: %s", name, v)
		return v
	}
	return ""
}

func getStringFromIniSection(section *ini.Section, key string) string {
	if v := section.Key(key).String(); v != "" {
		Log.Debug().Msgf("Blackfire: Read INI key %s: %s", key, v)
		return v
	}
	return ""
}

func parseSeconds(value string) (time.Duration, error) {
	re := regexp.MustCompile(`([0-9.]+)`)
	found := re.FindStringSubmatch(value)

	if len(found) == 0 {
		return 0, fmt.Errorf("%s: No seconds value found", value)
	}

	seconds, err := strconv.ParseFloat(found[1], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(float64(time.Second) * seconds), nil
}
