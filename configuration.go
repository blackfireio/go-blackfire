package blackfire

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-ini/ini"
	"github.com/rs/zerolog"
)

// This must match the value of `hz` in StartCPUProfile in runtime/pprof/pprof.go
// It's always been 100hz since the beginning, so it should be safe.
const golangDefaultCPUSampleRate = 100

type Configuration struct {
	// The configuration path to the Blackfire CLI ini file
	// Defaults to ~/.blackfire.ini
	ConfigFile string

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

	// Server ID for Blackfire-Auth header
	ServerID string

	// Server token for Blackfire-Auth header
	ServerToken string

	// The Blackfire API endpoint the profile data will be sent to (default https://blackfire.io)
	HTTPEndpoint *url.URL

	// A zerolog Logger (default stderr)
	Logger *zerolog.Logger

	// The maximum duration of a profile. A profile operation can never exceed
	// this duration (default 10 minutes).
	// This guards against runaway profile operations.
	MaxProfileDuration time.Duration

	// Default rate at which the CPU samples are taken. Values > 500 will likely
	// exceed the abilities of most environments.
	// See https://golang.org/src/runtime/pprof/pprof.go#L727
	DefaultCPUSampleRateHz int

	// If not empty, dump the original pprof profiles to this directory whenever
	// a profile ends.
	PProfDumpDir string

	// Disables the profiler unless the BLACKFIRE_QUERY env variable is set.
	// When the profiler is disabled, all API calls become no-ops.
	onDemandOnly bool

	loader sync.Once
	err    error
}

