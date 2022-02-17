package blackfire

// TODO: AgentTimeout

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/blackfireio/go-blackfire/bf_format"
	"github.com/blackfireio/go-blackfire/pprof_reader"
	"github.com/blackfireio/osinfo"
	"github.com/rs/zerolog"
)

type agentClient struct {
	agentNetwork              string
	agentAddress              string
	signingEndpoint           *url.URL
	signingAuth               string
	serverID                  string
	serverToken               string
	links                     []*linksMap
	profiles                  []*Profile
	logger                    *zerolog.Logger
	signingResponse           *signingResponseData
	signingResponseIsConsumed bool
}

type linksMap map[string]map[string]string

func NewAgentClient(configuration *Configuration) (*agentClient, error) {
	agentNetwork, agentAddress, err := parseNetworkAddressString(configuration.AgentSocket)
	if err != nil {
		return nil, err
	}

	signingEndpoint := configuration.HTTPEndpoint
	signingEndpoint.Path = path.Join(signingEndpoint.Path, "/api/v1/signing")

	signingResponse, err := signingResponseFromBFQuery(configuration.BlackfireQuery)
	if err != nil {
		return nil, err
	}

	a := &agentClient{
		agentNetwork:              agentNetwork,
		agentAddress:              agentAddress,
		signingEndpoint:           signingEndpoint,
		signingAuth:               fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(configuration.ClientID+":"+configuration.ClientToken))),
		links:                     make([]*linksMap, 10),
		profiles:                  make([]*Profile, 10),
		logger:                    configuration.Logger,
		serverID:                  configuration.ServerID,
		serverToken:               configuration.ServerToken,
		signingResponse:           signingResponse,
		signingResponseIsConsumed: signingResponse == nil,
	}
	return a, nil
}

func (c *agentClient) CurrentBlackfireQuery() (string, error) {
	if err := c.updateSigningRequest(); err != nil {
		return "", err
	}
	return c.signingResponse.QueryString, nil
}

