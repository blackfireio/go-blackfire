package blackfire

import (
	"net/url"
	"os"
	"testing"
	"time"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type BlackfireSuite struct{}

var _ = Suite(&BlackfireSuite{})

func URL(contents string) *url.URL {
	result, err := url.Parse(contents)
	if err != nil {
		panic(err)
	}
	return result
}

func newConfig() *BlackfireConfiguration {
	return &BlackfireConfiguration{
		AgentSocket:    "tcp://127.0.0.1:3333",
		AgentTimeout:   time.Second * 3,
		BlackfireQuery: "blackfire_query_manual",
		ClientID:       "client_id_manual",
		ClientToken:    "client_token_manual",
		HTTPEndpoint:   URL("https://blackfire.io/manual"),
		LogFile:        "/var/blackfire-manual.log",
		LogLevel:       3,
	}
}

func newMixedConfig() *BlackfireConfiguration {
	config := newConfig()

	// Use default
	os.Unsetenv("BLACKFIRE_ENDPOINT")
	config.HTTPEndpoint = nil

	// Use env
	config.LogFile = ""

	// Use ini
	config.AgentTimeout = 0

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

func newBlackfireConfiguration(config *BlackfireConfiguration) *BlackfireConfiguration {
	if config == nil {
		config = &BlackfireConfiguration{}
	}
	config.load()
	return config
}

func (s *BlackfireSuite) TestConfigurationDefaults(c *C) {
	config := newBlackfireConfiguration(nil)
	c.Assert("https://blackfire.io", Equals, config.HTTPEndpoint.String())
	c.Assert("stderr", Equals, config.LogFile)
	c.Assert(1, Equals, config.LogLevel)
	c.Assert(time.Millisecond*250, Equals, config.AgentTimeout)
}

func (s *BlackfireSuite) TestConfigurationIniFile(c *C) {
	config := newBlackfireConfiguration(&BlackfireConfiguration{IniFilePath: "fixtures/test_blackfire.ini"})
	c.Assert("https://blackfire.io/ini", Equals, config.HTTPEndpoint.String())
	c.Assert("ab6f24b1-3103-4503-9f68-93d4b3f10c7c", Equals, config.ClientID)
	c.Assert("ec4f5fb9f43ec7004b44fc2f217c944c324c6225efcf144c2cee65eb5c45754c", Equals, config.ClientToken)
	c.Assert(time.Second*1, Equals, config.AgentTimeout)
}

func (s *BlackfireSuite) TestConfigurationEnv(c *C) {
	setupEnv()
	defer unsetEnv()

	config := newBlackfireConfiguration(nil)
	c.Assert("tcp://127.0.0.1:2222", Equals, config.AgentSocket)
	c.Assert("blackfire_query_env", Equals, config.BlackfireQuery)
	c.Assert("client_id_env", Equals, config.ClientID)
	c.Assert("client_token_env", Equals, config.ClientToken)
	c.Assert("https://blackfire.io/env", Equals, config.HTTPEndpoint.String())
	c.Assert("/var/blackfire-env.log", Equals, config.LogFile)
	c.Assert(2, Equals, config.LogLevel)
	c.Assert(time.Millisecond*250, Equals, config.AgentTimeout)

	setupEnv()
	config = newBlackfireConfiguration(&BlackfireConfiguration{IniFilePath: "fixtures/test_blackfire.ini"})
	c.Assert("tcp://127.0.0.1:2222", Equals, config.AgentSocket)
	c.Assert("blackfire_query_env", Equals, config.BlackfireQuery)
	c.Assert("client_id_env", Equals, config.ClientID)
	c.Assert("client_token_env", Equals, config.ClientToken)
	c.Assert("https://blackfire.io/env", Equals, config.HTTPEndpoint.String())
	c.Assert("/var/blackfire-env.log", Equals, config.LogFile)
	c.Assert(2, Equals, config.LogLevel)
	c.Assert(time.Second*1, Equals, config.AgentTimeout)
}

func (s *BlackfireSuite) TestConfigurationManual(c *C) {
	config := newConfig()
	config.load()
	c.Assert("tcp://127.0.0.1:3333", Equals, config.AgentSocket)
	c.Assert("blackfire_query_manual", Equals, config.BlackfireQuery)
	c.Assert("client_id_manual", Equals, config.ClientID)
	c.Assert("client_token_manual", Equals, config.ClientToken)
	c.Assert("https://blackfire.io/manual", Equals, config.HTTPEndpoint.String())
	c.Assert("/var/blackfire-manual.log", Equals, config.LogFile)
	c.Assert(3, Equals, config.LogLevel)
	c.Assert(time.Second*3, Equals, config.AgentTimeout)

	config = newConfig()
	config.IniFilePath = "fixtures/test_blackfire.ini"
	config.load()
	c.Assert("tcp://127.0.0.1:3333", Equals, config.AgentSocket)
	c.Assert("blackfire_query_manual", Equals, config.BlackfireQuery)
	c.Assert("client_id_manual", Equals, config.ClientID)
	c.Assert("client_token_manual", Equals, config.ClientToken)
	c.Assert("https://blackfire.io/manual", Equals, config.HTTPEndpoint.String())
	c.Assert("/var/blackfire-manual.log", Equals, config.LogFile)
	c.Assert(3, Equals, config.LogLevel)
	c.Assert(time.Second*3, Equals, config.AgentTimeout)
}

func (s *BlackfireSuite) TestConfigurationMixed(c *C) {
	setupEnv()
	defer unsetEnv()

	config := newMixedConfig()
	config.load()
	c.Assert("tcp://127.0.0.1:2222", Equals, config.AgentSocket)
	c.Assert("blackfire_query_env", Equals, config.BlackfireQuery)
	c.Assert("client_id_env", Equals, config.ClientID)
	c.Assert("client_token_env", Equals, config.ClientToken)
	c.Assert("https://blackfire.io", Equals, config.HTTPEndpoint.String())
	c.Assert("/var/blackfire-env.log", Equals, config.LogFile)
	c.Assert(2, Equals, config.LogLevel)
	c.Assert(time.Millisecond*250, Equals, config.AgentTimeout)

	setupEnv()
	config = newMixedConfig()
	config.IniFilePath = "fixtures/test2_blackfire.ini"
	config.load()
	c.Assert("tcp://127.0.0.1:2222", Equals, config.AgentSocket)
	c.Assert("blackfire_query_env", Equals, config.BlackfireQuery)
	c.Assert("client_id_env", Equals, config.ClientID)
	c.Assert("client_token_env", Equals, config.ClientToken)
	c.Assert("https://blackfire.io", Equals, config.HTTPEndpoint.String())
	c.Assert("/var/blackfire-env.log", Equals, config.LogFile)
	c.Assert(2, Equals, config.LogLevel)
	c.Assert(time.Second*1, Equals, config.AgentTimeout)
}
