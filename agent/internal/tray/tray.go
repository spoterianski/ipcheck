// Package tray shows the overall status in the menu bar (macOS) / system tray.
package tray

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"runtime"

	"fyne.io/systray"

	"ipcheck/internal/agent"
	"ipcheck/internal/checker"
)

var (
	iconGray  = dot(color.RGBA{0x9c, 0xa3, 0xaf, 0xff})
	iconGreen = dot(color.RGBA{0x22, 0xc5, 0x5e, 0xff})
	iconRed   = dot(color.RGBA{0xef, 0x44, 0x44, 0xff})
)

// Quit closes the tray and makes Run return.
func Quit() { systray.Quit() }

// Run blocks on the main thread until the user picks "Quit".
func Run(a *agent.Agent, groups int, dashboardURL string, onExit func()) {
	systray.Run(func() { onReady(a, groups, dashboardURL) }, onExit)
}

func onReady(a *agent.Agent, groups int, dashboardURL string) {
	systray.SetIcon(iconGray)
	systray.SetTooltip("IP Check")

	header := systray.AddMenuItem("Проверка…", "")
	header.Disable()
	systray.AddSeparator()
	items := make([]*systray.MenuItem, groups)
	for i := range items {
		items[i] = systray.AddMenuItem("…", "")
	}
	systray.AddSeparator()
	mCheck := systray.AddMenuItem("Проверить сейчас", "")
	mOpen := systray.AddMenuItem("Открыть дашборд", dashboardURL)
	mQuit := systray.AddMenuItem("Выход", "")

	render := func() {
		s := a.Snapshot()
		switch {
		case len(s.Results) == 0:
			return
		case s.OK:
			systray.SetIcon(iconGreen)
			systray.SetTitle("")
			systray.SetTooltip("IP Check: всё в порядке")
			header.SetTitle("🟢 Все маршруты в порядке · " + s.Updated.Format("15:04:05"))
		default:
			systray.SetIcon(iconRed)
			systray.SetTitle(fmt.Sprint(s.Problems))
			systray.SetTooltip(fmt.Sprintf("IP Check: проблем %d", s.Problems))
			header.SetTitle(fmt.Sprintf("🔴 Проблем: %d · %s", s.Problems, s.Updated.Format("15:04:05")))
		}
		for i, r := range s.Results {
			if i < len(items) {
				items[i].SetTitle(itemTitle(r))
				items[i].SetTooltip(r.Message)
			}
		}
	}

	ch, _ := a.Subscribe()
	render()
	go func() {
		for {
			select {
			case <-ch:
				render()
			case <-mCheck.ClickedCh:
				a.CheckNow()
			case <-mOpen.ClickedCh:
				openURL(dashboardURL)
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func itemTitle(r checker.Result) string {
	mark := "🟢"
	if r.Status != checker.OK {
		mark = "🔴"
	}
	if r.IP == "" {
		return fmt.Sprintf("%s %s — %s", mark, r.Group, agent.Describe(r.Status))
	}
	t := fmt.Sprintf("%s %s %s — %s %s", mark, checker.Flag(r.Country), r.Group, r.Exit, r.IP)
	if r.Exit == "" {
		t = fmt.Sprintf("%s ❔ %s — %s", mark, r.Group, r.IP)
	}
	if r.Status != checker.OK {
		t += fmt.Sprintf(" (ожидался %s %s)", checker.Flag(r.ExpectCountry), r.Expect)
	}
	return t
}

func openURL(u string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", u).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		exec.Command("xdg-open", u).Start()
	}
}

// dot renders a filled circle; Windows needs it wrapped into an .ico container.
func dot(c color.RGBA) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	r := float64(size)/2 - 4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-size/2+0.5, float64(y)-size/2+0.5
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	if runtime.GOOS != "windows" {
		return buf.Bytes()
	}
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, 1})
	ico.Write([]byte{size, size, 0, 0})
	binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})
	binary.Write(&ico, binary.LittleEndian, []uint32{uint32(buf.Len()), 22})
	ico.Write(buf.Bytes())
	return ico.Bytes()
}
