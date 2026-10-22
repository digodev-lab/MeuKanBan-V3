//go:build windows

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

// White noise is synthesized in-memory and streamed to the standard Windows
// waveform-audio API. No downloaded samples, internet, PowerShell, or extra DLL.
type waveFormatEx struct {
	formatTag      uint16
	channels       uint16
	sampleRate     uint32
	bytesPerSecond uint32
	blockAlign     uint16
	bitsPerSample  uint16
	extra          uint16
}
type waveHeader struct {
	data     uintptr
	length   uint32
	recorded uint32
	user     uintptr
	flags    uint32
	loops    uint32
	next     uintptr
	reserved uintptr
}
type noiseChunk struct {
	pcm    []int16
	header waveHeader
	ready  bool
}
type pomodoroPrefs struct {
	FocusMinutes int `json:"focus_minutes"`
	ShortMinutes int `json:"short_minutes"`
	LongMinutes  int `json:"long_minutes"`
	Volume       int `json:"white_noise_volume"`
	Sessions     int `json:"completed_focus_sessions"`
}

var (
	winmm          = syscall.NewLazyDLL("winmm.dll")
	pWaveOpen      = winmm.NewProc("waveOutOpen")
	pWavePrepare   = winmm.NewProc("waveOutPrepareHeader")
	pWaveWrite     = winmm.NewProc("waveOutWrite")
	pWaveUnprepare = winmm.NewProc("waveOutUnprepareHeader")
	pWaveReset     = winmm.NewProc("waveOutReset")
	pWaveClose     = winmm.NewProc("waveOutClose")
	pSetTimer      = u32.NewProc("SetTimer")
	pKillTimer     = u32.NewProc("KillTimer")

	pomPrefs           = pomodoroPrefs{FocusMinutes: 25, ShortMinutes: 5, LongMinutes: 15, Volume: 18}
	pomMode            = "focus"
	pomSeconds         = 25 * 60
	pomRunning         bool
	pomDeadline        time.Time
	pomSessionsInRound int
	pomStatus          = "Pronto para começar um período de concentração."

	noiseOn     bool
	waveDevice  uintptr
	noiseChunks []noiseChunk
	noiseSeed   uint32 = 0xA4C32F71
)

