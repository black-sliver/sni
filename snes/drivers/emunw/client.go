package emunw

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"net"
	"sni/protos/sni"
	"sni/snes"
	"time"
)

const hextable = "0123456789abcdef"

type Client struct {
	addr *net.TCPAddr
	name string

	c           *net.TCPConn
	isConnected bool
	isClosed    bool

	readWriteTimeout time.Duration
}

// isCloseWorthy returns true if the error should close the connection
func isCloseWorthy(err error) bool {
	var coded *snes.CodedError
	if errors.As(err, &coded) {
		if coded.Code == codes.Internal {
			return true
		}
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return false
	}
	return true
}

func NewClient(addr *net.TCPAddr, name string, timeout time.Duration) (c *Client) {
	c = &Client{
		addr:             addr,
		name:             name,
		readWriteTimeout: timeout,
	}

	return
}

func (c *Client) IsConnected() bool { return c.isConnected }
func (c *Client) IsClosed() bool    { return c.isClosed }

func (c *Client) Connect() (err error) {
	c.isClosed = false
	c.c, err = net.DialTCP("tcp", nil, c.addr)
	if err != nil {
		c.isConnected = false
		return
	}
	c.isConnected = true
	return
}

func (c *Client) Close() (err error) {
	c.isClosed = true
	c.isConnected = false
	err = c.c.Close()
	return
}

func (c *Client) GetId() string {
	return c.name
}

func (c *Client) DefaultAddressSpace(context.Context) (sni.AddressSpace, error) {
	return defaultAddressSpace, nil
}

func (c *Client) writeWithDeadline(bytes []byte, deadline time.Time) (err error) {
	err = c.c.SetWriteDeadline(deadline)
	if err != nil {
		return
	}
	_, err = c.c.Write(bytes)
	return
}

func (c *Client) MultiReadMemory(ctx context.Context, reads ...snes.MemoryReadRequest) (mrsp []snes.MemoryReadResponse, err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.readWriteTimeout)
	}

	err = c.writeWithDeadline([]byte("EMU_RESET\n"), deadline)

	err = nil
	return
}

func (c *Client) MultiWriteMemory(ctx context.Context, writes ...snes.MemoryWriteRequest) (mrsp []snes.MemoryWriteResponse, err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.readWriteTimeout)
	}

	_ = deadline

	return
}

func (c *Client) ResetSystem(ctx context.Context) (err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.readWriteTimeout)
	}

	err = c.writeWithDeadline([]byte("EMU_RESET\n"), deadline)
	return
}

func (c *Client) PauseUnpause(ctx context.Context, pausedState bool) (newState bool, err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.readWriteTimeout)
	}

	if pausedState {
		err = c.writeWithDeadline([]byte("EMU_RESUME\n"), deadline)
	} else {
		err = c.writeWithDeadline([]byte("EMU_PAUSE\n"), deadline)
	}
	if err != nil {
		return
	}

	return
}

func (c *Client) PauseToggle(ctx context.Context) (err error) {
	return fmt.Errorf("capability unavailable")
}
