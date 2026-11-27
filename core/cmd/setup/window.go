//go:build windows

package main

import (
	"archive/zip"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"PorosCore/internal/release"
	"PorosCore/internal/wincom"
)

// The IDs of setup.rc.
const (
	iconApp        = 1
	dialogSetup    = 100
	idFolderLabel  = 101
	idFolder       = 102
	idBrowse       = 103
	idNote         = 104
	idDesktop      = 105
	idStartMenu    = 106
	idStatus       = 107
	idLaunch       = 108
	idUnpack       = 1 // IDOK
	idCancel       = 2 // IDCANCEL
	shortcutName   = "Poros.lnk"
	wmInitDialog   = 0x0110
	wmCommand      = 0x0111
	wmSetIcon      = 0x0080
	wmNextDlgCtl   = 0x0028
	wmUnpacked     = 0x8000 + 1 // WM_APP + 1
	bnClicked      = 0
	bstChecked     = 1
	imageIcon      = 1
	iconSmall      = 0
	iconBig        = 1
	smCxIcon       = 11
	smCyIcon       = 12
	smCxSmallIcon  = 49
	smCySmallIcon  = 50
	lrDefaultColor = 0
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procDialogBoxParamW      = user32.NewProc("DialogBoxParamW")
	procEndDialog            = user32.NewProc("EndDialog")
	procGetDlgItem           = user32.NewProc("GetDlgItem")
	procSetDlgItemTextW      = user32.NewProc("SetDlgItemTextW")
	procSetWindowTextW       = user32.NewProc("SetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procCheckDlgButton       = user32.NewProc("CheckDlgButton")
	procIsDlgButtonChecked   = user32.NewProc("IsDlgButtonChecked")
	procEnableWindow         = user32.NewProc("EnableWindow")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procLoadImageW           = user32.NewProc("LoadImageW")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
)

type window struct {
	archive *zip.Reader
	text    texts
	module  uintptr
	handle  uintptr
	// folder is where the archive goes, and done gets the result while it is unpacked there.
	folder string
	done   chan error
}

// showWindow shows the window until the user closes it or Poros is unpacked and started.
func showWindow(archive *zip.Reader, text texts) error {
	var module windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &module); err != nil {
		return err
	}
	w := &window{archive: archive, text: text, module: uintptr(module)}
	result, _, err := procDialogBoxParamW.Call(w.module, dialogSetup, 0, windows.NewCallback(w.procedure), 0)
	if int(result) == -1 {
		return err
	}
	return nil
}

func (w *window) procedure(handle, message, wparam, _ uintptr) uintptr {
	switch message {
	case wmInitDialog:
		w.handle = handle
		w.start()
		// The focus is on Unpack already.
		return 0
	case wmCommand:
		if wparam>>16&0xffff != bnClicked {
			return 0
		}
		switch wparam & 0xffff {
		case idBrowse:
			w.browse()
		case idUnpack:
			w.unpack()
		case idCancel:
			if w.done == nil {
				procEndDialog.Call(handle, 0)
			}
		default:
			return 0
		}
		return 1
	case wmUnpacked:
		w.finish(<-w.done)
		return 1
	}
	return 0
}

func (w *window) start() {
	title := "Poros"
	if version != "" {
		title += " " + version
	}
	procSetWindowTextW.Call(w.handle, uintptr(unsafe.Pointer(wide(title))))
	for id, text := range map[int]string{
		idFolderLabel: w.text.folder,
		idBrowse:      w.text.browse,
		idNote:        w.text.note,
		idDesktop:     w.text.desktop,
		idStartMenu:   w.text.startMenu,
		idLaunch:      w.text.launch,
		idUnpack:      w.text.unpack,
		idCancel:      w.text.cancel,
		idFolder:      defaultFolder(),
	} {
		w.setText(id, text)
	}
	procCheckDlgButton.Call(w.handle, idDesktop, bstChecked)
	procCheckDlgButton.Call(w.handle, idStartMenu, bstChecked)
	for kind, size := range map[uintptr][2]uintptr{iconBig: {smCxIcon, smCyIcon}, iconSmall: {smCxSmallIcon, smCySmallIcon}} {
		width, _, _ := procGetSystemMetrics.Call(size[0])
		height, _, _ := procGetSystemMetrics.Call(size[1])
		icon, _, _ := procLoadImageW.Call(w.module, iconApp, imageIcon, width, height, lrDefaultColor)
		procSendMessageW.Call(w.handle, wmSetIcon, kind, icon)
	}
	w.focusUnpack()
}