func pomConfigPath() string { return filepath.Join(appDir, "pomodoro.json") }
func loadPomPrefs() {
	b, err := os.ReadFile(pomConfigPath())
	if err == nil {
		var saved pomodoroPrefs
		if json.Unmarshal(b, &saved) == nil {
			if saved.FocusMinutes >= 1 && saved.FocusMinutes <= 120 {
				pomPrefs.FocusMinutes = saved.FocusMinutes
			}
			if saved.ShortMinutes >= 1 && saved.ShortMinutes <= 60 {
				pomPrefs.ShortMinutes = saved.ShortMinutes
			}
			if saved.LongMinutes >= 1 && saved.LongMinutes <= 90 {
				pomPrefs.LongMinutes = saved.LongMinutes
			}
			if saved.Volume >= 0 && saved.Volume <= 100 {
				pomPrefs.Volume = saved.Volume
			}
			if saved.Sessions >= 0 {
				pomPrefs.Sessions = saved.Sessions
			}
		}
	}
	pomSeconds = pomPrefs.FocusMinutes * 60
}
func savePomPrefs() {
	b, err := json.MarshalIndent(pomPrefs, "", "  ")
	if err != nil {
		return
	}
	tmp := pomConfigPath() + ".tmp"
	if os.WriteFile(tmp, b, 0644) == nil {
		_ = atomicReplaceFile(tmp, pomConfigPath())
	}
}
func pomDuration(mode string) int {
	switch mode {
	case "short":
		return pomPrefs.ShortMinutes * 60
	case "long":
		return pomPrefs.LongMinutes * 60
	default:
		return pomPrefs.FocusMinutes * 60
	}
}
func pomModeName() string {
	switch pomMode {
	case "short":
		return "PAUSA CURTA"
	case "long":
		return "PAUSA LONGA"
	default:
		return "HORA DE FOCAR"
	}
}
func pomSelect(mode string) {
	pomMode = mode
	pomRunning = false
	pomSeconds = pomDuration(mode)
	switch mode {
	case "short":
		pomStatus = "Respire e aproveite uma pausa rápida."
	case "long":
		pomStatus = "Faça uma pausa maior para renovar as energias."
	default:
		pomStatus = "Concentre-se em uma tarefa de cada vez."
	}
	invalidate()
}
func pomStartPause() {
	if pomRunning {
		pomSeconds = pomRemainingNow()
		pomRunning = false
		pomStatus = "Cronômetro pausado. Continue quando quiser."
	} else {
		if pomSeconds <= 0 {
			pomSeconds = pomDuration(pomMode)
		}
		pomDeadline = time.Now().Add(time.Duration(pomSeconds) * time.Second)
		pomRunning = true
		if pomMode == "focus" {
			pomStatus = "Foco ativo. Um passo de cada vez."
		} else {
			pomStatus = "Pausa em andamento. Relaxe um pouco."
		}
	}
	invalidate()
}
func pomRemainingNow() int {
	if !pomRunning {
		return pomSeconds
	}
	remaining := time.Until(pomDeadline)
	if remaining <= 0 {
		return 0
	}
	return int((remaining + time.Second - 1) / time.Second)
}
func pomComplete() {
	if pomMode == "focus" {
		pomPrefs.Sessions++
		pomSessionsInRound++
		savePomPrefs()
		if pomSessionsInRound%4 == 0 {
			pomMode = "long"
			pomStatus = "Quatro focos completos! Pausa longa iniciada."
		} else {
			pomMode = "short"
			pomStatus = "Foco concluído! Pausa curta iniciada."
		}
	} else {
		pomMode = "focus"
		pomStatus = "Pausa concluída! Um novo foco começou."
	}
	pomSeconds = pomDuration(pomMode)
	pomDeadline = time.Now().Add(time.Duration(pomSeconds) * time.Second)
	pomRunning = true // cycle automatically until the user pauses
	invalidate()
}
func pomTick() {
	// Audio must continue regardless of which page is shown.
	if noiseOn {
		noisePoll()
	}
	if pomRunning {
		r := pomRemainingNow()
		if r <= 0 {
			pomComplete()
			return
		}
		if r != pomSeconds {
			pomSeconds = r
			if page == "pomodoro" {
				invalidate()
			}
		}
	}
}
func pomAction(action string) {
	switch action {
	case "pomopen":
		page = "pomodoro"
		invalidate()
	case "pomtoggle":
		pomStartPause()
	case "pomreset":
		pomRunning = false
		pomSeconds = pomDuration(pomMode)
		pomStatus = "Cronômetro reiniciado."
		invalidate()
	case "pomskip":
		// Skipping is not counted as a completed focus session.
		if pomMode == "focus" {
			pomSelect("short")
		} else {
			pomSelect("focus")
		}
		pomStatus = "Etapa pulada. Aperte Iniciar quando quiser."
		invalidate()
	case "pomfocus":
		pomSelect("focus")
	case "pomshort":
		pomSelect("short")
	case "pomlong":
		pomSelect("long")
	case "noise":
		if noiseOn {
			noiseStop()
		} else if err := noiseStart(); err != nil {
			alert("Não foi possível iniciar o ruído branco. Verifique se há um dispositivo de áudio ativo.\n\n" + err.Error())
		}
		invalidate()
	case "volminus":
		pomPrefs.Volume -= 10
		if pomPrefs.Volume < 0 {
			pomPrefs.Volume = 0
		}
		savePomPrefs()
		invalidate()
	case "volplus":
		pomPrefs.Volume += 10
		if pomPrefs.Volume > 100 {
			pomPrefs.Volume = 100
		}
		savePomPrefs()
		invalidate()
	case "focusminus", "focusplus", "shortminus", "shortplus", "longminus", "longplus":
		delta := 1
		if action == "focusminus" || action == "shortminus" || action == "longminus" {
			delta = -1
		}
		switch {
		case len(action) >= 5 && action[:5] == "focus":
			pomPrefs.FocusMinutes = clampPom(pomPrefs.FocusMinutes+delta, 1, 120)
		case len(action) >= 5 && action[:5] == "short":
			pomPrefs.ShortMinutes = clampPom(pomPrefs.ShortMinutes+delta, 1, 60)
		case len(action) >= 4 && action[:4] == "long":
			pomPrefs.LongMinutes = clampPom(pomPrefs.LongMinutes+delta, 1, 90)
		}
		if !pomRunning {
			pomSeconds = pomDuration(pomMode)
		}
		savePomPrefs()
		invalidate()
	}
}
func clampPom(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func drawPomodoro(dc uintptr) {
	x := int32(264)
	width := winW - x - 30
	label(dc, "POMODORO / FOCUS ROOM", x, 29, width, 28, rgb(73, 79, 111), fontSmall)
	label(dc, "Tempo de foco com um visual mais calmo.", x, 60, width, 46, rgb(28, 31, 43), fontTitle)
	label(dc, "Cronômetro, pausas e som ambiente suave em um painel mais equilibrado.", x, 106, width, 30, rgb(90, 96, 126), fontSmall)
	gap := int32(16)
	leftW := width * 58 / 100
	rightW := width - leftW - gap
	if rightW < 320 {
		rightW = 320
		leftW = width - rightW - gap
	}
	rightX := x + leftW + gap
	top := int32(150)
	bottom := winH - 42
	leftH := bottom - top

	rounded(dc, rect(x, top, leftW, leftH), rgb(28, 31, 49), 22)
	rounded(dc, rect(x+24, top+22, 170, 34), rgb(18, 20, 34), 12)
	textDraw(dc, pomModeName(), rect(x+24, top+22, 170, 34), rgb(211, 215, 248), fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	label(dc, pomStatus, x+24, top+68, leftW-48, 32, rgb(159, 165, 201), fontSmall)
	sec := pomRemainingNow()
	clock := fmt.Sprintf("%02d:%02d", sec/60, sec%60)
	clockSize := int32(94)
	if leftW < 520 {
		clockSize = 82
	}
	clockFont := makeFont(clockSize, 700)
	textDraw(dc, clock, rect(x+20, top+106, leftW-40, 116), rgb(236, 239, 255), clockFont, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	call(pDeleteObject, clockFont)
	total := pomDuration(pomMode)
	passed := int32(0)
	if total > 0 {
		passed = int32((total - sec) * 1000 / total)
	}
	if passed < 0 {
		passed = 0
	}
	if passed > 1000 {
		passed = 1000
	}
	barX := x + 34
	barY := top + 238
	barW := leftW - 68
	rounded(dc, rect(barX, barY, barW, 12), rgb(45, 51, 79), 10)
	if passed > 0 {
		rounded(dc, rect(barX, barY, barW*passed/1000, 12), rgb(84, 180, 244), 10)
	}
	label(dc, fmt.Sprintf("%d%% do período", passed/10), barX, barY+18, leftW/2, 22, rgb(144, 151, 184), fontSmall)
	if pomRunning {
		label(dc, "Em andamento", barX+barW-128, barY+18, 128, 22, rgb(126, 209, 171), fontSmall)
	}

	by := top + 292
	buttonW := (leftW - 68 - 2*10) / 3
	primary := "▶  Iniciar"
	if pomRunning {
		primary = "Ⅱ  Pausar"
	} else if sec < total {
		primary = "▶  Continuar"
	}
	button(dc, primary, rect(x+34, by, buttonW, 44), rgb(255, 255, 255), rgb(84, 180, 244), "pomtoggle")
	button(dc, "↻ Reiniciar", rect(x+44+buttonW, by, buttonW, 44), rgb(235, 239, 255), rgb(39, 43, 67), "pomreset")
	button(dc, "Pular ›", rect(x+54+2*buttonW, by, buttonW, 44), rgb(255, 255, 255), rgb(182, 118, 248), "pomskip")
	line(dc, x+28, top+366, x+leftW-28, top+366, rgb(54, 60, 88))
	yButtons := top + 384
	sW := (leftW - 68 - 2*10) / 3
	modes := []struct{ txt, act, name string }{{"Foco", "pomfocus", "focus"}, {"Pausa curta", "pomshort", "short"}, {"Pausa longa", "pomlong", "long"}}
	for i, m := range modes {
		bg := rgb(18, 20, 34)
		fg := rgb(188, 194, 227)
		if pomMode == m.name {
			bg = rgb(92, 77, 185)
			fg = rgb(255, 255, 255)
		}
		button(dc, m.txt, rect(x+34+int32(i)*(sW+10), yButtons, sW, 42), fg, bg, m.act)
	}

	rounded(dc, rect(rightX, top, rightW, 228), rgb(28, 31, 49), 22)
	label(dc, "SOM AMBIENTE", rightX+22, top+18, rightW-44, 28, rgb(233, 236, 255), fontMedium)
	textDraw(dc, "Áudio contínuo mais macio e confortável para estudar ou trabalhar.", rect(rightX+22, top+54, rightW-44, 48), rgb(156, 163, 194), fontSmall, DT_LEFT|DT_WORDBREAK)
	txt := "▶  Ligar som ambiente"
	col := rgb(84, 180, 244)
	if noiseOn {
		txt = "■  Desligar som"
		col = rgb(126, 209, 171)
	}
	button(dc, txt, rect(rightX+22, top+104, rightW-44, 40), rgb(255, 255, 255), col, "noise")
	label(dc, "Volume", rightX+22, top+166, rightW-44, 22, rgb(201, 206, 231), fontSmall)
	button(dc, "−", rect(rightX+22, top+188, 34, 30), rgb(235, 239, 255), rgb(18, 20, 34), "volminus")
	textDraw(dc, fmt.Sprintf("%d%%", pomPrefs.Volume), rect(rightX+64, top+188, rightW-128, 30), rgb(233, 236, 255), fontMedium, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	button(dc, "+", rect(rightX+rightW-56, top+188, 34, 30), rgb(235, 239, 255), rgb(18, 20, 34), "volplus")

	sy := top + 244
	rounded(dc, rect(rightX, sy, rightW, bottom-sy), rgb(28, 31, 49), 22)
	label(dc, "PERSONALIZAR CICLOS", rightX+20, sy+12, rightW-40, 28, rgb(233, 236, 255), fontMedium)
	type dur struct {
		name        string
		n           int
		minus, plus string
	}
	rows := []dur{{"Foco", pomPrefs.FocusMinutes, "focusminus", "focusplus"}, {"Pausa curta", pomPrefs.ShortMinutes, "shortminus", "shortplus"}, {"Pausa longa", pomPrefs.LongMinutes, "longminus", "longplus"}}
	for i, r := range rows {
		yy := sy + 50 + int32(i)*50
		label(dc, r.name, rightX+20, yy, rightW-150, 32, rgb(197, 202, 229), fontSmall)
		button(dc, "−", rect(rightX+rightW-126, yy, 30, 30), rgb(235, 239, 255), rgb(18, 20, 34), r.minus)
		textDraw(dc, fmt.Sprintf("%d min", r.n), rect(rightX+rightW-92, yy, 58, 30), rgb(233, 236, 255), fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		button(dc, "+", rect(rightX+rightW-32, yy, 24, 30), rgb(235, 239, 255), rgb(18, 20, 34), r.plus)
	}
	label(dc, fmt.Sprintf("Sessões de foco concluídas: %d", pomPrefs.Sessions), x, winH-34, width, 24, rgb(77, 84, 118), fontSmall)
}

func random16() int32 {
	noiseSeed ^= noiseSeed << 13
	noiseSeed ^= noiseSeed >> 17
	noiseSeed ^= noiseSeed << 5
	return int32(noiseSeed>>16) - 32768
}
func refillNoise(c *noiseChunk) {
	// Softer than pure white noise: average multiple random sources and
	// apply a light low-pass filter so the result sounds smoother.
	peak := int32(pomPrefs.Volume * 5200 / 100)
	if peak < 0 {
		peak = 0
	}
	if peak > 5200 {
		peak = 5200
	}
	var smooth int32
	if len(c.pcm) > 0 {
		smooth = int32(c.pcm[len(c.pcm)-1])
	}
	for i := range c.pcm {
		raw := (random16() + random16() + random16()) / 3
		smooth = (smooth*15 + raw) / 16
		sample := (smooth*3 + raw) / 4
		c.pcm[i] = int16(sample * peak / 32768)
	}
	c.header.data = uintptr(unsafe.Pointer(&c.pcm[0]))
	c.header.length = uint32(len(c.pcm) * 2)
	c.header.recorded = 0
	c.header.flags = 0
	c.header.loops = 0
}

func noiseQueue(c *noiseChunk) bool {
	refillNoise(c)
	if call(pWavePrepare, waveDevice, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Sizeof(c.header))) != 0 {
		return false
	}
	c.ready = true
	if call(pWaveWrite, waveDevice, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Sizeof(c.header))) != 0 {
		_ = call(pWaveUnprepare, waveDevice, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Sizeof(c.header)))
		c.ready = false
		return false
	}
	return true
}
func noiseStart() error {
	if noiseOn {
		return nil
	}
	var device uintptr
	wf := waveFormatEx{formatTag: 1, channels: 1, sampleRate: 44100, bytesPerSecond: 88200, blockAlign: 2, bitsPerSample: 16}
	rc := call(pWaveOpen, uintptr(unsafe.Pointer(&device)), uintptr(^uint32(0)), uintptr(unsafe.Pointer(&wf)), 0, 0, 0)
	if rc != 0 {
		return fmt.Errorf("áudio do Windows: código %d", rc)
	}
	waveDevice = device
	// Eight ~150ms buffers avoid gaps even while the window is being redrawn.
	noiseChunks = make([]noiseChunk, 8)
	for i := range noiseChunks {
		noiseChunks[i].pcm = make([]int16, 6615)
	}
	noiseOn = true
	for i := range noiseChunks {
		if !noiseQueue(&noiseChunks[i]) {
			noiseStop()
			return fmt.Errorf("não foi possível preparar o áudio")
		}
	}
	return nil
}
func noisePoll() {
	if !noiseOn || waveDevice == 0 {
		return
	}
	for i := range noiseChunks {
		c := &noiseChunks[i]
		if c.ready && c.header.flags&1 != 0 { // WHDR_DONE
			if call(pWaveUnprepare, waveDevice, uintptr(unsafe.Pointer(&c.header)), uintptr(unsafe.Sizeof(c.header))) == 0 {
				c.ready = false
				if !noiseQueue(c) {
					noiseStop()
					return
				}
			}
		}
	}
}
func noiseStop() {
	if waveDevice != 0 {
		_ = call(pWaveReset, waveDevice)
		for i := range noiseChunks {
			if noiseChunks[i].ready {
				_ = call(pWaveUnprepare, waveDevice, uintptr(unsafe.Pointer(&noiseChunks[i].header)), uintptr(unsafe.Sizeof(noiseChunks[i].header)))
				noiseChunks[i].ready = false
			}
		}
		_ = call(pWaveClose, waveDevice)
	}
	waveDevice = 0
	noiseChunks = nil
	noiseOn = false
}

// Explicitly retain the package dependency when cross-compiling: WAVEFORMATEX
// is passed directly to winmm.dll, which is already shipped with Windows.
var _ = binary.LittleEndian