func (c *agentClient) LastProfiles() []*Profile {
	profiles := []*Profile{}
	for _, profile := range c.profiles {
		if profile == nil {
			continue
		}
		c.logger.Debug().Msgf("Blackfire: Get profile data for %s", profile.UUID)
		if err := profile.load(c.signingAuth); err != nil {
			c.logger.Debug().Msgf("Blackfire: Unable to get profile data for %s: %s", profile.UUID, err)
			continue
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

func (c *agentClient) ProbeOptions() bf_format.ProbeOptions {
	return c.signingResponse.Options
}

func (c *agentClient) getGoVersion() string {
	return fmt.Sprintf("go-%s", runtime.Version()[2:])
}

func (c *agentClient) getBlackfireProbeHeader(hasBlackfireYaml bool) string {
	builder := strings.Builder{}
	builder.WriteString(c.getGoVersion())
	if hasBlackfireYaml {
		builder.WriteString(", blackfire_yml")
	}
	if c.signingResponse.Options.IsTimespanFlagSet() {
		builder.WriteString(", timespan")
	}
	return builder.String()
}

func (c *agentClient) loadBlackfireYaml() (data []byte, err error) {
	filenames := []string{".blackfire.yml", ".blackfire.yaml"}

	var filename string
	for _, filename = range filenames {
		if data, err = ioutil.ReadFile(filename); err == nil {
			c.logger.Debug().Msgf("Loaded %s", filename)
			break
		} else if os.IsNotExist(err) {
			c.logger.Debug().Msgf("%s does not exist", filename)
		} else {
			return nil, err
		}
	}
	if os.IsNotExist(err) {
		err = nil
	}
	return
}

func (c *agentClient) sendBlackfireYaml(conn *agentConnection, contents []byte) (err error) {
	if err = conn.WriteStringHeader("Blackfire-Yaml-Size", strconv.Itoa(len(contents))); err != nil {
		return
	}

	c.logger.Debug().Str("blackfire.yml", string(contents)).Msgf("Send blackfire.yml, size %d", len(contents))
	err = conn.WriteRawData(contents)
	return
}

func (c *agentClient) sendProfilePrologue(conn *agentConnection) (err error) {
	// https://private.blackfire.io/knowledge-base/protocol/profiler/04-sending.html
	bfQuery, err := c.CurrentBlackfireQuery()
	if err != nil {
		return
	}

	var osVersion url.Values
	if osVersion, err = getProfileOSHeaderValue(); err != nil {
		return
	}

	var blackfireYaml []byte
	if blackfireYaml, err = c.loadBlackfireYaml(); err != nil {
		return
	}
	hasBlackfireYaml := blackfireYaml != nil

	// These must be done separately from the rest of the headers because they
	// either must be sent in a specific order, or use nonstandard encoding.
	var orderedHeaders []string
	if c.serverID != "" && c.serverToken != "" {
		orderedHeaders = append(orderedHeaders, fmt.Sprintf("Blackfire-Auth: %v:%v", c.serverID, c.serverToken))
	}
	orderedHeaders = append(orderedHeaders, fmt.Sprintf("Blackfire-Query: %s", bfQuery))
	orderedHeaders = append(orderedHeaders, fmt.Sprintf("Blackfire-Probe: %s", c.getBlackfireProbeHeader(hasBlackfireYaml)))

	unorderedHeaders := make(map[string]interface{})
	unorderedHeaders["os-version"] = osVersion

	// We've now consumed the current Blackfire query, and must fetch a new one next time.
	c.signingResponseIsConsumed = true

	// Send the ordered headers first, then wait for the Blackfire-Response,
	// then send the unordered headers.
	if err = conn.WriteOrderedHeaders(orderedHeaders); err != nil {
		return
	}

	if hasBlackfireYaml {
		if err = conn.WriteEndOfHeaders(); err != nil {
			return
		}

		var responseName string
		var responseValue string
		if responseName, responseValue, err = conn.ReadEncodedHeader(); err != nil {
			return
		}
		switch responseName {
		case "Blackfire-Response":
			var values url.Values
			if values, err = url.ParseQuery(responseValue); err != nil {
				return
			}
			if result := values.Get("blackfire_yml"); result == "true" {
				if err = c.sendBlackfireYaml(conn, blackfireYaml); err != nil {
					return
				}
			}
		case "Blackfire-Error":
			return fmt.Errorf(strings.TrimSpace(responseValue))
		default:
			return fmt.Errorf("Unexpected agent response: %s", responseValue)
		}
	}

	if err = conn.WriteHeaders(unorderedHeaders); err != nil {
		return
	}
	err = conn.WriteEndOfHeaders()
	return
}

func (c *agentClient) SendProfile(profile *pprof_reader.Profile, title string) (err error) {
	var conn *agentConnection
	if conn, err = newAgentConnection(c.agentNetwork, c.agentAddress, c.logger); err != nil {
		return
	}
	defer func() {
		if err == nil {
			c.logger.Debug().Msgf("Profile sent")
			err = conn.Close()
		} else {
			// We want the error that occurred earlier, not an error from close.
			conn.Close()
		}
	}()

	if err = c.sendProfilePrologue(conn); err != nil {
		return
	}

	var response http.Header
	if response, err = conn.ReadResponse(); err != nil {
		return err
	}
	if response.Get("Blackfire-Error") != "" {
		return fmt.Errorf("Blackfire-Error: %s", response.Get("Blackfire-Error"))
	}

	profileBuffer := new(bytes.Buffer)
	if err := bf_format.WriteBFFormat(profile, profileBuffer, c.ProbeOptions(), title); err != nil {
		return err
	}
	encodedProfile := profileBuffer.Bytes()

	c.logger.Debug().Str("contents", string(encodedProfile)).Msg("Blackfire: Send profile")
	if err = conn.WriteRawData(encodedProfile); err != nil {
		return
	}

	return
}

func (c *agentClient) updateSigningRequest() (err error) {
	if !c.signingResponseIsConsumed {
		return
	}

	var response *http.Response
	c.logger.Debug().Msgf("Blackfire: Get authorization from %s", c.signingEndpoint)
	request, err := http.NewRequest("POST", c.signingEndpoint.String(), nil)
	if err != nil {
		return
	}
	request.Header.Add("Authorization", c.signingAuth)
	c.logger.Debug().Msg("Blackfire: Send signing request")
	client := http.DefaultClient
	response, err = client.Do(request)
	if err != nil {
		return
	}
	if response.StatusCode != 201 {
		return fmt.Errorf("Signing request to %s failed: %s", c.signingEndpoint, response.Status)
	}
	var responseData []byte
	responseData, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}
	c.logger.Debug().Interface("response", string(responseData)).Msg("Blackfire: Receive signing response")
	err = json.Unmarshal(responseData, &c.signingResponse)
	if err != nil {
		return fmt.Errorf("JSON error: %v", err)
	}
	if c.signingResponse.QueryString == "" {
		return fmt.Errorf("Signing response blackfire query was empty")
	}
	profileURL, ok := c.signingResponse.Links["profile"]
	if !ok {
		return fmt.Errorf("Signing response blackfire profile URL was empty")
	}
	c.links = append([]*linksMap{&c.signingResponse.Links}, c.links[:9]...)
	c.profiles = append([]*Profile{{
		UUID:   c.signingResponse.UUID,
		URL:    c.signingResponse.Links["graph_url"]["href"],
		APIURL: profileURL["href"],
	}}, c.profiles[:9]...)

	c.signingResponseIsConsumed = false

	return
}

var nonOptionQueryFields = map[string]bool{
	"expires":     true,
	"userId":      true,
	"agentIds":    true,
	"collabToken": true,
	"signature":   true,
}

func signingResponseFromBFQuery(query string) (response *signingResponseData, err error) {
	if query == "" {
		return
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return
	}

	firstValue := func(values url.Values, key string) string {
		if vArr := values[key]; vArr != nil {
			if len(vArr) > 0 {
				return vArr[0]
			}
		}
		return ""
	}

	expires, err := strconv.ParseUint(firstValue(values, "expires"), 10, 64)
	if err != nil {
		return
	}

	response = newSigningResponseData()
	response.Agents = values["agentIds"]
	response.CollabToken = firstValue(values, "collabToken")
	response.Expires = expires
	response.QueryString = query
	response.Signature = firstValue(values, "signature")
	response.UserID = firstValue(values, "userId")

	for key, arrValues := range values {
		if nonOptionQueryFields[key] {
			continue
		}
		if len(arrValues) < 1 {
			continue
		}
		response.Options[key] = arrValues[0]
	}

	return
}

func parseNetworkAddressString(agentSocket string) (network string, address string, err error) {
	re := regexp.MustCompile(`^([^:]+)://(.*)`)
	matches := re.FindAllStringSubmatch(agentSocket, -1)
	if matches == nil {
		err = fmt.Errorf("Could not parse agent socket value: [%v]", agentSocket)
		return
	}
	network = matches[0][1]
	address = matches[0][2]
	return
}

func getProfileOSHeaderValue() (values url.Values, err error) {
	var info *osinfo.OSInfo
	info, err = osinfo.GetOSInfo()
	if err != nil {
		return
	}

	values = make(url.Values)
	values["family"] = []string{info.Family}
	values["arch"] = []string{info.Architecture}
	values["id"] = []string{info.ID}
	values["version"] = []string{info.Version}
	if len(info.Codename) > 0 {
		values["codename"] = []string{info.Codename}
	}
	if len(info.Build) > 0 {
		values["build"] = []string{info.Build}
	}

	return values, nil
}

type signingResponseData struct {
	UserID      string                 `json:"userId"`
	ProfileSlot string                 `json:"profileSlot"`
	CollabToken string                 `json:"collabToken"`
	Agents      []string               `json:"agents"`
	Expires     uint64                 `json:"expires,string"`
	Signature   string                 `json:"signature"`
	Options     bf_format.ProbeOptions `json:"options"`
	Links       linksMap               `json:"_links"`
	UUID        string                 `json:"uuid"`
	QueryString string                 `json:"query_string"`
}

func newSigningResponseData() *signingResponseData {
	s := new(signingResponseData)
	s.Options = make(bf_format.ProbeOptions)
	return s
}
