package blackfire

import (
	"os"
	"testing"
	"time"
)

func assertEqual(t *testing.T, expected, actual interface{}) {
	if actual != expected {
		t.Errorf("Expected %v but got %v", expected, actual)
	}
}

func newConfig() *BlackfireConfiguration {
	config := new(BlackfireConfiguration)
	config.AgentSocket = "tcp://127.0.0.1:3333"
	config.AgentTimeout = time.Second * 3
	config.BlackfireQuery = "blackfire_query_manual"
	config.ClientId = "client_id_manual"
	config.ClientToken = "client_token_manual"
	config.Endpoint = "https://blackfire.io/manual"
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

func TestConfigurationDefaults(t *testing.T) {
	configure(nil, "")
	assertEqual(t, "https://blackfire.io/", blackfireConfig.Endpoint)
	assertEqual(t, "go-probe.log", blackfireConfig.LogFile)
	assertEqual(t, 1, blackfireConfig.LogLevel)
	assertEqual(t, time.Millisecond*250, blackfireConfig.AgentTimeout)
}

func TestConfigurationIniFile(t *testing.T) {
	configure(nil, "fixtures/test_blackfire.ini")
	assertEqual(t, "https://blackfire.io/ini", blackfireConfig.Endpoint)
	assertEqual(t, "ab6f24b1-3103-4503-9f68-93d4b3f10c7c", blackfireConfig.ClientId)
	assertEqual(t, "ec4f5fb9f43ec7004b44fc2f217c944c324c6225efcf144c2cee65eb5c45754c", blackfireConfig.ClientToken)
	assertEqual(t, time.Second*1, blackfireConfig.AgentTimeout)
}

func TestConfigurationEnv(t *testing.T) {
	setupEnv()

	configure(nil, "")
	assertEqual(t, "tcp://127.0.0.1:2222", blackfireConfig.AgentSocket)
	assertEqual(t, "blackfire_query_env", blackfireConfig.BlackfireQuery)
	assertEqual(t, "client_id_env", blackfireConfig.ClientId)
	assertEqual(t, "client_token_env", blackfireConfig.ClientToken)
	assertEqual(t, "https://blackfire.io/env", blackfireConfig.Endpoint)
	assertEqual(t, "/var/blackfire-env.log", blackfireConfig.LogFile)
	assertEqual(t, 2, blackfireConfig.LogLevel)
	assertEqual(t, time.Millisecond*250, blackfireConfig.AgentTimeout)

	configure(nil, "fixtures/test_blackfire.ini")
	assertEqual(t, "tcp://127.0.0.1:2222", blackfireConfig.AgentSocket)
	assertEqual(t, "blackfire_query_env", blackfireConfig.BlackfireQuery)
	assertEqual(t, "client_id_env", blackfireConfig.ClientId)
	assertEqual(t, "client_token_env", blackfireConfig.ClientToken)
	assertEqual(t, "https://blackfire.io/env", blackfireConfig.Endpoint)
	assertEqual(t, "/var/blackfire-env.log", blackfireConfig.LogFile)
	assertEqual(t, 2, blackfireConfig.LogLevel)
	assertEqual(t, time.Second*1, blackfireConfig.AgentTimeout)

	unsetEnv()
}

func TestConfigurationManual(t *testing.T) {
	setupEnv()
	config := newConfig()

	configure(config, "")
	assertEqual(t, "tcp://127.0.0.1:3333", blackfireConfig.AgentSocket)
	assertEqual(t, "blackfire_query_manual", blackfireConfig.BlackfireQuery)
	assertEqual(t, "client_id_manual", blackfireConfig.ClientId)
	assertEqual(t, "client_token_manual", blackfireConfig.ClientToken)
	assertEqual(t, "https://blackfire.io/manual", blackfireConfig.Endpoint)
	assertEqual(t, "/var/blackfire-manual.log", blackfireConfig.LogFile)
	assertEqual(t, 3, blackfireConfig.LogLevel)
	assertEqual(t, time.Second*3, blackfireConfig.AgentTimeout)

	configure(config, "fixtures/test_blackfire.ini")
	assertEqual(t, "tcp://127.0.0.1:3333", blackfireConfig.AgentSocket)
	assertEqual(t, "blackfire_query_manual", blackfireConfig.BlackfireQuery)
	assertEqual(t, "client_id_manual", blackfireConfig.ClientId)
	assertEqual(t, "client_token_manual", blackfireConfig.ClientToken)
	assertEqual(t, "https://blackfire.io/manual", blackfireConfig.Endpoint)
	assertEqual(t, "/var/blackfire-manual.log", blackfireConfig.LogFile)
	assertEqual(t, 3, blackfireConfig.LogLevel)
	assertEqual(t, time.Second*3, blackfireConfig.AgentTimeout)

	unsetEnv()
}

func TestConfigurationMixed(t *testing.T) {
	setupEnv()
	config := newConfig()

	// Use default
	os.Unsetenv("BLACKFIRE_ENDPOINT")
	config.Endpoint = ""

	// Use env
	config.LogFile = ""

	// Use ini
	config.AgentTimeout = 0

	configure(config, "")
	assertEqual(t, "tcp://127.0.0.1:3333", blackfireConfig.AgentSocket)
	assertEqual(t, "blackfire_query_manual", blackfireConfig.BlackfireQuery)
	assertEqual(t, "client_id_manual", blackfireConfig.ClientId)
	assertEqual(t, "client_token_manual", blackfireConfig.ClientToken)
	assertEqual(t, "https://blackfire.io/", blackfireConfig.Endpoint)
	assertEqual(t, "/var/blackfire-env.log", blackfireConfig.LogFile)
	assertEqual(t, 3, blackfireConfig.LogLevel)
	assertEqual(t, time.Millisecond*250, blackfireConfig.AgentTimeout)

	configure(config, "fixtures/test2_blackfire.ini")
	assertEqual(t, "tcp://127.0.0.1:3333", blackfireConfig.AgentSocket)
	assertEqual(t, "blackfire_query_manual", blackfireConfig.BlackfireQuery)
	assertEqual(t, "client_id_manual", blackfireConfig.ClientId)
	assertEqual(t, "client_token_manual", blackfireConfig.ClientToken)
	assertEqual(t, "https://blackfire.io/", blackfireConfig.Endpoint)
	assertEqual(t, "/var/blackfire-env.log", blackfireConfig.LogFile)
	assertEqual(t, 3, blackfireConfig.LogLevel)
	assertEqual(t, time.Second*1, blackfireConfig.AgentTimeout)

	unsetEnv()
}
