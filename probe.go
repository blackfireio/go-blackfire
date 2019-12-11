package blackfire

import (
	"bufio"
	"bytes"
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
)

var agentSocket string
var blackfireQuery string

var isProfiling *uint32 = new(uint32)

var profileCount = 0

func init() {
	agentSocket = os.Getenv("BLACKFIRE_AGENT_SOCKET")
	blackfireQuery = os.Getenv("BLACKFIRE_QUERY")
}

func profileFor(duration time.Duration) (profile *pprof_reader.Profile, err error) {
	var buffer bytes.Buffer

	if err = pprof.StartCPUProfile(&buffer); err != nil {
		return nil, err
	}

	time.Sleep(duration)
	pprof.StopCPUProfile()

	profile, err = pprof_reader.ReadFromPProf(&buffer)
	if err != nil {
		return nil, err
	}

	if profile == nil {
		return nil, fmt.Errorf("Profile was not created")
	}

	return
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
	if err := writeHeaders(sendHeaders, conn); err != nil {
		return err
	}

	// TODO: Don't just throw out the headers
	_, err := readHeaders(conn)

	return err
}

func checkSocketAndQueryValues() error {
	if len(agentSocket) == 0 {
		return fmt.Errorf("Profiling is disabled: Blackfire agent socket not set. Run via 'blackfire run' or call SetAgentSocket()")
	}
	if len(blackfireQuery) == 0 {
		return fmt.Errorf("Profiling is disabled: Blackfire query not set. Run via 'blackfire run' or call SetBlackfireQuery()")
	}
	return nil
}

func profileAndSend(duration time.Duration) error {
	if err := checkSocketAndQueryValues(); err != nil {
		return err
	}

	log.Printf("Profiling for %v seconds...\n", duration)

	profile, err := profileFor(duration)
	if err != nil {
		return err
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

	if err == nil {
		log.Printf("Profiling complete.\n")
	}

	return err
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

// Profile the current process for the specified number of seconds, then
// connect to the agent and upload the generated profile.
func ProfileFor(duration time.Duration) (err error) {
	if atomic.CompareAndSwapUint32(isProfiling, 0, 1) {
		err = profileAndSend(duration)
		atomic.StoreUint32(isProfiling, 0)
	} else {
		log.Printf("A Blackfire profile is currently in progress. Please wait for it to finish.")
	}
	return
}

// Trigger for the specified number of seconds when the specified signal is received.
func TriggerOnSignal(sig os.Signal, profileDuration time.Duration) {
	if err := checkSocketAndQueryValues(); err != nil {
		log.Printf("%v\n", err)
		return
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, sig)
	go func() {
		for {
			<-sigs
			if err := ProfileFor(profileDuration); err != nil {
				log.Printf("Error profiling: %v\n", err)
			}
		}
	}()
}
