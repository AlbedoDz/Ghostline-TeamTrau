package startup

import (
	_ "embed"

	"github.com/hashcott/ghostline/internal/brand"
)

// GuardScript restores DNS and the system proxy from state.json using only
// Windows built-ins. Every other recovery layer runs ghostline.exe, and an
// antivirus that flags it kills the watchdog, quarantines the exe and
// deletes the tasks that point at it, all at once.
//
//go:embed guard.ps1
var GuardScript []byte

// Guard is the network guard task: guard.ps1 run as SYSTEM.
type Guard struct {
	PowerShell string // powershell.exe
	Script     string // guard.ps1, in an admin-only directory: it runs as SYSTEM
	State      string // state.json
	UserSID    string // whose system proxy to restore
	Log        string
}

// GuardTaskXML runs the guard every minute, at boot, and shortly after
// Microsoft Defender acts on a threat. SYSTEM runs it without a window.
func GuardTaskXML(g Guard) string {
	args := `-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "` + g.Script + `" -State "` + g.State +
		`" -UserSid "` + g.UserSID + `" -TaskName "` + brand.TaskGuard + `" -Log "` + g.Log + `"`
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>` + esc(brand.AppName) + `: restores DNS if Ghostline stops without restoring it</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <Repetition>
        <Interval>PT1M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
      <StartBoundary>2000-01-01T00:00:00</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
    <BootTrigger>
      <Enabled>true</Enabled>
    </BootTrigger>
    <EventTrigger>
      <Enabled>true</Enabled>
      <Delay>PT10S</Delay>
      <Subscription>` + esc(`<QueryList><Query Id="0" Path="Microsoft-Windows-Windows Defender/Operational"><Select Path="Microsoft-Windows-Windows Defender/Operational">*[System[(EventID=1116 or EventID=1117)]]</Select></Query></QueryList>`) + `</Subscription>
    </EventTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-18</UserId>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <ExecutionTimeLimit>PT2M</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + esc(g.PowerShell) + `</Command>
      <Arguments>` + esc(args) + `</Arguments>
    </Exec>
  </Actions>
</Task>
`
}
