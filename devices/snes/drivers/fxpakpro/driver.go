package fxpakpro

import (
	"fmt"
	"log"
	"net/url"
	"runtime"
	"sni/cmd/sni/config"
	"sni/devices"
	"sni/protos/sni"
	"strconv"
	"strings"
	"sync"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

const (
	driverName = "fxpakpro"
)

var driver *Driver

var baudRates = []int{
	921600, // first rate that works on Windows
	460800,
	256000,
	230400, // first rate that works on MacOS
	153600,
	128000,
	115200,
	76800,
	57600,
	38400,
	28800,
	19200,
	14400,
	9600,
}

const defaultAddressSpace = sni.AddressSpace_FxPakPro

type Driver struct {
	container devices.DeviceContainer
	enumLock  sync.Mutex
}

func (d *Driver) DisplayOrder() int {
	return 0
}

func (d *Driver) DisplayName() string {
	return "FX Pak Pro"
}

func (d *Driver) DisplayDescription() string {
	return "Connect to an FX Pak Pro or SD2SNES via USB"
}

func (d *Driver) Kind() string { return "fxpakpro" }

var driverCapabilities = []sni.DeviceCapability{
	sni.DeviceCapability_ReadMemory,
	sni.DeviceCapability_WriteMemory,
	sni.DeviceCapability_ResetSystem,
	sni.DeviceCapability_ResetToMenu,
	sni.DeviceCapability_ExecuteASM,
	sni.DeviceCapability_FetchFields,
	// filesystem:
	sni.DeviceCapability_ReadDirectory,
	sni.DeviceCapability_MakeDirectory,
	sni.DeviceCapability_RemoveFile,
	sni.DeviceCapability_RenameFile,
	sni.DeviceCapability_PutFile,
	sni.DeviceCapability_GetFile,
	sni.DeviceCapability_BootFile,
}

func (d *Driver) HasCapabilities(capabilities ...sni.DeviceCapability) (bool, error) {
	return devices.CheckCapabilities(capabilities, driverCapabilities)
}

func (d *Driver) DisconnectAll() {
	for _, deviceKey := range d.container.AllDeviceKeys() {
		device, ok := d.container.GetDevice(deviceKey)
		if ok {
			log.Printf("%s: disconnecting device '%s'\n", driverName, deviceKey)
			_ = device.Close()
			d.container.DeleteDevice(deviceKey)
		}
	}
}

func (d *Driver) Detect() (devs []devices.DeviceDescriptor, err error) {
	d.enumLock.Lock()
	defer d.enumLock.Unlock()

	var ports []*enumerator.PortDetails

	devs = make([]devices.DeviceDescriptor, 0, 2)

	ports, err = enumerator.GetDetailedPortsList()
	if err != nil {
		return
	}

	for _, port := range ports {
		if !port.IsUSB {
			continue
		}

		// When more than one fxpakpro is connected only one of the devices gets the SerialNumber="DEMO00000000";
		// This is likely a bug in serial library.
		if (port.SerialNumber == "DEMO00000000") || (port.VID == "1209" && port.PID == "5A22") {
			devs = append(devs, devices.DeviceDescriptor{
				Uri:                 url.URL{Scheme: driverName, Host: ".", Path: port.Name},
				DisplayName:         fmt.Sprintf("%s (%s:%s)", port.Name, port.VID, port.PID),
				Kind:                d.Kind(),
				Capabilities:        driverCapabilities[:],
				DefaultAddressSpace: defaultAddressSpace,
				System:              "snes",
			})
		}
	}

	err = nil
	return
}

func (d *Driver) openPort(portName string, baudRequest int) (f serial.Port, err error) {
	f = serial.Port(nil)

	// Try all the common baud rates in descending order:
	var baud int
	for _, baud = range baudRates {
		if baud > baudRequest {
			continue
		}

		log.Printf("%s: open(name=\"%s\", baud=%d)\n", driverName, portName, baud)
		f, err = serial.Open(portName, &serial.Mode{
			BaudRate: baud,
			DataBits: 8,
			Parity:   serial.NoParity,
			StopBits: serial.OneStopBit,
		})
		if err == nil {
			break
		}
		log.Printf("%s: open(name=\"%s\"): %v\n", driverName, portName, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: failed to open serial port at any baud rate: %w", driverName, err)
	}

	// set DTR:
	// log.Printf("serial: Set DTR on\n")
	if err = f.SetDTR(true); err != nil {
		// log.Printf("serial: %v\n", err)
		_ = f.Close()
		return nil, fmt.Errorf("%s: failed to set DTR: %w", driverName, err)
	}

	return
}

func (d *Driver) DeviceKey(uri *url.URL) (key string) {
	key = uri.Path
	// macos/linux paths:
	if strings.HasPrefix(key, "/dev/") {
		key = key[len("/dev/"):]
	}
	if strings.HasPrefix(key, "cu.usbmodem") {
		key = key[len("cu.usbmodem"):]
	}
	// macos   key should look like `DEMO000000001` with the final `1` suffix being the device index if multiple are connected.
	// windows key should look like `COM4` with the port number varying
	// linux   no idea what these devices look like yet, likely `/dev/...` possibly `ttyUSB0`?
	return
}

func (d *Driver) openDevice(uri *url.URL) (device devices.Device, err error) {
	portName := uri.Path

	var baudRequest int
	if runtime.GOOS == "darwin" {
		baudRequest = baudRates[3]
	} else {
		baudRequest = baudRates[0]
	}
	if baudStr := uri.Query().Get("baud"); baudStr != "" {
		baudRequest, _ = strconv.Atoi(baudStr)
	}

	var f serial.Port
	f, err = d.openPort(portName, baudRequest)
	if err != nil {
		return
	}

	dev := &Device{f: f}
	err = dev.Init()

	device = dev
	return
}

func (d *Driver) Device(uri *url.URL) devices.AutoCloseableDevice {
	return devices.NewAutoCloseableDevice(
		d.container,
		uri,
		d.DeviceKey(uri),
	)
}

var debugLog *log.Logger

func DriverInit() {
	if config.Config.GetBool("fxpakpro_disable") {
		log.Printf("Disabling fxpakpro snes driver\n")
		return
	}

	if config.Config.GetBool("debug") {
		log.Println("Debug Mode Active")
		defaultLogger := log.Default()
		debugLog = log.New(
			defaultLogger.Writer(),
			"fxpakpro: ",
			defaultLogger.Flags()|log.Lmsgprefix,
		)
	}

	log.Println("Enabling fxpakpro snes driver")

	driver = &Driver{}
	driver.container = devices.NewDeviceDriverContainer(driver.openDevice)
	devices.Register(driverName, driver)
}
