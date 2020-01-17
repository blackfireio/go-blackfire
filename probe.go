package blackfire

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/blackfireio/go-blackfire/pprof_reader"
	"github.com/blackfireio/osinfo"
)

const DefaultMaxProfileDuration = time.Minute * 10

type profilerState int

const (
	profilerStateIdle profilerState = iota
	profilerStateProfiling
	profilerStateSending
)

var profilerMutex sync.Mutex
var triggerDisableProfilingChan chan bool
var currentState profilerState
var endOnNextProfile bool

var agentSocket string
var blackfireQuery string
var maxProfileDuration = DefaultMaxProfileDuration
var cpuProfileBuffer bytes.Buffer
var profileCount = 0

func init() {
	agentSocket = os.Getenv("BLACKFIRE_AGENT_SOCKET")
	blackfireQuery = os.Getenv("BLACKFIRE_QUERY")
}

func connectToAgent() (net.Conn, error) {
	re := regexp.MustCompile(`^([^:]+)://(.*)`)
	matches := re.FindAllStringSubmatch(agentSocket, -1)
	if matches == nil {
		return nil, fmt.Errorf("Could not parse agent socket value: [%v]", agentSocket)
	}
	network := matches[0][1]
	address := matches[0][2]

	return net.Dial(network, address)
}

func readHeaders(conn net.Conn) (map[string]string, error) {
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

func writeHeaders(headers map[string]string, conn net.Conn) error {
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

func getOSHeader() string {
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

func sendPrologue(conn net.Conn) error {
	if blackfireQuery == "" {
		return fmt.Errorf("Blackfire query not set")
	}

	fullBlackfireQuery := blackfireQuery
	if profileCount > 0 {
		fullBlackfireQuery = fmt.Sprintf("%v&sub_profile=:%09d", blackfireQuery, profileCount)
	}

	sendHeaders := make(map[string]string)
	sendHeaders["Blackfire-Query"] = fullBlackfireQuery
	sendHeaders["Blackfire-Probe"] = "go-1.13.3"
	sendHeaders["os-version"] = getOSHeader()
	if err := writeHeaders(sendHeaders, conn); err != nil {
		return err
	}

	// TODO: Maybe validate the headers rather than ignoring them?
	_, err := readHeaders(conn)

	return err
}

func startProfiling() error {
	if currentState != profilerStateIdle {
		return ProfilerErrorAlreadyProfiling
	}

	if err := AssertCanProfile(); err != nil {
		return err
	}

	if err := pprof.StartCPUProfile(&cpuProfileBuffer); err != nil {
		return err
	}

	endOnNextProfile = false
	currentState = profilerStateProfiling
	return nil
}

func stopProfiling() {
	if currentState != profilerStateProfiling {
		return
	}

	pprof.StopCPUProfile()
	currentState = profilerStateIdle
	return
}

func endProfile() error {
	if currentState == profilerStateSending {
		stopProfiling()
	}

	if currentState != profilerStateIdle {
		return nil
	}

	currentState = profilerStateSending
	defer func() {
		currentState = profilerStateIdle
	}()

	profile, err := pprof_reader.ReadFromPProf(&cpuProfileBuffer)
	if err != nil {
		return err
	}

	if profile == nil {
		return fmt.Errorf("Profile was not created")
	}

	if !profile.HasData() {
		return nil
	}

	conn, err := connectToAgent()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := sendPrologue(conn); err != nil {
		return err
	}

	if err := pprof_reader.WriteBFFormat(profile, profile.BiggestImpactEntryPoint(), conn); err != nil {
		return err
	}

	profileCount++
	return nil
}

func triggerProfilerDisable() {
	channel := triggerDisableProfilingChan
	triggerDisableProfilingChan = make(chan bool)
	channel <- true
}

func onProfileDisableTriggered(callback func()) {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if endOnNextProfile {
		if err := endProfile(); err != nil {
			log.Printf("Blackfire Error (ProfileWithCallback): %v", err)
		}
	} else {
		stopProfiling()
	}

	if callback != nil {
		go callback()
	}
}

// ----------
// Public API
// ----------

var ProfilerErrorAlreadyProfiling = errors.New("A Blackfire profile is currently in progress. Please wait for it to finish.")
var ProfilerErrorProfilingDisabled = errors.New("Profiling is disabled because the required ENV variables are not set. To enable profiling, run via 'blackfire run', set BLACKFIRE_AGENT_SOCKET and BLACKFIRE_QUERY env variables, or call SetAgentSocket() and SetBlackfireQuery() manually.")

func AssertCanProfile() error {
	if len(agentSocket) == 0 || len(blackfireQuery) == 0 {
		return ProfilerErrorProfilingDisabled
	}
	return nil
}

// Set the agent socket to connect to. Defaults to whatever is in the env BLACKFIRE_AGENT_SOCKET.
// Example: tcp://127.0.0.1:40635
func SetAgentSocket(newValue string) {
	agentSocket = newValue
}

// Set the blackfire query contents. Defaults to the env BLACKFIRE_QUERY.
func SetBlackfireQuery(newValue string) {
	blackfireQuery = newValue
}

// Call this if you need to profile for longer than the default maximum from DefaultMaxProfileDuration
func SetMaxProfileDuration(duration time.Duration) {
	maxProfileDuration = duration
}

// Check if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	return currentState != profilerStateIdle
}

// Start profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (DefaultMaxProfileDuration or whatever you set via SetMaxProfileDuration()).
func Enable() error {
	return ProfileFor(maxProfileDuration)
}

// Stop profiling and upload the result to the agent.
func Disable() {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if currentState != profilerStateProfiling {
		return
	}

	triggerProfilerDisable()
	return
}

// Stop profiling and upload the result to the agent.
func End() {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	switch currentState {
	case profilerStateProfiling:
		endOnNextProfile = true
		triggerProfilerDisable()
	case profilerStateIdle:
		endOnNextProfile = true
		go func() {
			onProfileDisableTriggered(nil)
		}()
	}

	return
}

// Profile the current process for the specified duration, then
// connect to the agent and upload the generated profile.
func ProfileFor(duration time.Duration) error {
	return ProfileWithCallback(duration, nil)
}

// Does the following:
// - Profile the current process for the specified duration.
// - Connect to the agent and upload the generated profile.
// - Call the callback in a goroutine (if not null).
func ProfileWithCallback(duration time.Duration, callback func()) error {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if duration > maxProfileDuration {
		duration = maxProfileDuration
	}

	if err := startProfiling(); err != nil {
		return err
	}

	channel := make(chan bool)

	go func() {
		<-channel
		onProfileDisableTriggered(callback)
	}()

	go func() {
		<-time.After(duration)
		channel <- true
	}()

	triggerDisableProfilingChan = channel

	return nil
}
