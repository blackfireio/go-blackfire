package blackfire

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/go-ini/ini"
)

type BlackfireConfiguration struct {
	// True if this configuration has been validated and is ready for use.
	isValid bool
	// Errors encountered while validating
	validationErrors []error

	// Time before dropping an unresponsive agent connection (default 250ms)
	AgentTimeout time.Duration
	// The socket to use when connecting to the Blackfire agent (default depends on OS)
	AgentSocket string
	// The Blackfire query string to be sent with any profiles. This is either
	// provided by the `blackfire run` command in an ENV variable, or acquired
	// via a signing request to Blackfire. You won't need to set this manually.
	BlackfireQuery string
	// Client ID to authenticate with the Blackfire API
	ClientId string
	// Client token to authenticate with the Blackfire API
	ClientToken string
	// The Blackfire API endpoint the profile data will be sent to (default https://blackfire.io)
	HTTPEndpoint *url.URL
	// Path to the log file (default go-probe.log)
	LogFile string
	// Log verbosity 4: debug, 3: info, 2: warning, 1: error (default 1)
	LogLevel int
	// The maximum duration of a profile. A profile operation can never exceed
	// this duration (default 10 minutes).
	// This guards against runaway profile operations.
	MaxProfileDuration time.Duration
}

func (c *BlackfireConfiguration) setEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	c.HTTPEndpoint = u
	return nil
}

