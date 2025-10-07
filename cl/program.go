package cl

// #include <stdlib.h>
// #include "cl.h"
import "C"

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"unsafe"
)

type BuildError string

func (e BuildError) Error() string {
	return fmt.Sprintf("cl: build error (%s)", string(e))
}

type Program struct {
	clProgram C.cl_program
	devices   []*Device
}

func releaseProgram(p *Program) {
	if p.clProgram != nil {
		C.clReleaseProgram(p.clProgram)
		p.clProgram = nil
	}
	p.devices = nil
}

func (p *Program) Release() {
	releaseProgram(p)
}

func (p *Program) BuildProgram(devices []*Device, options string) error {
	var cOptions *C.char
	if options != "" {
		cOptions = C.CString(options)
		defer C.free(unsafe.Pointer(cOptions))
	}
	var deviceList []C.cl_device_id
	var deviceListPtr *C.cl_device_id
	numDevices := C.cl_uint(len(devices))
	if len(devices) > 0 {
		deviceList = buildDeviceIdList(devices)
		deviceListPtr = &deviceList[0]
	}
	if errCode := C.clBuildProgram(p.clProgram, numDevices, deviceListPtr, cOptions, nil, nil); errCode != C.CL_SUCCESS {
		if buildErr := p.wrapBuildError(errCode, devices); buildErr != nil {
			return buildErr
		}
		return toError(errCode)
	}
	if len(devices) > 0 {
		p.devices = append([]*Device(nil), devices...)
	} else if len(p.devices) == 0 {
		if progDevices, err := p.associatedDevices(); err == nil && len(progDevices) > 0 {
			p.devices = progDevices
		}
	}
	return nil
}

func (p *Program) CreateKernel(name string) (*Kernel, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	var err C.cl_int
	clKernel := C.clCreateKernel(p.clProgram, cName, &err)
	if err != C.CL_SUCCESS {
		return nil, toError(err)
	}
	kernel := &Kernel{clKernel: clKernel, name: name}
	runtime.SetFinalizer(kernel, releaseKernel)
	return kernel, nil
}

func (p *Program) GetBuildLog(device *Device) (string, error) {
	if p == nil || p.clProgram == nil {
		return "", ErrInvalidProgram
	}
	if device == nil {
		return "", ErrInvalidDevice
	}
	var size C.size_t
	if err := C.clGetProgramBuildInfo(p.clProgram, device.id, C.CL_PROGRAM_BUILD_LOG, 0, nil, &size); err != C.CL_SUCCESS {
		return "", toError(err)
	}
	if size == 0 {
		return "", nil
	}
	buf := make([]byte, int(size))
	if len(buf) == 0 {
		return "", nil
	}
	if err := C.clGetProgramBuildInfo(p.clProgram, device.id, C.CL_PROGRAM_BUILD_LOG, size, unsafe.Pointer(&buf[0]), nil); err != C.CL_SUCCESS {
		return "", toError(err)
	}
	if idx := bytes.IndexByte(buf, 0); idx >= 0 {
		buf = buf[:idx]
	}
	return string(buf), nil
}

func (p *Program) wrapBuildError(code C.cl_int, requested []*Device) error {
	devices, err := p.collectDevicesForLogs(requested)
	if err != nil {
		return fmt.Errorf("cl: build error (%v; log unavailable: %v)", toError(code), err)
	}
	logs := p.collectBuildLogs(devices)
	if len(logs) == 0 {
		return toError(code)
	}
	status := fmt.Sprintf("status=%v", toError(code))
	logs = append([]string{status}, logs...)
	return BuildError(strings.Join(logs, "\n"))
}

func (p *Program) collectDevicesForLogs(requested []*Device) ([]*Device, error) {
	seen := make(map[uintptr]struct{})
	var list []*Device
	add := func(devs []*Device) {
		for _, d := range devs {
			key := deviceIDKey(d)
			if key == 0 {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			list = append(list, d)
		}
	}
	add(requested)
	add(p.devices)
	if len(list) == 0 {
		if progDevices, err := p.associatedDevices(); err == nil {
			add(progDevices)
		} else {
			return nil, err
		}
	}
	return list, nil
}

func (p *Program) collectBuildLogs(devices []*Device) []string {
	logs := make([]string, 0, len(devices))
	for _, dev := range devices {
		label := safeDeviceLabel(dev)
		log, err := p.GetBuildLog(dev)
		if err != nil {
			logs = append(logs, fmt.Sprintf("%s: <unable to fetch build log: %v>", label, err))
			continue
		}
		log = strings.TrimSpace(log)
		if log == "" {
			continue
		}
		logs = append(logs, fmt.Sprintf("%s:\n%s", label, log))
	}
	return logs
}

func (p *Program) associatedDevices() ([]*Device, error) {
	if p == nil || p.clProgram == nil {
		return nil, ErrInvalidProgram
	}
	var size C.size_t
	if err := C.clGetProgramInfo(p.clProgram, C.CL_PROGRAM_DEVICES, 0, nil, &size); err != C.CL_SUCCESS {
		return nil, toError(err)
	}
	if size == 0 {
		return nil, nil
	}
	var sample C.cl_device_id
	elemSize := C.size_t(unsafe.Sizeof(sample))
	if elemSize == 0 {
		return nil, ErrUnknown
	}
	count := int(size / elemSize)
	if count <= 0 {
		return nil, nil
	}
	ids := make([]C.cl_device_id, count)
	if err := C.clGetProgramInfo(p.clProgram, C.CL_PROGRAM_DEVICES, size, unsafe.Pointer(&ids[0]), nil); err != C.CL_SUCCESS {
		return nil, toError(err)
	}
	devices := make([]*Device, 0, count)
	for _, id := range ids {
		if id == nil {
			continue
		}
		devices = append(devices, &Device{id: id})
	}
	return devices, nil
}

func deviceIDKey(d *Device) uintptr {
	if d == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(d.id))
}

func safeDeviceLabel(d *Device) string {
	if d == nil {
		return "device<nil>"
	}
	key := deviceIDKey(d)
	label := fmt.Sprintf("device@0x%x", key)
	defer func() {
		if recover() != nil {
			label = fmt.Sprintf("device@0x%x", key)
		}
	}()
	if name := strings.TrimSpace(d.Name()); name != "" {
		label = name
	}
	return label
}
