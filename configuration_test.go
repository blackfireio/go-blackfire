package blackfire

import (
	"net/url"
	"os"
	"testing"
	"time"
)

func assertEqual(t *testing.T, expected, actual interface{}) {
	if actual != expected {
		t.Errorf("Expected %v but got %v", expected, actual)
	}
}

func URL(contents string) *url.URL {
	result, err := url.Parse(contents)
	if err != nil {
		panic(err)
	}
	return result
}

func newManualConfig() *BlackfireConfiguration {
	config := new(BlackfireConfiguration)
	config.AgentSocket = "tcp://127.0.0.1:3333"
	config.AgentTimeout = time.Second * 3
	config.BlackfireQuery = "blackfire_query_manual"
	config.ClientId = "client_id_manual"
	config.ClientToken = "client_token_manual"
	config.HTTPEndpoint = URL("https://blackfire.io/manual")
	config.LogFile = "/var/blackfire-manual.log"
	config.LogLevel = 3

	return config
}

func setupEnv() {
	os.Setenv("BLACKFIRE_AGENT_SOCKET", "tcp://127.0.0.1:2222")
	os.Setenv("BLACKFIRE_QUERY", "blackfire_query_env")
	os.Setenv("BLACKFIRE_CLIENT_ID", "client_id_env")
	os.Setenv("BLACKFIRE_CLIENT_TOKEN", "client_token_env")
	os.Setenv("BLACKFIRE_ENDPOINT", "https://blackfire.io/env")
	os.Setenv("BLACKFIRE_LOG_FILE", "/var/blackfire-env.log")
	os.Setenv("BLACKFIRE_LOG_LEVEL", "2")
}

func unsetEnv() {
	os.Unsetenv("BLACKFIRE_AGENT_SOCKET")
	os.Unsetenv("BLACKFIRE_QUERY")
	os.Unsetenv("BLACKFIRE_CLIENT_ID")
	os.Unsetenv("BLACKFIRE_CLIENT_TOKEN")
	os.Unsetenv("BLACKFIRE_ENDPOINT")
	os.Unsetenv("BLACKFIRE_LOG_FILE")
	os.Unsetenv("BLACKFIRE_LOG_LEVEL")
}

func newBlackfireConfiguration(manualConfig *BlackfireConfiguration, iniFilePath string) *BlackfireConfiguration {
	config := new(BlackfireConfiguration)
	config.configure(manualConfig, iniFilePath)
	return config
}

func TestConfigurationDefaults(t *testing.T) {
	config := newBlackfireConfiguration(nil, "")
	assertEqual(t, "https://blackfire.io", config.HTTPEndpoint.String())
	assertEqual(t, "go-probe.log", config.LogFile)
	assertEqual(t, 3, config.LogLevel)
	assertEqual(t, time.Millisecond*250, config.AgentTimeout)
}

func TestConfigurationIniFile(t *testing.T) {
	config := newBlackfireConfiguration(nil, "fixtures/test_blackfire.ini")
	assertEqual(t, "https://blackfire.io/ini", config.HTTPEndpoint.String())
	assertEqual(t, "ab6f24b1-3103-4503-9f68-93d4b3f10c7c", config.ClientId)
	assertEqual(t, "ec4f5fb9f43ec7004b44fc2f217c944c324c6225efcf144c2cee65eb5c45754c", config.ClientToken)
	assertEqual(t, time.Second*1, config.AgentTimeout)
}

