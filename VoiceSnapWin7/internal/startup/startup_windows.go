//go:build windows

package startup

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const registryKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
const appName = "VoiceSnap"

var (
	advapi32          = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW  = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueExW = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey    = advapi32.NewProc("RegCloseKey")
)

const (
	hkeyCurrentUser = 0x80000001
	keyQueryValue   = 0x0001
	keySetValue     = 0x0002
	regSZ           = 1
)

func isEnabled() bool {
	keyPath, _ := syscall.UTF16PtrFromString(registryKey)
	valueName, _ := syscall.UTF16PtrFromString(appName)

	var hKey syscall.Handle
	ret, _, _ := procRegOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(keyPath)),
		0,
		keyQueryValue,
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return false
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	// Query value size
	var dataType uint32
	var dataSize uint32
	ret, _, _ = procRegQueryValueExW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valueName)),
		0,
		uintptr(unsafe.Pointer(&dataType)),
		0,
		uintptr(unsafe.Pointer(&dataSize)),
	)
	return ret == 0 && dataSize > 0
}

func setEnabled(enable bool) error {
	keyPath, _ := syscall.UTF16PtrFromString(registryKey)
	valueName, _ := syscall.UTF16PtrFromString(appName)

	var hKey syscall.Handle
	ret, _, _ := procRegOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(keyPath)),
		0,
		keySetValue,
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return fmt.Errorf("failed to open registry key, error code: %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	if enable {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		val := fmt.Sprintf(`"%s"`, exe)
		valU, _ := syscall.UTF16FromString(val)
		dataSize := uint32(len(valU) * 2) // size in bytes
		ret, _, _ = procRegSetValueExW.Call(
			uintptr(hKey),
			uintptr(unsafe.Pointer(valueName)),
			0,
			regSZ,
			uintptr(unsafe.Pointer(&valU[0])),
			uintptr(dataSize),
		)
		if ret != 0 {
			return fmt.Errorf("failed to set registry value, error code: %d", ret)
		}
		return nil
	}

	// Delete value
	ret, _, _ = procRegDeleteValueW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valueName)),
	)
	// Ignore error if value doesn't exist
	return nil
}
