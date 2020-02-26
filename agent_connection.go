package blackfire

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"regexp"
)

var headerRegex *regexp.Regexp = regexp.MustCompile(`^([^:]+):(.*)`)

type agentConnection struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func newAgentConnection(network, address string) (*agentConnection, error) {
	this := new(agentConnection)
	err := this.Init(network, address)
	return this, err
}

func (this *agentConnection) Init(network, address string) (err error) {
	if this.conn, err = net.Dial(network, address); err != nil {
		return
	}

	this.reader = bufio.NewReader(this.conn)
	this.writer = bufio.NewWriter(this.conn)
	return
}

func (this *agentConnection) ReadEncodedHeader() (name string, urlEncodedValue string, err error) {
	line, err := this.reader.ReadString('\n')
	if err != nil {
		return
	}
	if line == "\n" {
		return
	}
	Log.Debug().Str("read header", line).Msgf("Recv header")
	matches := headerRegex.FindAllStringSubmatch(line, -1)
	if matches == nil {
		err = fmt.Errorf("Could not parse header: [%v]", line)
		return
	}
	name = matches[0][1]
	urlEncodedValue = matches[0][2]
	return
}

func (this *agentConnection) WriteEncodedHeader(name string, urlEncodedValue string) error {
	line := fmt.Sprintf("%v: %v\n", name, urlEncodedValue)
	Log.Debug().Str("write header", line).Msgf("Send header")
	_, err := this.writer.WriteString(line)
	return err
}

func (this *agentConnection) WriteStringHeader(name string, value string) error {
	return this.WriteEncodedHeader(name, url.QueryEscape(value))
}

func (this *agentConnection) WriteMapHeader(name string, values url.Values) error {
	return this.WriteEncodedHeader(name, values.Encode())
}

// Write headers in a specific order.
// The headers are assumed to be formatted and URL encoded properly.
func (this *agentConnection) WriteOrderedHeaders(encodedHeaders []string) error {
	for _, header := range encodedHeaders {
		Log.Debug().Str("write header", header).Msgf("Send ordered header")
		if _, err := this.writer.WriteString(header); err != nil {
			return err
		}
		if _, err := this.writer.WriteString("\n"); err != nil {
			return err
		}
	}
	return nil
}

func (this *agentConnection) WriteHeaders(nonEncodedHeaders map[string]interface{}) error {
	for k, v := range nonEncodedHeaders {
		if asString, ok := v.(string); ok {
			if err := this.WriteStringHeader(k, asString); err != nil {
				return err
			}
		} else {
			if err := this.WriteMapHeader(k, v.(url.Values)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (this *agentConnection) WriteEndOfHeaders() (err error) {
	Log.Debug().Msgf("Send end-of-headers")
	if _, err = this.writer.WriteString("\n"); err != nil {
		return
	}
	return this.Flush()
}

func (this *agentConnection) WriteRawData(data []byte) error {
	_, err := this.writer.Write(data)
	return err
}

func (this *agentConnection) Flush() error {
	return this.writer.Flush()
}

func (this *agentConnection) Close() error {
	this.Flush()
	return this.conn.Close()
}
