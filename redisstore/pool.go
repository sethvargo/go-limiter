package redisstore

import (
	"fmt"
	"net"
	"strings"
)

// pool is a pooled block of clients.
type pool struct {
	// clients is a buffered channel of the available clients.
	clients chan *client

	// available is a buffered channel that allows up to the max number of
	// clients.
	available chan struct{}

	// clientFunc is the function that connects to Redis and configures a new
	// client.
	clientFunc func() (*client, error)

	// stopCh is used to signal the pool is stopped.
	stopCh chan struct{}
}

type poolConfig struct {
	initial, max uint64
	dialFunc     func() (net.Conn, error)

	// username and password are for auth.
	username string
	password string
}

var errPoolClosed = fmt.Errorf("pool is closed")

func newPool(c *poolConfig) (*pool, error) {
	if c.initial > c.max {
		return nil, fmt.Errorf("initial cannot be greater than max")
	}

	p := &pool{
		clients:   make(chan *client, c.max),
		available: make(chan struct{}, c.max),
		stopCh:    make(chan struct{}),

		clientFunc: func() func() (*client, error) {
			dialFunc := c.dialFunc
			return func() (*client, error) {
				conn, err := dialFunc()
				if err != nil {
					return nil, err
				}

				client, err := newClient(conn, c.username, c.password)
				if err != nil {
					return nil, err
				}
				return client, nil
			}
		}(),
	}

	// Create initial connections.
	for i := uint64(0); i < c.initial; i++ {
		client, err := p.clientFunc()
		if err != nil {
			return nil, err
		}

		p.clients <- client
		p.available <- struct{}{}
	}

	return p, nil
}

func (p *pool) get() (*client, error) {
	select {
	case <-p.stopCh:
		return nil, errPoolClosed
	case client, ok := <-p.clients:
		if !ok {
			return nil, errPoolClosed
		}
		return client, nil
	default:
	}

	for {
		select {
		case <-p.stopCh:
			return nil, errPoolClosed
		case client, ok := <-p.clients:
			if !ok {
				return nil, errPoolClosed
			}
			return client, nil
		case p.available <- struct{}{}:
			client, err := p.clientFunc()
			if err != nil {
				<-p.available
				return nil, err
			}

			select {
			case p.clients <- client:
				return client, nil
			default:
				<-p.available
				if err := client.conn.Close(); err != nil {
					return nil, err
				}
			}
		}
	}
}

func (p *pool) put(client *client) error {
	if client == nil {
		return nil
	}

	select {
	case <-p.stopCh:
		if err := client.conn.Close(); err != nil {
			return err
		}
		return errPoolClosed
	default:
	}

	select {
	case <-p.stopCh:
		if err := client.conn.Close(); err != nil {
			return err
		}
		return errPoolClosed
	case p.clients <- client:
		return nil
	default:
		select {
		case <-p.available:
		default:
		}

		return client.conn.Close()
	}
}

func (p *pool) close() error {
	close(p.stopCh)
	close(p.clients)
	close(p.available)

	var errs []error
	for client := range p.clients {
		if err := client.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		strs := make([]string, len(errs))
		for i, err := range errs {
			strs[i] = err.Error()
		}
		return fmt.Errorf("%d errors trying to close: %v", len(errs), strings.Join(strs, ", "))
	}
}
