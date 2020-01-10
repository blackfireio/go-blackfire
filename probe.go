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

var maxProfileDuration time.Duration = time.Minute * 30

var agentSocket string
var blackfireQuery string
var cpuProfileBuffer bytes.Buffer
var stopProfilingTriggerChan chan bool
var isProfiling bool
var profileCount = 0
var profilerMutex sync.Mutex

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
	if isProfiling {
		return ProfilerErrorAlreadyProfiling
	}

	if err := AssertCanProfile(); err != nil {
		return err
	}

	if err := pprof.StartCPUProfile(&cpuProfileBuffer); err != nil {
		return err
	}

	isProfiling = true
	return nil
}

func stopProfiling() error {
	if !isProfiling {
		return nil
	}

	pprof.StopCPUProfile()
	isProfiling = false

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

// ----------
// Public API
// ----------

var ProfilerErrorAlreadyProfiling = errors.New("A Blackfire profile is currently in progress. Please wait for it to finish.")
var ProfilerErrorProfilingDisabled = errors.New("Profiling is disabled because the required variables are not set. To enable profiling, run via 'blackfire run' or call SetAgentSocket() and SetBlackfireQuery() manually.")

func AssertCanProfile() error {
	if len(agentSocket) == 0 || len(blackfireQuery) == 0 {
		return ProfilerErrorProfilingDisabled
	}
	return nil
}

// Check if we are running via `blackfire run`
func IsRunningViaBlackfire() bool {
	_, isRunningBlackfire := os.LookupEnv("BLACKFIRE_AGENT_SOCKET")
	return isRunningBlackfire
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

// Call this if you need to profile for longer than the default maximum (30 minutes)
func SetMaxProfileDuration(duration time.Duration) {
	maxProfileDuration = duration
}

// Check if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	return isProfiling
}

// Start profiling. Profiling will continue until you call StopProfiling().
// If you forget to stop profiling, it will automatically stop after the maximum
// allowed duration (30 minutes or whatever you set via SetMaxProfileDuration()).
func StartProfiling() error {
	return ProfileFor(maxProfileDuration)
}

// Stop profiling and upload the result to the agent.
func StopProfiling() (err error) {
	profilerMutex.Lock()
	defer profilerMutex.Unlock()

	if !isProfiling {
		return
	}

	channel := stopProfilingTriggerChan
	stopProfilingTriggerChan = make(chan bool)
	channel <- true
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
// - Call the callback (if not null).
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
	stopProfilingTriggerChan = channel

	go func() {
		<-stopProfilingTriggerChan
		profilerMutex.Lock()
		defer profilerMutex.Unlock()
		if err := stopProfiling(); err != nil {
			log.Printf("Blackfire Error (ProfileWithCallback): %v", err)
		}
		if callback != nil {
			go callback()
		}
	}()

	go func() {
		<-time.After(duration)
		channel <- true
	}()

	return nil
}

// Alias to StartProfiling
func Enable() error {
	return StartProfiling()
}

// Alias to StopProfiling
func Disable() error {
	return StopProfiling()
}
