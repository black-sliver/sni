package connectorlib

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sni/snes/services/connectorlib"
	"sync"
	"sync/atomic"
	"time"
)

type Device struct {
	lock sync.Mutex
	c    *net.TCPConn
	br   *bufio.Reader
	jd   *json.Decoder

	deviceKey string

	isClosed bool

	clientName string

	nextID uint32
}

func NewDevice(conn *net.TCPConn, key string) *Device {
	d := &Device{
		c:          conn,
		deviceKey:  key,
		clientName: conn.RemoteAddr().String(),
		isClosed:   false,
	}
	d.br = bufio.NewReader(d.c)
	d.jd = json.NewDecoder(d.br)

	return d
}

func (d *Device) Close() (err error) {
	if d.isClosed {
		return nil
	}

	d.isClosed = true
	err = d.c.Close()

	// remove device from driver:
	driver.DeleteDevice(d.deviceKey)

	return
}

func (d *Device) IsClosed() bool { return d.isClosed }

func (d *Device) Init() {
	go d.initConnection()
}

func (d *Device) initConnection() {
	var err error
	defer func() {
		if err != nil {
			log.Printf("connectorlib: %v\n", err)
			err := d.Close()
			if err != nil {
				log.Printf("connectorlib: close error: %v\n", err)
				return
			}
		}
	}()

	_ = d.c.SetNoDelay(true)

	log.Printf("connectorlib: client '%s'\n", d.clientName)

	for {
		// every 2 seconds, check if connection is closed; only way to do this reliably is to read data:
		time.Sleep(time.Second * 2)

		err = d.Ping(context.Background())
		if err != nil {
			return
		}
	}
}

func (d *Device) FormMessage(block *connectorlib.LuaBlock) (b []byte, err error) {
	b, err = json.Marshal(block)
	if err != nil {
		return
	}
	// protocol requires 0 termination:
	b = append(b, 0)
	return
}

func (d *Device) WriteMessage(ctx context.Context, write *connectorlib.LuaBlock) (n int, err error) {
	var b []byte
	b, err = d.FormMessage(write)
	if err != nil {
		return
	}

	defer d.lock.Unlock()
	d.lock.Lock()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	err = d.c.SetWriteDeadline(deadline)
	if err != nil {
		_ = d.Close()
		return
	}

	n, err = d.c.Write(b)
	if err != nil {
		_ = d.Close()
	}
	return
}

func (d *Device) readMessage(read *connectorlib.LuaBlock) (err error) {
	err = d.jd.Decode(read)
	if err != nil {
		_ = d.Close()
		return
	}

	var zero byte
	zero, err = d.br.ReadByte()
	if zero != 0 {
		err = fmt.Errorf("connectorlib: expected $00 byte after json message but got $%02x", zero)
		_ = d.Close()
		return
	}

	return
}

func (d *Device) ReadMessage(ctx context.Context, read *connectorlib.LuaBlock) (err error) {
	defer d.lock.Unlock()
	d.lock.Lock()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	err = d.c.SetReadDeadline(deadline)
	if err != nil {
		_ = d.Close()
		return
	}

	err = d.readMessage(read)
	return
}

func (d *Device) WriteThenRead(ctx context.Context, write *connectorlib.LuaBlock, read *connectorlib.LuaBlock) (err error) {
	var b []byte
	b, err = d.FormMessage(write)
	if err != nil {
		return
	}

	defer d.lock.Unlock()
	d.lock.Lock()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	err = d.c.SetWriteDeadline(deadline)
	if err != nil {
		return
	}

	_, err = d.c.Write(b)
	if err != nil {
		return
	}

	err = d.c.SetReadDeadline(deadline)
	if err != nil {
		return
	}

	err = d.readMessage(read)
	return
}

func (d *Device) Ping(ctx context.Context) (err error) {
	var response connectorlib.LuaBlock
	request := &connectorlib.LuaBlock{
		ID:      d.NextID(),
		Stamp:   time.Time{},
		Type:    0xFF,
		Message: "",
		Address: 0,
		Domain:  "",
		Value:   0,
		Block:   nil,
	}

	err = d.WriteThenRead(
		ctx,
		request,
		&response,
	)
	if err != nil {
		return
	}

	if response.ID != request.ID {
		err = fmt.Errorf("connectorlib: response ID %d != request ID %d", response.ID, request.ID)
	}
	return
}

func (d *Device) NextID() uint32 {
	return atomic.AddUint32(&d.nextID, 1)
}
