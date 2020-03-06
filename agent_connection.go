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
	c := new(agentConnection)
	err := c.Init(network, address)
	return c, err
}

func (c *agentConnection) Init(network, address string) (err error) {
	if c.conn, err = net.Dial(network, address); err != nil {
		return
	}

	c.reader = bufio.NewReader(c.conn)
	c.writer = bufio.NewWriter(c.conn)
	return
}

func (c *agentConnection) ReadEncodedHeader() (name string, urlEncodedValue string, err error) {
	line, err := c.reader.ReadString('\n')
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

func (c *agentConnection) ReadResponse() (map[string]url.Values, error) {
	response := make(map[string]url.Values)

	for {
		name, urlEncodedValue, err := c.ReadEncodedHeader()
		if err != nil {
			return nil, err
		}
		if name == "" {
			break
		}
		response[name], err = url.ParseQuery(urlEncodedValue)
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (c *agentConnection) WriteEncodedHeader(name string, urlEncodedValue string) error {
	line := fmt.Sprintf("%v: %v\n", name, urlEncodedValue)
	Log.Debug().Str("write header", line).Msgf("Send header")
	_, err := c.writer.WriteString(line)
	return err
}

func (c *agentConnection) WriteStringHeader(name string, value string) error {
	return c.WriteEncodedHeader(name, url.QueryEscape(value))
}

func (c *agentConnection) WriteMapHeader(name string, values url.Values) error {
	return c.WriteEncodedHeader(name, values.Encode())
}

// Write headers in a specific order.
// The headers are assumed to be formatted and URL encoded properly.
func (c *agentConnection) WriteOrderedHeaders(encodedHeaders []string) error {
	for _, header := range encodedHeaders {
		Log.Debug().Str("write header", header).Msgf("Send ordered header")
		if _, err := c.writer.WriteString(header); err != nil {
			return err
		}
		if _, err := c.writer.WriteString("\n"); err != nil {
			return err
		}
	}
	return nil
}

func (c *agentConnection) WriteHeaders(nonEncodedHeaders map[string]interface{}) error {
	for k, v := range nonEncodedHeaders {
		if asString, ok := v.(string); ok {
			if err := c.WriteStringHeader(k, asString); err != nil {
				return err
			}
		} else {
			if err := c.WriteMapHeader(k, v.(url.Values)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *agentConnection) WriteEndOfHeaders() (err error) {
	Log.Debug().Msgf("Send end-of-headers")
	if _, err = c.writer.WriteString("\n"); err != nil {
		return
	}
	return c.Flush()
}

func (c *agentConnection) WriteRawData(data []byte) error {
	_, err := c.writer.Write(data)
	return err
}

func (c *agentConnection) Flush() error {
	return c.writer.Flush()
}

func (c *agentConnection) Close() error {
	c.Flush()
	return c.conn.Close()
}
