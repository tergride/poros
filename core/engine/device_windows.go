package engine

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const osName = "Windows"

func osVersion() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}
