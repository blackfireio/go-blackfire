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

func getAgentSigningURL(endpoint *url.URL) *url.URL {
	const signingPath = "/api/v1/signing"
	u := new(url.URL)
	*u = *endpoint
	u.Path = path.Join(u.Path, signingPath)
	return u
}

func getSigningAuthorization(clientID, clientToken string) string {
	idToken := clientID + ":" + clientToken
	return fmt.Sprintf("Basic %v", base64.StdEncoding.EncodeToString([]byte(idToken)))
}

func sendSigningRequest(baseURL *url.URL, clientID string, clientToken string) (signingResponse map[string]interface{}, err error) {
	signingURL := getAgentSigningURL(baseURL)
	signingAuth := getSigningAuthorization(clientID, clientToken)
	var request *http.Request
	var response *http.Response
	Log.Debug().Msgf("Blackfire: Get authorization from %v", signingURL)
	request, err = http.NewRequest("POST", signingURL.String(), nil)
	if err != nil {
		return
	}
	request.Header.Add("Authorization", signingAuth)
	Log.Debug().Msg("Blackfire: Send signing request")
	client := http.DefaultClient
	response, err = client.Do(request)
	if err != nil {
		return
	}
	if response.StatusCode != 201 {
		err = fmt.Errorf("Signing request to %v failed: %v", signingURL, response.Status)
		return
	}
	var responseData []byte
	responseData, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}
	Log.Debug().Interface("response", string(responseData)).Msg("Blackfire: Receive signing response")
	err = json.Unmarshal(responseData, &signingResponse)
	if err != nil {
		err = fmt.Errorf("JSON error: %v", err)
		return
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

type AgentClient struct {
	profileCount      int
	agentNetwork      string
	agentAddress      string
	rawBlackfireQuery string
}

func NewAgentClient(agentSocket, blackfireQuery string) (*AgentClient, error) {
	this := new(AgentClient)
	if err := this.Init(agentSocket, blackfireQuery); err != nil {
		return nil, err
	}
	return this, nil
}

func NewAgentClientWithSigningRequest(agentSocket string, httpEndpoint *url.URL, clientID string, clientToken string) (*AgentClient, error) {
	this := new(AgentClient)
	if err := this.InitWithSigningRequest(agentSocket, httpEndpoint, clientID, clientToken); err != nil {
		return nil, err
	}
	return this, nil
}

func (this *AgentClient) Init(agentSocket, blackfireQuery string) (err error) {
	this.agentNetwork, this.agentAddress, err = parseNetworkAddressString(agentSocket)
	if err != nil {
		return
	}
	this.rawBlackfireQuery = blackfireQuery
	return
}

func (this *AgentClient) InitWithSigningRequest(agentSocket string, authHTTPEndpoint *url.URL, clientID string, clientToken string) (err error) {
	var signingResponse map[string]interface{}
	if signingResponse, err = sendSigningRequest(authHTTPEndpoint, clientID, clientToken); err != nil {
		return
	}

	blackfireQuery, ok := signingResponse["query_string"].(string)
	if !ok {
		return fmt.Errorf("Signing response didn't contain blackfire query")
	}
	if blackfireQuery == "" {
		return fmt.Errorf("Signing response blackfire query was empty")
	}
	return this.Init(agentSocket, blackfireQuery)
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

func (this *AgentClient) getGoVersion() string {
	return fmt.Sprintf("go-%v", runtime.Version()[2:])
}

func (this *AgentClient) getBlackfireQueryHeader() string {
	builder := strings.Builder{}
	builder.WriteString(this.rawBlackfireQuery)
	if this.profileCount > 0 {
		builder.WriteString("&sub_profile=")
		builder.WriteString(fmt.Sprintf("%09d", this.profileCount))
	}
	return builder.String()
}

func (this *AgentClient) getBlackfireProbeHeader(hasBlackfireYaml bool) string {
	builder := strings.Builder{}
	builder.WriteString(this.getGoVersion())
	if hasBlackfireYaml {
		builder.WriteString(", blackfire_yml")
	}
	return builder.String()
}

func (this *AgentClient) loadBlackfireYaml() (data []byte, err error) {
	filenames := []string{".blackfire.yml", ".blackfire.yaml"}

	var filename string
	for _, filename = range filenames {
		if data, err = ioutil.ReadFile(filename); err == nil {
			Log.Debug().Msgf("Loaded %v", filename)
			break
		} else if os.IsNotExist(err) {
			Log.Debug().Msgf("%v does not exist", filename)
		} else {
			return nil, err
		}
	}
	if os.IsNotExist(err) {
		err = nil
	}
	return
}

func (this *AgentClient) sendBlackfireYaml(conn *agentConnection, contents []byte) (err error) {
	if err = conn.WriteStringHeader("Blackfire-Yaml-Size", strconv.Itoa(len(contents))); err != nil {
		return
	}

	Log.Debug().Str("blackfire.yml", string(contents)).Msgf("Send blackfire.yml, size %v", len(contents))
	err = conn.WriteRawData(contents)
	return
}

func (this *AgentClient) sendProfilePrologue(conn *agentConnection) (err error) {
	// https://private.blackfire.io/docs/tech/profiling-protocol/#profile-creation-prolog
	if len(this.rawBlackfireQuery) == 0 {
		return fmt.Errorf("Agent client has not been properly initialized (Blackfire query is not set)")
	}

	var osVersion url.Values
	if osVersion, err = getProfileOSHeaderValue(); err != nil {
		return
	}

	var blackfireYaml []byte
	if blackfireYaml, err = this.loadBlackfireYaml(); err != nil {
		return
	}
	hasBlackfireYaml := blackfireYaml != nil

	// These must be done separately from the rest of the headers because they
	// either must be sent in a specific order, or use nonstandard encoding.
	orderedHeaders := []string{
		fmt.Sprintf("Blackfire-Query: %v", this.getBlackfireQueryHeader()),
		fmt.Sprintf("Blackfire-Probe: %v", this.getBlackfireProbeHeader(hasBlackfireYaml)),
	}

	unorderedHeaders := make(map[string]interface{})
	unorderedHeaders["os-version"] = osVersion

	// Send the ordered headers first, then wait for the Blackfire-Response,
	// then send the unordered headers.
	if err = conn.WriteOrderedHeaders(orderedHeaders); err != nil {
		return
	}
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
			if err = this.sendBlackfireYaml(conn, blackfireYaml); err != nil {
				return
			}
		}
	case "Blackfire-Error":
		return fmt.Errorf(strings.TrimSpace(responseValue))
	default:
		return fmt.Errorf("Unexpected agent response: %v", responseValue)
	}

	if err = conn.WriteHeaders(unorderedHeaders); err != nil {
		return
	}
	err = conn.WriteEndOfHeaders()
	return
}

func (this *AgentClient) SendProfile(encodedProfile []byte) (err error) {
	var conn *agentConnection
	if conn, err = newAgentConnection(this.agentNetwork, this.agentAddress); err != nil {
		return
	}
	defer conn.Close()

	if err = this.sendProfilePrologue(conn); err != nil {
		return
	}

	Log.Debug().Str("contents", string(encodedProfile)).Msg("Blackfire: Send profile")
	if err = conn.WriteRawData(encodedProfile); err != nil {
		return
	}

	// Force a close here so that we can catch any errors. This is idempotent
	// so it's fine.
	if err = conn.Close(); err != nil {
		return
	}

	this.profileCount++
	Log.Debug().Msgf("Profile %v sent", this.profileCount)
	return
}