func (c *BlackfireConfiguration) getDefaultIniPath() string {
	getIniPath := func(dir string) string {
		if dir == "" {
			return ""
		}
		fileName := ".blackfire.ini"
		filePath := path.Join(path.Clean(dir), fileName)
		_, err := os.Stat(filePath)
		Log.Debug().Msgf("Blackfire: Does configuration file exist at %v: %v", filePath, err == nil)
		if err != nil {
			return ""
		}
		return filePath
	}

	if iniPath := getIniPath(os.Getenv("BLACKFIRE_HOME")); iniPath != "" {
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

func (c *BlackfireConfiguration) configureFromDefaults() {
	switch runtime.GOOS {
	case "windows":
		c.AgentSocket = "tcp://127.0.0.1:8307"
	case "darwin":
		c.AgentSocket = "unix:///usr/local/var/run/blackfire-agent.sock"
	case "linux":
		c.AgentSocket = "unix:///var/run/blackfire/agent.sock"
	case "freebsd":
		c.AgentSocket = "unix:///var/run/blackfire/agent.sock"
	}

	c.setEndpoint("https://blackfire.io")
	c.LogFile = "go-probe.log"
	c.LogLevel = 3
	c.AgentTimeout = time.Millisecond * 250
	c.MaxProfileDuration = time.Minute * 10

	setLogFile(c.LogFile)
	setLogLevel(c.LogLevel)
}

func readEnvVar(name string) string {
	if v := os.Getenv(name); v != "" {
		Log.Debug().Msgf("Blackfire: Read ENV var %v: %v", name, v)
		return v
	}
	return ""
}

func (c *BlackfireConfiguration) readLoggingFromEnv() {
	if v := readEnvVar("BLACKFIRE_LOG_LEVEL"); v != "" {
		level, err := strconv.Atoi(v)
		if err != nil {
			Log.Error().Msgf("Blackfire: Unable to set from env var BLACKFIRE_LOG_LEVEL %v: %v", v, err)
		} else {
			c.LogLevel = level
		}
	}

	if v := readEnvVar("BLACKFIRE_LOG_FILE"); v != "" {
		c.LogFile = v
	}
}

func (c *BlackfireConfiguration) configureLoggingFromEnv() {
	c.readLoggingFromEnv()

	setLogFile(c.LogFile)
	setLogLevel(c.LogLevel)
}

func (c *BlackfireConfiguration) configureFromEnv() {
	c.readLoggingFromEnv()

	if v := readEnvVar("BLACKFIRE_AGENT_SOCKET"); v != "" {
		c.AgentSocket = v
	}

	if v := readEnvVar("BLACKFIRE_QUERY"); v != "" {
		c.BlackfireQuery = v
	}

	if v := readEnvVar("BLACKFIRE_CLIENT_ID"); v != "" {
		c.ClientId = v
	}

	if v := readEnvVar("BLACKFIRE_CLIENT_TOKEN"); v != "" {
		c.ClientToken = v
	}

	if v := readEnvVar("BLACKFIRE_ENDPOINT"); v != "" {
		if err := c.setEndpoint(v); err != nil {
			Log.Error().Msgf("Blackfire: Unable to set from env var BLACKFIRE_ENDPOINT %v: %v", v, err)
		}
	}

	setLogFile(c.LogFile)
	setLogLevel(c.LogLevel)
}

func (c *BlackfireConfiguration) parseSeconds(value string) (time.Duration, error) {
	re := regexp.MustCompile(`([0-9.]+)`)
	found := re.FindStringSubmatch(value)

	if len(found) == 0 {
		return 0, fmt.Errorf("%v: No seconds value found", value)
	}

	seconds, err := strconv.ParseFloat(found[1], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(float64(time.Second) * seconds), nil
}

func getStringFromIniSection(section *ini.Section, key string) string {
	if v := section.Key(key).String(); v != "" {
		Log.Debug().Msgf("Blackfire: Read INI key %v: %v", key, v)
		return v
	}
	return ""
}

func (c *BlackfireConfiguration) configureFromIniFile(path string) {
	if path == "" {
		if path = c.getDefaultIniPath(); path == "" {
			return
		}
	}

	iniConfig, err := ini.Load(path)
	if err != nil {
		Log.Error().Msgf("Blackfire: Could not load Blackfire config file %v: %v", path, err)
		return
	}

	section := iniConfig.Section("blackfire")

	if section.HasKey("client-id") {
		c.ClientId = getStringFromIniSection(section, "client-id")
	}

	if section.HasKey("client-token") {
		c.ClientToken = getStringFromIniSection(section, "client-token")
	}

	if section.HasKey("agent_socket") {
		c.AgentSocket = getStringFromIniSection(section, "agent_socket")
	}

	if section.HasKey("endpoint") {
		endpoint := getStringFromIniSection(section, "endpoint")
		if err := c.setEndpoint(endpoint); err != nil {
			Log.Error().Msgf("Blackfire: Unable to set from ini file %v, endpoint %v: %v", path, endpoint, err)
		}
	}

	if section.HasKey("timeout") {
		timeout := getStringFromIniSection(section, "timeout")
		var err error
		if c.AgentTimeout, err = c.parseSeconds(timeout); err != nil {
			Log.Error().Msgf("Blackfire: Unable to set from ini file %v, timeout %v: %v", path, timeout, err)
		}
	}

	setLogFile(c.LogFile)
	setLogLevel(c.LogLevel)
}

func (c *BlackfireConfiguration) configureFromConfiguration(srcConfig *BlackfireConfiguration) {
	if srcConfig == nil {
		Log.Debug().Msgf("Blackfire: Manual config not provided")
		return
	}

	sv := reflect.ValueOf(srcConfig).Elem()
	dv := reflect.ValueOf(c).Elem()
	for i := 0; i < sv.NumField(); i++ {
		sField := sv.Field(i)
		dField := dv.Field(i)
		if !valueIsZero(sField) {
			Log.Debug().Msgf("Blackfire: Set %v manually to %v", sField.Type().Name(), sField)
			dField.Set(sField)
		}
	}

	setLogFile(c.LogFile)
	setLogLevel(c.LogLevel)
}

func (c *BlackfireConfiguration) validate() {
	errors := []error{}
	c.isValid = false

	if c.AgentTimeout <= 0 {
		errors = append(errors, fmt.Errorf("Agent timeout cannot be less than 1"))
	}

	if c.AgentSocket == "" {
		errors = append(errors, fmt.Errorf("Agent socket cannot be empty"))
	}

	if c.BlackfireQuery == "" {
		if c.ClientId == "" || c.ClientToken == "" {
			errors = append(errors, fmt.Errorf("Either Blackfire query must be set, or client ID and client token must be set"))
		}
	}

	if c.HTTPEndpoint == nil {
		errors = append(errors, fmt.Errorf("HTTP endpoint cannot be empty"))
	}

	if c.LogFile != "" && c.LogFile != "stdout" && c.LogFile != "stderr" {
		if _, err := os.Stat(c.LogFile); err != nil {
			errors = append(errors, fmt.Errorf("Log file %v not found", c.LogFile))
		}
	}

	if c.LogLevel < 1 || c.LogLevel > 4 {
		errors = append(errors, fmt.Errorf("Log level must be from 1 to 4"))
	}

	if c.MaxProfileDuration < 1 {
		errors = append(errors, fmt.Errorf("Max profile duration cannot be less than 1"))
	}

	c.validationErrors = errors
	if len(errors) == 0 {
		c.isValid = true
	}
}

func (c *BlackfireConfiguration) configure(manualConfig *BlackfireConfiguration, iniFilePath string) error {
	Log.Debug().Msgf("Blackfire: build configuration")

	c.configureFromDefaults()

	// This allows us to debug ini file loading issues.
	c.configureLoggingFromEnv()

	Log.Debug().Msgf("Blackfire: Read configuration from INI file %v", iniFilePath)
	c.configureFromIniFile(iniFilePath)
	Log.Debug().Msgf("Blackfire: Read configuration from ENV")
	c.configureFromEnv()

	Log.Debug().Msgf("Blackfire: Read configuration from manual settings")
	c.configureFromConfiguration(manualConfig)

	c.validate()

	if len(c.validationErrors) > 0 {
		return fmt.Errorf("blackfire.Configure() encountered errors: %v", c.validationErrors)
	}

	Log.Debug().Interface("configuration", c).Msg("Finished configuration")
	return nil
}
