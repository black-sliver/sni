package luabridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sni/protos/sni"
	"sni/snes"
	"sni/snes/mapping"
	"time"
)

const readWriteTimeout = time.Second * 15

func (d *Device) MultiReadMemory(ctx context.Context, reads ...snes.MemoryReadRequest) (rsp []snes.MemoryReadResponse, err error) {
	defer func() {
		if err != nil {
			rsp = nil
			closeErr := d.Close()
			if closeErr != nil {
				log.Printf("luabridge: close error: %v\n", closeErr)
			}
		}
	}()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(readWriteTimeout)
	}

	rsp = make([]snes.MemoryReadResponse, len(reads))
	for j, read := range reads {
		addr := mapping.TranslateAddress(
			read.RequestAddress,
			read.RequestAddressSpace,
			d.Mapping,
			sni.AddressSpace_SnesABus,
		)

		sb := bytes.NewBuffer(make([]byte, 0, 64))
		_, _ = fmt.Fprintf(sb, "Read|%d|%d\n", addr, read.Size)

		data := make([]byte, 65536)
		var n int
		n, err = d.WriteThenRead(sb.Bytes(), data, deadline)
		if err != nil {
			return
		}

		// parse response as json:
		type tmpResultJson struct {
			Data []byte `json:"data"`
		}
		tmp := tmpResultJson{}
		err = json.Unmarshal(data[:n], &tmp)
		if err != nil {
			return
		}
		if actual, expected := len(tmp.Data), read.Size; actual != expected {
			err = fmt.Errorf("response did not provide enough data to meet request size; actual $%x, expected $%x", actual, expected)
			return
		}

		rsp[j] = snes.MemoryReadResponse{
			MemoryReadRequest:  read,
			DeviceAddress:      addr,
			DeviceAddressSpace: sni.AddressSpace_SnesABus,
			Data:               tmp.Data,
		}
	}

	return
}

func (d *Device) MultiWriteMemory(ctx context.Context, writes ...snes.MemoryWriteRequest) (rsp []snes.MemoryWriteResponse, err error) {
	defer func() {
		if err != nil {
			rsp = nil
			closeErr := d.Close()
			if closeErr != nil {
				log.Printf("luabridge: close error: %v\n", closeErr)
			}
		}
	}()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(readWriteTimeout)
	}

	rsp = make([]snes.MemoryWriteResponse, len(writes))
	for j, write := range writes {
		addr := mapping.TranslateAddress(
			write.RequestAddress,
			write.RequestAddressSpace,
			d.Mapping,
			sni.AddressSpace_SnesABus,
		)

		// preallocate enough space to write the whole command:
		sb := bytes.NewBuffer(make([]byte, 0, 24 + 4*len(write.Data)))
		_, _ = fmt.Fprintf(sb, "Write|%d", addr)
		for _, b := range write.Data {
			_, _ = fmt.Fprintf(sb, "|%d", b)
		}
		sb.WriteByte('\n')

		// send the command:
		var n int
		n, err = d.WriteDeadline(sb.Bytes(), deadline)
		if err != nil {
			return
		}
		_ = n

		rsp[j] = snes.MemoryWriteResponse{
			RequestAddress:      write.RequestAddress,
			RequestAddressSpace: write.RequestAddressSpace,
			DeviceAddress:       addr,
			DeviceAddressSpace:  sni.AddressSpace_SnesABus,
			Size:                len(write.Data),
		}
	}

	return
}