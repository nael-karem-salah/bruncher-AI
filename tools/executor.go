package tools

import (
"bytes"
"fmt"
"image/png"
"os"
"os/exec"
"path/filepath"
"syscall"
"time"

"github.com/kbinani/screenshot"
)

var (
user32           = syscall.NewLazyDLL("user32.dll")
procSetCursorPos = user32.NewProc("SetCursorPos")
procMouseEvent   = user32.NewProc("mouse_event")
)

const (
MOUSEEVENTF_LEFTDOWN  = 0x0002
MOUSEEVENTF_LEFTUP    = 0x0004
MOUSEEVENTF_RIGHTDOWN = 0x0008
MOUSEEVENTF_RIGHTUP   = 0x0010
)

type ToolExecutor struct{}

type ActionExecutor = ToolExecutor

func NewToolExecutor() *ToolExecutor {
return &ToolExecutor{}
}

func NewActionExecutor() *ToolExecutor {
return NewToolExecutor()
}

func (e *ToolExecutor) Execute(action string, params map[string]interface{}) (string, error) {
switch action {
case "powershell":
cmdStr, _ := params["command"].(string)
return e.runPowerShell(cmdStr)

case "move_mouse":
x := int(params["x"].(float64))
y := int(params["y"].(float64))
procSetCursorPos.Call(uintptr(x), uintptr(y))
return fmt.Sprintf("Moved mouse to (%d, %d)", x, y), nil

case "click":
button, _ := params["button"].(string)
x, hasX := params["x"].(float64)
y, hasY := params["y"].(float64)

if hasX && hasY {
procSetCursorPos.Call(uintptr(int(x)), uintptr(int(y)))
}

if button == "right" {
procMouseEvent.Call(MOUSEEVENTF_RIGHTDOWN, 0, 0, 0, 0)
time.Sleep(50 * time.Millisecond)
procMouseEvent.Call(MOUSEEVENTF_RIGHTUP, 0, 0, 0, 0)
} else {
procMouseEvent.Call(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
time.Sleep(50 * time.Millisecond)
procMouseEvent.Call(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
}
return "Click performed", nil

case "type_text":
text, _ := params["text"].(string)
psCmd := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('%s')`, text)
return e.runPowerShell(psCmd)

case "take_screenshot":
n := screenshot.NumActiveDisplays()
if n <= 0 {
return "", fmt.Errorf("no active displays found")
}
bounds := screenshot.GetDisplayBounds(0)
img, err := screenshot.CaptureRect(bounds)
if err != nil {
return "", err
}

filePath := filepath.Join(os.TempDir(), "win_agent_screenshot.png")
file, err := os.Create(filePath)
if err != nil {
return "", err
}
defer file.Close()

if err := png.Encode(file, img); err != nil {
return "", err
}
return fmt.Sprintf("Screenshot saved to %s", filePath), nil

default:
return "", fmt.Errorf("unknown action: %s", action)
}
}

func (e *ToolExecutor) runPowerShell(command string) (string, error) {
cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command)
var out bytes.Buffer
var stderr bytes.Buffer
cmd.Stdout = &out
cmd.Stderr = &stderr

err := cmd.Run()
if err != nil {
return "", fmt.Errorf("%s", stderr.String())
}
return out.String(), nil
}
