package connectorlib

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sni/protos/sni"
	"sni/snes"
	"time"
)

var downstreamDevice snes.AutoCloseableDevice = nil

// SetDownstreamDevice sets the downstream device that handles all memory read/write requests.
func SetDownstreamDevice(device snes.AutoCloseableDevice) {
	downstreamDevice = device
}

type Client struct {
	c *net.TCPConn
}

func (c *Client) FormMessage(block *LuaBlock) (b []byte, err error) {
	b, err = json.Marshal(block)
	if err != nil {
		return
	}
	// protocol requires 0 termination:
	b = append(b, 0)
	return
}

func (c *Client) WriteMessage(ctx context.Context, write *LuaBlock) (n int, err error) {
	var b []byte
	b, err = c.FormMessage(write)
	if err != nil {
		return
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	err = c.c.SetWriteDeadline(deadline)
	if err != nil {
		return
	}

	n, err = c.c.Write(b)
	if err != nil {
		return
	}
	return
}

func (c *Client) handleRead() {
	var err error
	defer func() {
		log.Printf("connectorlib: %s\n", err)
		_ = c.c.Close()
	}()

	br := bufio.NewReaderSize(c.c, 65536)
	jd := json.NewDecoder(br)
	for {
		var req LuaBlock
		err = jd.Decode(&req)
		if err != nil {
			return
		}
		var zero byte
		zero, err = br.ReadByte()
		if err != nil {
			return
		}
		if zero != 0 {
			err = fmt.Errorf("connectorlib: expected $00 byte after json message; got $%02x", zero)
			return
		}

		// create response:
		rsp := req
		rsp.Stamp = time.Now()
		rsp.Message = ""

		if req.Type == 0xFF {
			// ping-pong
		} else if req.Type == 0xF0 {
			// show message
			log.Printf("connectorlib: %s\n", req.Message)
		} else {
			// memory read/write ops:
			if downstreamDevice == nil {
				log.Printf("connectorlib: cannot service request; target device not selected\n")
			} else {
				switch req.Type {
				case 0x00: // read_u8
					if req.Domain == "System Bus" {
						downstreamDevice.MultiReadMemory(
							context.Background(),
							snes.MemoryReadRequest{
								RequestAddress: snes.AddressTuple{
									Address:       uint32(req.Address),
									AddressSpace:  sni.AddressSpace_SnesABus,
									MemoryMapping: sni.MemoryMapping_Unknown,
								},
								Size: 1,
							})
					}
				}
			}
		}

		_, err = c.WriteMessage(context.Background(), &rsp)
		if err != nil {
			return
		}
	}
}
