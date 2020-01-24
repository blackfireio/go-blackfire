package blackfire

import (
	"log"
	"math"
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
	// Time before dropping an unresponsive agent connection (default 250ms)
	AgentTimeout time.Duration
	// The socket to use when connecting to the Blackfire agent (default depends on OS)
	AgentSocket    string
	BlackfireQuery string
	// Client ID to authenticate with the Blackfire API
	ClientId string
	// Client token to authenticate with the Blackfire API
	ClientToken string
	// The Blackfire API endpoint the profile data will be sent to (default https://blackfire.io/)
	Endpoint string
	// Path to the log file (default go-probe.log)
	LogFile string
	// Log verbosity 4: debug, 3: info, 2: warning, 1: error (default 1)
	LogLevel int
}

var blackfireConfig BlackfireConfiguration

func getDefaultIniPath() string {
	getIniPath := func(dir string) string {
		fileName := ".blackfire.ini"
		filePath := path.Join(path.Dir(dir), fileName)
		_, err := os.Stat(filePath)
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

func configureFromDefaults(config *BlackfireConfiguration) {
	switch runtime.GOOS {
	case "windows":
		config.AgentSocket = "tcp://127.0.0.1:8307"
	case "darwin":
		config.AgentSocket = "unix:///usr/local/var/run/blackfire-agent.sock"
	case "linux":
		config.AgentSocket = "unix:///var/run/blackfire/agent.sock"
	case "freebsd":
		config.AgentSocket = "unix:///var/run/blackfire/agent.sock"
	}

	config.Endpoint = "https://blackfire.io/"
	config.LogFile = "go-probe.log"
	config.LogLevel = 1
	config.AgentTimeout = time.Millisecond * 250
}

func configureFromEnv(config *BlackfireConfiguration) {
	if v := os.Getenv("BLACKFIRE_AGENT_SOCKET"); v != "" {
		config.AgentSocket = v
	}

	if v := os.Getenv("BLACKFIRE_QUERY"); v != "" {
		config.BlackfireQuery = v
	}

	if v := os.Getenv("BLACKFIRE_CLIENT_ID"); v != "" {
		config.ClientId = v
	}

	if v := os.Getenv("BLACKFIRE_CLIENT_TOKEN"); v != "" {
		config.ClientToken = v
	}

	if v := os.Getenv("BLACKFIRE_ENDPOINT"); v != "" {
		config.Endpoint = v
	}

	if v := os.Getenv("BLACKFIRE_LOG_FILE"); v != "" {
		config.LogFile = v
	}

	if v := os.Getenv("BLACKFIRE_LOG_LEVEL"); v != "" {
		level, err := strconv.Atoi(v)
		if err != nil {
			log.Printf("Warning: env BLACKFIRE_LOG_LEVEL value %v: %v", v, err)
		} else {
			config.LogLevel = level
		}
	}
}

func parseSeconds(value string) time.Duration {
	re := regexp.MustCompile(`([0-9.]+)`)
	found := re.FindStringSubmatch(value)

	seconds, err := strconv.ParseFloat(found[1], 64)
	if err != nil {
		log.Printf("Error: blackfire.ini time value: %v: Must be numeric", value)
		return 0
	}
	return time.Duration(float64(time.Second) * seconds)
}

func configureFromIniFile(config *BlackfireConfiguration, path string) {
	if path == "" {
		if path = getDefaultIniPath(); path == "" {
			return
		}
	}

	iniConfig, err := ini.Load(path)
	if err != nil {
		log.Printf("Warning: Could not load Blackfire config file %v: %v", path, err)
		return
	}

	section := iniConfig.Section("blackfire")

	if section.HasKey("client-id") {
		config.ClientId = section.Key("client-id").String()
	}

	if section.HasKey("client-token") {
		config.ClientToken = section.Key("client-token").String()
	}

	if section.HasKey("endpoint") {
		config.Endpoint = section.Key("endpoint").String()
	}

	if section.HasKey("timeout") {
		config.AgentTimeout = parseSeconds(section.Key("timeout").String())
	}
}

// Necessary because go 1.12 doesn't have reflect.IsZero
func valueIsZero(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}

	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return math.Float64bits(v.Float()) == 0
	case reflect.Complex64, reflect.Complex128:
		c := v.Complex()
		return math.Float64bits(real(c)) == 0 && math.Float64bits(imag(c)) == 0
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return v.IsNil()
	case reflect.String:
		return v.Len() == 0
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !valueIsZero(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !valueIsZero(v.Field(i)) {
				return false
			}
		}
		return true
	}
	return false
}

func configureFromConfiguration(srcConfig *BlackfireConfiguration, dstConfig *BlackfireConfiguration) {
	if srcConfig == nil {
		return
	}

	sv := reflect.ValueOf(srcConfig).Elem()
	dv := reflect.ValueOf(dstConfig).Elem()
	for i := 0; i < sv.NumField(); i++ {
		sField := sv.Field(i)
		dField := dv.Field(i)
		if !valueIsZero(sField) {
			dField.Set(sField)
		}
	}
}

func configure(manualConfig *BlackfireConfiguration, iniFilePath string) {
	configureFromDefaults(&blackfireConfig)
	configureFromIniFile(&blackfireConfig, iniFilePath)
	configureFromEnv(&blackfireConfig)
	configureFromConfiguration(manualConfig, &blackfireConfig)
}
