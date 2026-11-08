//go:build windows

package logontask

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Name is the task of the copy of Poros whose core is at core: every portable copy has its own.
func Name(core string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(filepath.Dir(core)))))
	return "Poros core at sign-in " + hex.EncodeToString(sum[:4])
}

func shellOf(core string) string {
	return filepath.Join(filepath.Dir(core), "Poros.exe")
}

// Register creates or replaces the task of this copy. It takes administrator rights.
func Register(core, dataDir string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	windowsDir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return err
	}
	// Not in the data folder: another program of the user could change the file there before the Task Scheduler
	// reads it. A file an administrator makes in the Temp folder of Windows is closed to the user.
	file, err := os.CreateTemp(filepath.Join(windowsDir, "Temp"), "poros-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(utf16File(definition(user.User.Sid.String(), core, dataDir)))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = schtasks("/Create", "/TN", Name(core), "/XML", file.Name(), "/F"); err != nil {
		return err
	}
	clearRemoval(dataDir)
	return nil
}

// Remove deletes the task of this copy; one that is not there is fine. It takes administrator rights.
func Remove(core, dataDir string) error {
	if schtasks("/Query", "/TN", Name(core)) == nil {
		if err := schtasks("/Delete", "/TN", Name(core), "/F"); err != nil {
			return err
		}
	}
	clearRemoval(dataDir)
	return nil
}

// AutostartWaits tells whether the autostart of this copy starts a shell that waits for the core of the task.
func AutostartWaits(core string) bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	command, _, err := key.GetStringValue("Poros")
	return err == nil && waits(command, shellOf(core))
}

func waits(command, shell string) bool {
	command = strings.ToLower(command)
	return strings.HasPrefix(command, strings.ToLower(`"`+shell+`"`)) && strings.Contains(command, " "+ShellFlag)
}

// VerifyShell makes sure that the channel is served by the shell of this copy: another program of the user could
// have left the file to get a core with rights for itself.
func VerifyShell(conn net.Conn, hostPID int, core string) error {
	file, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("the channel has no handle")
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(file.Fd()), &pid); err != nil {
		return fmt.Errorf("the process of the channel: %w", err)
	}
	if int(pid) != hostPID {
		return fmt.Errorf("the channel belongs to process %d, not to the shell %d", pid, hostPID)
	}
	image, err := processImage(pid)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(image), filepath.Clean(shellOf(core))) {
		return fmt.Errorf("the channel belongs to %s", image)
	}
	return nil
}

func processImage(pid uint32) (string, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", fmt.Errorf("the program of process %d: %w", pid, err)
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

func schtasks(args ...string) error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(system, "schtasks.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}

func escape(text string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(text))
	return b.String()
}

// definition is the task: at the user's sign-in, the core with the highest rights, for as long as the session lasts,
// on battery too. It cannot be started by hand, so it is no way to get rights at any other time.
func definition(sid, core, dataDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>Poros</Author>
    <Description>Starts the core of Poros in %[3]s with administrator rights when you sign in, so that TUN and the kill switch need no UAC prompt. Poros removes the task when its setting goes off.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%[1]s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%[1]s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>false</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>5</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%[2]s</Command>
      <Arguments>%[4]s</Arguments>
      <WorkingDirectory>%[3]s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`, escape(sid), escape(core), escape(filepath.Dir(core)), escape("--logon --data-dir "+syscall.EscapeArg(dataDir)))
}

// utf16File is the text as the Task Scheduler reads a definition: UTF-16 with its byte order mark.
func utf16File(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, 2+2*len(units))
	data[0], data[1] = 0xFF, 0xFE
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[2+2*i:], unit)
	}
	return data
}
