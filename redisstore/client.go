//go:generate stringer -type=responseType -linecomment

package redisstore

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
)

const (
	colon  = ':'
	dollar = '$'
	minus  = '-'
	plus   = '+'
	star   = '*'

	cr = "\r\n"
)

// responseType is an enum for the response type from redis.
type responseType int

const (
	_          responseType = iota
	typeArray               // Array
	typeBulk                // Bulk
	typeInt                 // Int
	typeNull                // Null
	typeString              // String
)

type response struct {
	typ responseType
	a   []*response
	i   int64
	s   string
}

func (r *response) array() []*response {
	return r.a
}

func (r *response) uint64() uint64 {
	return uint64(r.i)
}

// client is an individual connection to a redis instance.
type client struct {
	conn net.Conn
}

func newClient(conn net.Conn, username, password string) (*client, error) {
	c := &client{conn: conn}

	// auth
	if password != "" {
		if err := c.auth(username, password); err != nil {
			return nil, err
		}
	}

	// ping
	if err := c.ping(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *client) auth(username, password string) error {
	if username == "" && password == "" {
		return fmt.Errorf("cannot auth with empty credentials")
	}

	args := make([]string, 0, 3)
	args = append(args, "AUTH")

	if username != "" {
		args = append(args, username)
	}
	args = append(args, password)

	if _, err := c.do(args...); err != nil {
		return err
	}
	return nil
}

func (c *client) ping() error {
	if _, err := c.do("PING"); err != nil {
		return err
	}
	return nil
}

func (c *client) do(args ...string) (*response, error) {
	r := c.buildRequest(args...)
	if _, err := c.conn.Write(r); err != nil {
		return nil, err
	}

	resp, err := c.parseResponse(c.conn)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *client) release(p *pool) error {
	return p.put(c)
}

func (c *client) parseResponse(r io.Reader) (*response, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// sanity check
	if len(line) < 1 {
		return nil, fmt.Errorf("response is invalid: %v", line)
	}

	// chomp off /r/n
	content := line[1 : len(line)-2]

	switch line[0] {
	case colon:
		i, err := strconv.ParseInt(string(content), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse value as int: %w", err)
		}
		return &response{typ: typeInt, i: i}, nil
	case dollar:
		count, err := strconv.ParseInt(string(content), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bulk count: %w", err)
		}
		if count == -1 {
			return &response{typ: typeNull}, nil
		}

		// Read all bulk data. Add 2 to account for \r\n.
		read := int(count + 2)
		buf := make([]byte, read)
		n, err := io.ReadFull(br, buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read bulk response: %w", err)
		}
		if n < read {
			return nil, fmt.Errorf("expected %d bytes, got %d", read, n)
		}
		return &response{typ: typeBulk, s: string(buf[:count])}, nil
	case minus:
		return nil, fmt.Errorf(string(content))
	case plus:
		return &response{typ: typeString, s: string(content)}, nil
	case star:
		count, err := strconv.ParseInt(string(content), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bulk count: %w", err)
		}
		if count == -1 {
			return &response{typ: typeNull}, nil
		}

		responses := make([]*response, count)
		for i := int64(0); i < count; i++ {
			resp, err := c.parseResponse(br)
			if err != nil {
				return nil, err
			}
			responses[i] = resp
		}

		return &response{typ: typeArray, a: responses}, nil
	}

	return nil, fmt.Errorf("unknown response type: %v", string(line))
}

func (c *client) buildRequest(args ...string) []byte {
	l := len(args)

	b := bytes.NewBuffer(make([]byte, 0, len(args)*8))
	b.WriteByte(star)
	b.WriteString(strconv.FormatInt(int64(l), 10))
	b.WriteString(cr)

	for _, arg := range args {
		b.WriteByte(dollar)
		b.WriteString(strconv.FormatInt(int64(len(arg)), 10))
		b.WriteString(cr)
		b.WriteString(arg)
		b.WriteString(cr)
	}

	return b.Bytes()
}
