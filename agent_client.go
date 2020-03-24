package blackfire

// TODO: AgentTimeout

import (
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

	"github.com/blackfireio/osinfo"
)

type agentClient struct {
	agentNetwork        string
	agentAddress        string
	signingEndpoint     *url.URL
	signingAuth         string
	firstBlackfireQuery string
	rawBlackfireQuery   string
	links               []linksMap
}

type linksMap map[string]map[string]string

type signature struct {
	QueryString string   `json:"query_string"`
	Links       linksMap `json:"_links"`
}

func NewAgentClient(configuration *BlackfireConfiguration) (*agentClient, error) {
	agentNetwork, agentAddress, err := parseNetworkAddressString(configuration.AgentSocket)
	if err != nil {
		return nil, err
	}

	signingEndpoint := configuration.HTTPEndpoint
	signingEndpoint.Path = path.Join(signingEndpoint.Path, "/api/v1/signing")

	return &agentClient{
		agentNetwork:        agentNetwork,
		agentAddress:        agentAddress,
		signingEndpoint:     signingEndpoint,
		signingAuth:         fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(configuration.ClientID+":"+configuration.ClientToken))),
		firstBlackfireQuery: configuration.BlackfireQuery,
		links:               make([]linksMap, 10),
	}, nil
}

func (c *agentClient) CurrentBlackfireQuery() (string, error) {
	if c.rawBlackfireQuery != "" {
		return c.rawBlackfireQuery, nil
	}
	if c.firstBlackfireQuery != "" {
		c.rawBlackfireQuery = c.firstBlackfireQuery
		c.firstBlackfireQuery = ""
		return c.rawBlackfireQuery, nil
	}
	query, err := c.createRequest()
	if err != nil {
		return "", err
	}
	c.rawBlackfireQuery = query
	return c.rawBlackfireQuery, nil
}

func (c *agentClient) LastProfileURLs() []string {
	urls := []string{}
	for _, links := range c.links {
		if graphURL, ok := links["graph_url"]; ok {
			urls = append(urls, graphURL["href"])
		}
	}

	return urls
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
	return builder.String()
}

func (c *agentClient) loadBlackfireYaml() (data []byte, err error) {
	filenames := []string{".blackfire.yml", ".blackfire.yaml"}

	var filename string
	for _, filename = range filenames {
		if data, err = ioutil.ReadFile(filename); err == nil {
			Log.Debug().Msgf("Loaded %s", filename)
			break
		} else if os.IsNotExist(err) {
			Log.Debug().Msgf("%s does not exist", filename)
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

	Log.Debug().Str("blackfire.yml", string(contents)).Msgf("Send blackfire.yml, size %d", len(contents))
	err = conn.WriteRawData(contents)
	return
}

func (c *agentClient) sendProfilePrologue(conn *agentConnection) (err error) {
	// https://private.blackfire.io/docs/tech/profiling-protocol/#profile-creation-prolog
	if _, err := c.CurrentBlackfireQuery(); err != nil {
		return err
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
	orderedHeaders := []string{
		fmt.Sprintf("Blackfire-Query: %s", c.rawBlackfireQuery),
		fmt.Sprintf("Blackfire-Probe: %s", c.getBlackfireProbeHeader(hasBlackfireYaml)),
	}
	c.rawBlackfireQuery = ""

	unorderedHeaders := make(map[string]interface{})
	unorderedHeaders["os-version"] = osVersion

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

func (c *agentClient) SendProfile(encodedProfile []byte) (err error) {
	var conn *agentConnection
	if conn, err = newAgentConnection(c.agentNetwork, c.agentAddress); err != nil {
		return
	}
	defer func() {
		if err == nil {
			Log.Debug().Msgf("Profile sent")
			err = conn.Close()
		} else {
			// We want the error that occurred earlier, not an error from close.
			conn.Close()
		}
	}()

	if err = c.sendProfilePrologue(conn); err != nil {
		return
	}

	var response map[string]url.Values
	if response, err = conn.ReadResponse(); err != nil {
		return err
	}
	if errResp, ok := response["Blackfire-Error"]; ok {
		return fmt.Errorf("Blackfire-Error: %s", errResp)
	}

	Log.Debug().Str("contents", string(encodedProfile)).Msg("Blackfire: Send profile")
	if err = conn.WriteRawData(encodedProfile); err != nil {
		return
	}

	return
}

func (c *agentClient) createRequest() (string, error) {
	var response *http.Response
	Log.Debug().Msgf("Blackfire: Get authorization from %s", c.signingEndpoint)
	request, err := http.NewRequest("POST", c.signingEndpoint.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Add("Authorization", c.signingAuth)
	Log.Debug().Msg("Blackfire: Send signing request")
	client := http.DefaultClient
	response, err = client.Do(request)
	if err != nil {
		return "", err
	}
	if response.StatusCode != 201 {
		return "", fmt.Errorf("Signing request to %s failed: %s", c.signingEndpoint, response.Status)
	}
	var responseData []byte
	responseData, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	Log.Debug().Interface("response", string(responseData)).Msg("Blackfire: Receive signing response")
	var signingResponse signature
	err = json.Unmarshal(responseData, &signingResponse)
	if err != nil {
		err = fmt.Errorf("JSON error: %v", err)
		return "", err
	}
	if signingResponse.QueryString == "" {
		return "", fmt.Errorf("Signing response blackfire query was empty")
	}
	c.links = append([]linksMap{signingResponse.Links}, c.links[:9]...)
	return signingResponse.QueryString, nil
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
