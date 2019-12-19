package blackfire

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"github.com/blackfireio/go-blackfire/pprof_reader"
	"github.com/blackfireio/osinfo"
)

var agentSocket string
var blackfireQuery string

var cpuProfileBuffer bytes.Buffer

var isProfiling *uint32 = new(uint32)

var profileCount = 0

type ProfilerErrorType int

var ProfilerErrorAlreadyProfiling = errors.New("A Blackfire profile is currently in progress. Please wait for it to finish.")
var ProfilerErrorProfilingDisabled = errors.New("Profiling is disabled because the required variables are not set. To enable profiling, run via 'blackfire run' or call SetAgentSocket() and SetBlackfireQuery() manually.")

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

func acquireProfileLock() bool {
	return atomic.CompareAndSwapUint32(isProfiling, 0, 1)
}

func releaseProfileLock() {
	atomic.StoreUint32(isProfiling, 0)
}

func checkCanProfile() error {
	if len(agentSocket) == 0 || len(blackfireQuery) == 0 {
		return ProfilerErrorProfilingDisabled
	}
	return nil
}

// Check if we are running via `blackfire run`
func IsRunningViaBlackfire() bool {
	_, isBlackfire := os.LookupEnv("BLACKFIRE_AGENT_SOCKET")
	return isBlackfire
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

// Check if the profiler is running. Only one profiler may run at a time.
func IsProfiling() bool {
	return atomic.LoadUint32(isProfiling) != 0
}

// Alias to StartProfiling
func Enable() error {
	return StartProfiling()
}

// Start profiling. Profiling will continue until you call StopProfiling().
//
// WARNING: If you forget to stop profiling, the profile buffer will continue
// growing until it fills up all memory!
func StartProfiling() error {
	if err := checkCanProfile(); err != nil {
		return err
	}

	if acquireProfileLock() {
		if err := pprof.StartCPUProfile(&cpuProfileBuffer); err != nil {
			releaseProfileLock()
			return err
		}
	} else {
		return ProfilerErrorAlreadyProfiling
	}
	return nil
}

// Alias to StopProfiling
func Disable() error {
	return StopProfiling()
}

// Stop profiling and upload the result to the agent.
func StopProfiling() error {
	if IsProfiling() {
		defer releaseProfileLock()

		pprof.StopCPUProfile()

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
	}
	return nil
}

// Profile the current process for the specified number of seconds, then
// connect to the agent and upload the generated profile.
func ProfileFor(duration time.Duration) error {
	if err := StartProfiling(); err != nil {
		return err
	}
	time.Sleep(duration)
	if err := StopProfiling(); err != nil {
		return err
	}
	return nil
}

// Set up a trigger to start profiling when the specified signal is received.
// The profiler will profile for the specified number of seconds and then upload
// the result to the agent.
func TriggerOnSignal(sig os.Signal, profileDuration time.Duration) error {
	if err := checkCanProfile(); err != nil {
		return err
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, sig)
	go func() {
		for {
			<-sigs
			log.Printf("Blackfire: Triggered by signal %v. Profiling for %v seconds\n", sig, float64(profileDuration)/1000000000)
			if err := ProfileFor(profileDuration); err != nil {
				log.Printf("Blackfire: Error profiling: %v\n", err)
			} else {
				log.Printf("Blackfire: Profiling complete\n")
			}
		}
	}()
	return nil
}