func TestConfigurationEnv(t *testing.T) {
	setupEnv()

	config := newBlackfireConfiguration(nil, "")
	assertEqual(t, "tcp://127.0.0.1:2222", config.AgentSocket)
	assertEqual(t, "blackfire_query_env", config.BlackfireQuery)
	assertEqual(t, "client_id_env", config.ClientId)
	assertEqual(t, "client_token_env", config.ClientToken)
	assertEqual(t, "https://blackfire.io/env", config.HTTPEndpoint.String())
	assertEqual(t, "/var/blackfire-env.log", config.LogFile)
	assertEqual(t, 2, config.LogLevel)
	assertEqual(t, time.Millisecond*250, config.AgentTimeout)

	config = newBlackfireConfiguration(nil, "fixtures/test_blackfire.ini")
	assertEqual(t, "tcp://127.0.0.1:2222", config.AgentSocket)
	assertEqual(t, "blackfire_query_env", config.BlackfireQuery)
	assertEqual(t, "client_id_env", config.ClientId)
	assertEqual(t, "client_token_env", config.ClientToken)
	assertEqual(t, "https://blackfire.io/env", config.HTTPEndpoint.String())
	assertEqual(t, "/var/blackfire-env.log", config.LogFile)
	assertEqual(t, 2, config.LogLevel)
	assertEqual(t, time.Second*1, config.AgentTimeout)

	unsetEnv()
}

func TestConfigurationManual(t *testing.T) {
	setupEnv()
	manualConfig := newManualConfig()

	config := newBlackfireConfiguration(manualConfig, "")
	assertEqual(t, "tcp://127.0.0.1:3333", config.AgentSocket)
	assertEqual(t, "blackfire_query_manual", config.BlackfireQuery)
	assertEqual(t, "client_id_manual", config.ClientId)
	assertEqual(t, "client_token_manual", config.ClientToken)
	assertEqual(t, "https://blackfire.io/manual", config.HTTPEndpoint.String())
	assertEqual(t, "/var/blackfire-manual.log", config.LogFile)
	assertEqual(t, 3, config.LogLevel)
	assertEqual(t, time.Second*3, config.AgentTimeout)

	config = newBlackfireConfiguration(manualConfig, "fixtures/test_blackfire.ini")
	assertEqual(t, "tcp://127.0.0.1:3333", config.AgentSocket)
	assertEqual(t, "blackfire_query_manual", config.BlackfireQuery)
	assertEqual(t, "client_id_manual", config.ClientId)
	assertEqual(t, "client_token_manual", config.ClientToken)
	assertEqual(t, "https://blackfire.io/manual", config.HTTPEndpoint.String())
	assertEqual(t, "/var/blackfire-manual.log", config.LogFile)
	assertEqual(t, 3, config.LogLevel)
	assertEqual(t, time.Second*3, config.AgentTimeout)

	unsetEnv()
}

func TestConfigurationMixed(t *testing.T) {
	setupEnv()
	manualConfig := newManualConfig()

	// Use default
	os.Unsetenv("BLACKFIRE_ENDPOINT")
	manualConfig.HTTPEndpoint = nil

	// Use env
	manualConfig.LogFile = ""

	// Use ini
	manualConfig.AgentTimeout = 0

	config := newBlackfireConfiguration(manualConfig, "")
	assertEqual(t, "tcp://127.0.0.1:3333", config.AgentSocket)
	assertEqual(t, "blackfire_query_manual", config.BlackfireQuery)
	assertEqual(t, "client_id_manual", config.ClientId)
	assertEqual(t, "client_token_manual", config.ClientToken)
	assertEqual(t, "https://blackfire.io", config.HTTPEndpoint.String())
	assertEqual(t, "/var/blackfire-env.log", config.LogFile)
	assertEqual(t, 3, config.LogLevel)
	assertEqual(t, time.Millisecond*250, config.AgentTimeout)

	config = newBlackfireConfiguration(manualConfig, "fixtures/test2_blackfire.ini")
	assertEqual(t, "tcp://127.0.0.1:3333", config.AgentSocket)
	assertEqual(t, "blackfire_query_manual", config.BlackfireQuery)
	assertEqual(t, "client_id_manual", config.ClientId)
	assertEqual(t, "client_token_manual", config.ClientToken)
	assertEqual(t, "https://blackfire.io", config.HTTPEndpoint.String())
	assertEqual(t, "/var/blackfire-env.log", config.LogFile)
	assertEqual(t, 3, config.LogLevel)
	assertEqual(t, time.Second*1, config.AgentTimeout)

	unsetEnv()
}