func (w *window) browse() {
	folder, err := pickFolder(w.handle, w.text.picker, pickerStart(w.itemText(idFolder)))
	if err != nil {
		w.show(err.Error(), windows.MB_ICONERROR)
	} else if folder != "" {
		w.setText(idFolder, pickedFolder(folder))
	}
}

func (w *window) unpack() {
	dir := strings.Trim(strings.TrimSpace(w.itemText(idFolder)), `"`)
	if !filepath.IsAbs(dir) {
		w.show(w.text.notAbsolute, windows.MB_ICONERROR)
		return
	}
	dir = filepath.Clean(dir)
	if running(dir) {
		w.show(w.text.running, windows.MB_ICONINFORMATION)
		return
	}
	if err := writable(dir); err != nil {
		w.show(fmt.Sprintf(w.text.notWritable, dir), windows.MB_ICONERROR)
		return
	}

	w.setBusy(true)
	w.folder = dir
	w.done = make(chan error, 1)
	go func(archive *zip.Reader, done chan<- error, handle uintptr) {
		done <- release.Install(archive, dir, lockWait)
		procPostMessageW.Call(handle, wmUnpacked, 0, 0)
	}(w.archive, w.done, w.handle)
}

func (w *window) finish(err error) {
	w.done = nil
	if err != nil {
		w.setBusy(false)
		w.show(fmt.Sprintf(w.text.failed, err), windows.MB_ICONERROR)
		w.focusUnpack()
		return
	}
	w.setText(idStatus, "")

	program := filepath.Join(w.folder, release.Program)
	for id, place := range map[int]*windows.KNOWNFOLDERID{idDesktop: windows.FOLDERID_Desktop, idStartMenu: windows.FOLDERID_Programs} {
		if !w.checked(id) {
			continue
		}
		folder, err := windows.KnownFolderPath(place, windows.KF_FLAG_DEFAULT)
		if err == nil {
			err = wincom.CreateShortcut(filepath.Join(folder, shortcutName), program, w.folder)
		}
		if err != nil {
			w.show(fmt.Sprintf(w.text.noShortcut, err), windows.MB_ICONWARNING)
		}
	}

	if w.checked(idLaunch) {
		start := exec.Command(program)
		start.Dir = w.folder
		if err := start.Start(); err != nil {
			w.show(fmt.Sprintf(w.text.notStarted, err), windows.MB_ICONERROR)
		} else {
			_ = start.Process.Release()
		}
	} else {
		w.show(fmt.Sprintf(w.text.unpacked, w.folder), windows.MB_ICONINFORMATION)
	}
	procEndDialog.Call(w.handle, 0)
}

func (w *window) checked(id int) bool {
	state, _, _ := procIsDlgButtonChecked.Call(w.handle, uintptr(id))
	return state == bstChecked
}

func (w *window) setBusy(busy bool) {
	enabled := uintptr(1)
	status := ""
	if busy {
		enabled, status = 0, w.text.unpacking
	}
	for _, id := range []int{idFolder, idBrowse, idDesktop, idStartMenu, idLaunch, idUnpack, idCancel} {
		item, _, _ := procGetDlgItem.Call(w.handle, uintptr(id))
		procEnableWindow.Call(item, enabled)
	}
	w.setText(idStatus, status)
}

func (w *window) focusUnpack() {
	item, _, _ := procGetDlgItem.Call(w.handle, idUnpack)
	procSendMessageW.Call(w.handle, wmNextDlgCtl, item, 1)
}

func (w *window) setText(id int, text string) {
	procSetDlgItemTextW.Call(w.handle, uintptr(id), uintptr(unsafe.Pointer(wide(text))))
}

func (w *window) itemText(id int) string {
	item, _, _ := procGetDlgItem.Call(w.handle, uintptr(id))
	length, _, _ := procGetWindowTextLengthW.Call(item)
	buffer := make([]uint16, length+1)
	procGetWindowTextW.Call(item, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return syscall.UTF16ToString(buffer)
}

func (w *window) show(text string, icon uint32) {
	showMessage(w.handle, text, icon)
}

func showMessage(owner uintptr, text string, icon uint32) {
	_, _ = windows.MessageBox(windows.HWND(owner), wide(text), wide("Poros"), windows.MB_OK|icon)
}
