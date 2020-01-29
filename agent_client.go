package blackfire

// TODO: AgentTimeout

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"runtime"

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
	request, err = http.NewRequest("POST", signingURL.String(), nil)
	if err != nil {
		return
	}
	request.Header.Add("Authorization", signingAuth)
	client := http.DefaultClient
	response, err = client.Do(request)
	if response.StatusCode/100 != 2 {
		err = fmt.Errorf("Signing request to %v failed: %v", signingURL, response.Status)
		return
	}
	var responseData []byte
	responseData, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}
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
	profileCount   int
	agentNetwork   string
	agentAddress   string
	blackfireQuery string
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
	this.blackfireQuery = blackfireQuery
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

func readProfileResponseHeaders(conn net.Conn) (map[string]string, error) {
	re := regexp.MustCompile(`^([^:]+):(.*)`)
	headers := make(map[string]string)
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\n" {
			break
		}
		matches := re.FindAllStringSubmatch(line, -1)
		if matches == nil {
			return nil, fmt.Errorf("Could not parse header: [%v]", line)
		}
		k := matches[0][1]
		v := matches[0][2]

		headers[k] = v
	}

	return headers, nil
}

func writeProfileSendHeaders(headers map[string]string, conn net.Conn) error {
	w := bufio.NewWriter(conn)
	for k, v := range headers {
		line := fmt.Sprintf("%v: %v\n", k, v)
		_, err := w.WriteString(line)
		if err != nil {
			return err
		}
	}
	_, err := w.WriteString("\n")
	if err != nil {
		return err
	}
	return w.Flush()
}

func getProfileOSHeader() string {
	info, err := osinfo.GetOSInfo()
	if err != nil {
		log.Printf("OSINFO: %v\n", err)
	}
	codename := info.Codename
	if len(codename) > 0 {
		codename = " codename=" + codename
	}
	build := info.Build
	if len(build) > 0 {
		build = " build=" + build
	}

	return fmt.Sprintf("family=%v arch=%v id=%v version=%v %v%v", info.Family, info.Architecture, info.ID, info.Version, codename, build)
}

func (this *AgentClient) sendProfilePrologue(conn net.Conn) error {
	if this.blackfireQuery == "" {
		return fmt.Errorf("Agent client has not been properly initialized (Blackfire query is not set)")
	}

	fullBlackfireQuery := this.blackfireQuery
	if this.profileCount > 0 {
		fullBlackfireQuery = fmt.Sprintf("%v&sub_profile=:%09d", this.blackfireQuery, this.profileCount)
	}

	sendHeaders := make(map[string]string)
	sendHeaders["Blackfire-Query"] = fullBlackfireQuery
	sendHeaders["Blackfire-Probe"] = runtime.Version()
	sendHeaders["os-version"] = getProfileOSHeader()
	if err := writeProfileSendHeaders(sendHeaders, conn); err != nil {
		return err
	}

	// TODO: Maybe validate the headers rather than ignoring them?
	_, err := readProfileResponseHeaders(conn)

	return err
}

func (this *AgentClient) SendProfile(encodedProfile []byte) error {
	conn, err := net.Dial(this.agentNetwork, this.agentAddress)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := this.sendProfilePrologue(conn); err != nil {
		return err
	}

	if _, err := conn.Write(encodedProfile); err != nil {
		return err
	}

	this.profileCount++
	return nil
}
