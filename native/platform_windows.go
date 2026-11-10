package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protectData(data []byte) ([]byte, error) {
	return dpapi(data, false)
}

func unprotectData(data []byte) ([]byte, error) {
	return dpapi(data, true)
}

func dpapi(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty credentials")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}

func protectDirectory(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func lockState(dir string) (func(), error) {
	hash := sha256.Sum256([]byte(dir))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`Local\M365MCP-%x`, hash[:16]))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return nil, errors.New("another M365 MCP process is using this account; stop it first")
	}
	return func() { windows.CloseHandle(handle) }, nil
}

func openBrowser(address string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	url, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, url, nil, nil, windows.SW_SHOWNORMAL)
}

func attachWorkerLimits(pid int) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	closeJob := func() { windows.CloseHandle(job) }
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
		windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY | windows.JOB_OBJECT_LIMIT_PROCESS_TIME |
		windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	limits.ProcessMemoryLimit = 512 << 20
	limits.BasicLimitInformation.PerProcessUserTimeLimit = 15 * 10_000_000
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err != nil {
		closeJob()
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		closeJob()
		return nil, err
	}
	defer windows.CloseHandle(process)
	if err = windows.AssignProcessToJobObject(job, process); err != nil {
		closeJob()
		return nil, err
	}
	return closeJob, nil
}