func (c *Configuration) canProfile() bool {
	if c.BlackfireQuery == "" && c.onDemandOnly {
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
		c.Logger.Debug().Msgf("Blackfire: Does configuration file exist at %s: %t", filePath, err == nil)
		if err != nil {
			return ""
		}
		return filePath
	}

	if iniPath := getIniPath(c.readEnvVar("BLACKFIRE_HOME")); iniPath != "" {
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

func (c *Configuration) configureFromDefaults() {
	if c.AgentSocket == "" {
		switch runtime.GOOS {
		case "windows":
			c.AgentSocket = "tcp://127.0.0.1:8307"
		case "darwin":
			if runtime.GOARCH == "arm64" {
				c.AgentSocket = "unix:///opt/homebrew/var/run/blackfire-agent.sock"
			} else {
				c.AgentSocket = "unix:///usr/local/var/run/blackfire-agent.sock"
			}
		default:
			c.AgentSocket = "unix:///var/run/blackfire/agent.sock"
		}
	}

	if c.HTTPEndpoint == nil {
		c.setEndpoint("https://blackfire.io")
	}
	if c.AgentTimeout < 1 {
		c.AgentTimeout = time.Millisecond * 250
	}
	if c.MaxProfileDuration < 1 {
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
		c.Logger.Error().Msgf("Blackfire: Could not load Blackfire config file %s: %v", path, err)
		return
	}

	section := iniConfig.Section("blackfire")
	if section.HasKey("client-id") && c.ClientID == "" {
		c.ClientID = c.getStringFromIniSection(section, "client-id")
	}

	if section.HasKey("client-token") && c.ClientToken == "" {
		c.ClientToken = c.getStringFromIniSection(section, "client-token")
	}

	if section.HasKey("endpoint") && c.HTTPEndpoint == nil {
		endpoint := c.getStringFromIniSection(section, "endpoint")
		if err := c.setEndpoint(endpoint); err != nil {
			c.Logger.Error().Msgf("Blackfire: Unable to set from ini file %s, endpoint %s: %v", path, endpoint, err)
		}
	}

	if section.HasKey("timeout") && c.AgentTimeout == 0 {
		timeout := c.getStringFromIniSection(section, "timeout")
		var err error
		if c.AgentTimeout, err = parseSeconds(timeout); err != nil {
			c.Logger.Error().Msgf("Blackfire: Unable to set from ini file %s, timeout %s: %v", path, timeout, err)
		}
	}
}

func (c *Configuration) configureFromEnv() {
	if v := c.readEnvVar("BLACKFIRE_AGENT_SOCKET"); v != "" {
		c.AgentSocket = v
	}

	if v := c.readEnvVar("BLACKFIRE_QUERY"); v != "" {
		c.BlackfireQuery = v
		os.Unsetenv("BLACKFIRE_QUERY")
	}

	if v := c.readEnvVar("BLACKFIRE_CLIENT_ID"); v != "" {
		c.ClientID = v
	}

	if v := c.readEnvVar("BLACKFIRE_CLIENT_TOKEN"); v != "" {
		c.ClientToken = v
	}

	if v := c.readEnvVar("BLACKFIRE_SERVER_ID"); v != "" {
		c.ServerID = v
	}

	if v := c.readEnvVar("BLACKFIRE_SERVER_TOKEN"); v != "" {
		c.ServerToken = v
	}

	if v := c.readEnvVar("BLACKFIRE_ENDPOINT"); v != "" {
		if err := c.setEndpoint(v); err != nil {
			c.Logger.Error().Msgf("Blackfire: Unable to set from env var BLACKFIRE_ENDPOINT %s: %v", v, err)
		}
	}

	if v := c.readEnvVar("BLACKFIRE_PPROF_DUMP_DIR"); v != "" {
		absPath, err := filepath.Abs(v)
		if err != nil {
			c.Logger.Error().Msgf("Blackfire: Unable to set pprof dump dir to %v: %v", v, err)
		} else {
			c.PProfDumpDir = absPath
		}
	}
}

func (c *Configuration) load() error {
	c.loader.Do(func() {
		if c.Logger == nil {
			logger := NewLoggerFromEnvVars()
			c.Logger = &logger
		}
		c.configureFromEnv()
		// Used for test purposes
		if "1" != os.Getenv("BLACKFIRE_INTERNAL_IGNORE_INI") {
			c.configureFromIniFile()
		}
		c.configureFromDefaults()
		if c.err = c.validate(); c.err != nil {
			c.Logger.Warn().Err(c.err).Msg("Blackfire: Bad configuration")
		}
	})
	return c.err
}

func (c *Configuration) validate() error {
	if c.BlackfireQuery == "" {
		if c.ClientID == "" || c.ClientToken == "" {
			return errors.New("either BLACKFIRE_QUERY must be set, or client ID and client token must be set")
		}
	}

	if c.PProfDumpDir != "" {
		info, err := os.Stat(c.PProfDumpDir)
		if err != nil {
			return fmt.Errorf("Cannot dump pprof files to %v: %v", c.PProfDumpDir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("Cannot dump pprof files to %v: not a directory", c.PProfDumpDir)
		}

		// There's no 100% portable way to check for writability, so we just create
		// a temp zero-byte file and see if it succeeds.
		exePath, err := os.Executable()
		if err != nil {
			exePath = "go-unknown"
		} else {
			exePath = path.Base(exePath)
		}
		testPath := path.Join(c.PProfDumpDir, exePath+"-writability-test")
		// Delete it before starting, and make sure it gets deleted after
		os.Remove(testPath)
		defer os.Remove(testPath)
		if err = ioutil.WriteFile(testPath, []byte{}, 0644); err != nil {
			return fmt.Errorf("Cannot dump pprof files to %v: directory does not seem writable: %v", c.PProfDumpDir, err)
		}
	}
	return nil
}

func (c *Configuration) readEnvVar(name string) string {
	if v := os.Getenv(name); v != "" {
		c.Logger.Debug().Msgf("Blackfire: Read ENV var %s: %s", name, v)
		return v
	}
	return ""
}

func (c *Configuration) getStringFromIniSection(section *ini.Section, key string) string {
	if v := section.Key(key).String(); v != "" {
		c.Logger.Debug().Msgf("Blackfire: Read INI key %s: %s", key, v)
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
