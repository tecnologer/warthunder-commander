package player

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// PlayFile plays an MP3 via an available audio player at the given volume
// (0–200, 100=normal), blocking until done.
func PlayFile(path string, volume int) error {
	vol := strconv.Itoa(volume)

	// Try common cross-platform players first.
	players := []struct {
		name string
		args []string
	}{
		{"mpv", []string{"--really-quiet", "--volume=" + vol, path}},
		{"mplayer", []string{"-really-quiet", "-volume", vol, path}},
		{"ffplay", []string{"-nodisp", "-autoexit", "-loglevel", "quiet", path}},
		{"vlc", []string{"--intf", "dummy", "--play-and-exit", path}},
	}
	for _, p := range players {
		if bin, err := exec.LookPath(p.name); err == nil {
			if err := exec.CommandContext(context.Background(), bin, p.args...).Run(); err != nil {
				return fmt.Errorf("player: %s: %w", p.name, err)
			}

			return nil
		}
	}

	if runtime.GOOS == "windows" {
		return playFileWindows(path)
	}

	return fmt.Errorf("tts: no audio player found (install mpv, mplayer, or ffplay)")
}

// playFileWindows plays an MP3 on Windows using the MCI API via PowerShell.
// This is always available on Windows without any extra software.
func playFileWindows(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// Use backslashes and escape any single quotes in the path.
	abs = strings.ReplaceAll(filepath.FromSlash(abs), "'", "''")

	// PowerShell script using MCI API. Backtick is PowerShell's escape char;
	// we build the string without Go raw literals to avoid the conflict.
	script := "$sig = @'\n" +
		"using System;\n" +
		"using System.Runtime.InteropServices;\n" +
		"using System.Text;\n" +
		"public class Mci {\n" +
		"    [DllImport(\"winmm.dll\", CharSet = CharSet.Auto)]\n" +
		"    public static extern int mciSendString(string cmd, StringBuilder ret, int len, IntPtr hwnd);\n" +
		"}\n" +
		"'@\n" +
		"Add-Type -TypeDefinition $sig -ErrorAction SilentlyContinue\n" +
		"$f = '" + abs + "'\n" +
		"[Mci]::mciSendString(\"open `\"$f`\" type mpegvideo alias media\", $null, 0, [IntPtr]::Zero) | Out-Null\n" +
		"[Mci]::mciSendString(\"play media wait\", $null, 0, [IntPtr]::Zero) | Out-Null\n" +
		"[Mci]::mciSendString(\"close media\", $null, 0, [IntPtr]::Zero) | Out-Null\n"

	return exec.CommandContext(context.Background(), "powershell", "-NonInteractive", "-NoProfile", "-c", script).Run() //nolint:wrapcheck
}
