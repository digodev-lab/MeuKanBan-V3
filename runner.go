//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unsafe"

	"meukanban/nativo/internal/layout"
	"meukanban/nativo/internal/runnerphysics"
)

// Game state is independent from project data and the Pomodoro timer.
// A dedicated 16ms Windows timer only exists while the player chooses to play.
var (
	runnerState           = "ready" // ready, playing, paused, over
	runnerSelected        int32
	runnerScore           float64
	runnerBest            int32
	runnerElapsed         float64
	runnerObstacleX       float64
	runnerObstacleW       float64 = 20
	runnerJumpY           float64
	runnerJumpV           float64
	runnerScroll          float64
	runnerLastFrame       time.Time
	runnerObstaclesPassed int32
	// Reusable game buffer avoids allocating a GDI bitmap/DC every frame.
	gameDC, gameBitmap, gameOldBitmap uintptr
	gameBufferWidth, gameBufferHeight int32
)

type runnerPrefs struct {
	Selected int32 `json:"selected"`
	Best     int32 `json:"best"`
}

func runnerPrefsPath() string { return filepath.Join(appDir, "pixel-runner.json") }
func runnerLoad() {
	prefs := runnerPrefs{}
	if b, err := os.ReadFile(runnerPrefsPath()); err == nil {
		_ = json.Unmarshal(b, &prefs)
	}
	if prefs.Selected >= 0 && prefs.Selected <= 2 {
		runnerSelected = prefs.Selected
	}
	if prefs.Best >= 0 {
		runnerBest = prefs.Best
	}
	runnerReady()
}
func runnerSave() {
	b, err := json.MarshalIndent(runnerPrefs{Selected: runnerSelected, Best: runnerBest}, "", "  ")
	if err != nil {
		return
	}
	tmp := runnerPrefsPath() + ".novo"
	if os.WriteFile(tmp, b, 0644) == nil {
		_ = atomicReplaceFile(tmp, runnerPrefsPath())
	}
}
func runnerReady() {
	call(pKillTimer, hwnd, 2)
	runnerState = "ready"
	runnerScore = 0
	runnerElapsed = 0
	runnerJumpY = 0
	runnerJumpV = 0
	runnerScroll = 0
	runnerObstaclesPassed = 0
	runnerObstacleW = 20
	runnerObstacleX = float64(runnerTrackWidth() + 30)
}
func runnerTrackWidth() int32 { return runnerViewRect().R - runnerViewRect().L - 24 }
func runnerViewRect() RECT {
	r := layout.Home(winW).Runner
	return rect(r.X, r.Y, r.W, r.H)
}
func runnerStart() {
	if page != "home" || runnerState == "playing" {
		return
	}
	if runnerState == "over" || runnerState == "ready" {
		initial := runnerphysics.New(float64(runnerTrackWidth()))
		runnerScore, runnerElapsed, runnerScroll = 0, 0, 0
		runnerJumpY, runnerJumpV, runnerObstaclesPassed = 0, 0, 0
		runnerObstacleX, runnerObstacleW = initial.ObstacleX, initial.ObstacleW
	}
	delay := uintptr(16)
	if reducedMotion {
		delay = 40
	}
	if call(pSetTimer, hwnd, 2, delay, 0) == 0 {
		runnerState = "ready"
		alert("O Windows não conseguiu iniciar o temporizador do jogo.")
		return
	}
	runnerState = "playing"
	runnerLastFrame = time.Now()
	invalidate()
}
func runnerPause() {
	if runnerState == "playing" {
		runnerState = "paused"
		call(pKillTimer, hwnd, 2)
		invalidate()
	}
}
func runnerSelect(value string) {
	idx, err := strconv.Atoi(value)
	if err != nil || idx < 0 || idx > 2 {
		return
	}
	runnerSelected = int32(idx)
	runnerReady()
	runnerSave()
	invalidate()
}
func runnerJump() {
	if page != "home" || runnerState != "playing" {
		return
	}
	if runnerJumpY < 0.01 && runnerJumpV == 0 {
		runnerJumpV = runnerphysics.InitialJumpVelocity // Shared physics constant; stays smooth at varying frame rates
	}
}
func runnerTick() {
	if page != "home" || runnerState != "playing" {
		return
	}
	now := time.Now()
	dt := now.Sub(runnerLastFrame).Seconds()
	runnerLastFrame = now
	if dt <= 0 {
		return
	}
	physics := runnerphysics.State{
		Elapsed: runnerElapsed, Score: runnerScore, ObstacleX: runnerObstacleX,
		ObstacleW: runnerObstacleW, JumpY: runnerJumpY, JumpVelocity: runnerJumpV,
		Scroll: runnerScroll, Passed: runnerObstaclesPassed,
	}
	runnerphysics.Advance(&physics, dt, float64(runnerTrackWidth()))
	runnerElapsed, runnerScore = physics.Elapsed, physics.Score
	runnerObstacleX, runnerObstacleW = physics.ObstacleX, physics.ObstacleW
	runnerJumpY, runnerJumpV = physics.JumpY, physics.JumpVelocity
	runnerScroll, runnerObstaclesPassed = physics.Scroll, physics.Passed
	if physics.GameOver {
		runnerState = "over"
		call(pKillTimer, hwnd, 2)
	}
	if score := int32(runnerScore); score > runnerBest {
		runnerBest = score
	}
	if runnerState == "over" {
		runnerSave()
		invalidate()
		return
	}
	r := runnerViewRect()
	call(pInvalidate, hwnd, uintptr(unsafe.Pointer(&r)), 0)
}
func pixelBlocks(dc uintptr, x, y, scale int32, rows [][3]int32, color uintptr) {
	for _, b := range rows {
		fill(dc, rect(x+b[0]*scale, y+b[1]*scale, b[2]*scale, scale), color)
	}
}
func runnerPixelArt(dc uintptr, x, groundY int32) {
	const sc int32 = 3
	y := groundY - 9*sc - int32(math.Round(runnerJumpY))
	base := rgb(84, 180, 244)
	accent := rgb(189, 233, 255)
	if runnerSelected == 1 {
		base = rgb(94, 97, 143)
		accent = rgb(182, 118, 248)
	}
	if runnerSelected == 2 {
		base = rgb(220, 66, 72)
		accent = rgb(255, 213, 92)
	}
	// Original pixel mascots, not reproductions of trademarked characters.
	pixelBlocks(dc, x, y, sc, [][3]int32{{2, 0, 3}, {1, 1, 5}, {1, 2, 5}, {2, 3, 3}, {1, 4, 5}, {2, 5, 3}, {1, 6, 2}, {4, 6, 2}}, base)
	pixelBlocks(dc, x, y, sc, [][3]int32{{0, 1, 1}, {0, 2, 1}, {5, 1, 1}, {5, 2, 1}, {2, 1, 1}}, accent)
	pixelBlocks(dc, x, y, sc, [][3]int32{{2, 2, 2}}, rgb(242, 246, 255))
	frame := 0
	if runnerState == "playing" && runnerJumpY < 1 && !reducedMotion {
		frame = int(runnerElapsed*11) % 2
	}
	if runnerJumpY > 0 || frame == 0 {
		pixelBlocks(dc, x, y, sc, [][3]int32{{1, 7, 2}, {4, 7, 2}, {0, 8, 3}, {4, 8, 3}}, accent)
	} else {
		pixelBlocks(dc, x, y, sc, [][3]int32{{2, 7, 2}, {4, 7, 2}, {1, 8, 3}, {5, 8, 2}}, accent)
	}
	if runnerState == "playing" && runnerJumpY < 1 && !reducedMotion {
		pixelBlocks(dc, x-12, y+20, sc, [][3]int32{{0, 0, 2}, {1, 2, 2}}, rgb(75, 82, 121))
	}
}
func gameButton(dc uintptr, labelText string, r RECT, fg, bg uintptr, act string, interactive bool) {
	if interactive && hoveredAction == act {
		bg = shaded(bg)
	}
	rounded(dc, r, bg, 12)
	textDraw(dc, labelText, r, fg, fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	if interactive {
		hit(r, act)
	}
}
func drawRunnerMiniGame(dc uintptr, x, y, w, h int32, interactive bool) {
	rounded(dc, rect(x, y, w, h), rgb(16, 18, 30), 16)
	label(dc, "PIXEL RUNNER", x+14, y+7, w-100, 19, rgb(229, 233, 252), fontSmall)
	scoreWidth := w - 30
	if runnerState == "playing" || runnerState == "paused" {
		scoreWidth = w - 110
	}
	label(dc, fmt.Sprintf("Pontos: %d   Recorde: %d", int32(runnerScore), runnerBest), x+14, y+26, scoreWidth, 19, rgb(143, 219, 199), fontSmall)
	if runnerState == "playing" || runnerState == "paused" {
		btn := "Pausar"
		action := "runnerpause"
		if runnerState == "paused" {
			btn = "Continuar"
			action = "runnerstart"
		}
		gameButton(dc, btn, rect(x+w-88, y+8, 77, 27), rgb(255, 255, 255), rgb(54, 59, 90), action, interactive)
	}
	track := rect(x+12, y+46, w-24, h-95)
	rounded(dc, track, rgb(26, 30, 49), 10)
	groundY := track.B - 11
	line(dc, track.L+5, groundY, track.R-5, groundY, rgb(111, 120, 163))
	// Compact indicator: increases during a run, freezes when paused.
	pace := runnerphysics.SpeedAt(runnerElapsed) / runnerphysics.BaseSpeed
	label(dc, fmt.Sprintf("%.1fx", pace), track.R-56, track.T+4, 48, 18, rgb(168, 177, 215), fontSmall)
	for i := int32(0); i < 9; i++ {
		scroll := int32(0)
		if !reducedMotion {
			scroll = int32(math.Round(runnerScroll))
		}
		gx := track.L + 5 + i*42 - scroll
		if gx >= track.L+4 && gx+14 < track.R-4 {
			line(dc, gx, groundY+5, gx+14, groundY+5, rgb(68, 74, 111))
		}
	}
	runnerPixelArt(dc, track.L+29, groundY)
	obX := track.L + int32(math.Round(runnerObstacleX))
	if obX < track.R && obX+int32(runnerObstacleW) > track.L {
		obW := int32(runnerObstacleW)
		rounded(dc, rect(obX, groundY-17, obW, 17), rgb(238, 190, 95), 3)
		fill(dc, rect(obX+4, groundY-int32(runnerphysics.ObstacleHeight), obW-8, 4), rgb(255, 218, 117))
	}
	if runnerState == "ready" || runnerState == "over" || runnerState == "paused" {
		status := "PRONTO PARA JOGAR"
		prompt := "▶  JOGAR"
		if runnerState == "over" {
			status = "FIM DE JOGO"
			prompt = "↻  TENTAR DE NOVO"
		}
		if runnerState == "paused" {
			status = "JOGO PAUSADO"
			prompt = "▶  CONTINUAR"
		}
		tx := track.L + 85
		tw := track.R - track.L - 92
		if tw < 125 {
			tx = track.L + 52
			tw = track.R - track.L - 56
		}
		textDraw(dc, status, rect(tx, track.T+12, tw, 21), rgb(241, 243, 255), fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		gameButton(dc, prompt, rect(tx+9, track.T+42, tw-18, 32), rgb(255, 255, 255), rgb(92, 77, 185), "runnerstart", interactive)
	} else if interactive {
		hit(rect(track.L, track.T, track.R-track.L, track.B-track.T), "runnerjump")
	}
	// Character selection never starts a match: player explicitly presses JOGAR.
	names := []string{"Azul veloz", "Sombrio", "Relâmpago"}
	gap := int32(6)
	btnW := (w - 24 - 2*gap) / 3
	for i, name := range names {
		bx := x + 12 + int32(i)*(btnW+gap)
		bg := rgb(40, 44, 68)
		fg := rgb(214, 219, 245)
		if runnerSelected == int32(i) {
			bg = rgb(79, 106, 168)
			fg = rgb(255, 255, 255)
		}
		gameButton(dc, name, rect(bx, y+h-34, btnW, 27), fg, bg, fmt.Sprintf("runnerchar|%d", i), interactive)
	}
}

func runnerReleaseBuffer() {
	if gameDC != 0 {
		if gameOldBitmap != 0 {
			call(pSelect, gameDC, gameOldBitmap)
		}
		if gameBitmap != 0 {
			call(pDeleteObject, gameBitmap)
		}
		call(pDeleteDC, gameDC)
	}
	gameDC, gameBitmap, gameOldBitmap = 0, 0, 0
	gameBufferWidth, gameBufferHeight = 0, 0
}
func runnerEnsureBuffer(src uintptr, w, h int32) bool {
	if gameDC != 0 && gameBufferWidth == w && gameBufferHeight == h {
		return true
	}
	runnerReleaseBuffer()
	if w <= 0 || h <= 0 {
		return false
	}
	gameDC = call(pCreateDC, src)
	if gameDC == 0 {
		return false
	}
	gameBitmap = call(pCreateBitmap, src, ut(w), ut(h))
	if gameBitmap == 0 {
		runnerReleaseBuffer()
		return false
	}
	gameOldBitmap = call(pSelect, gameDC, gameBitmap)
	if gameOldBitmap == 0 {
		runnerReleaseBuffer()
		return false
	}
	gameBufferWidth, gameBufferHeight = w, h
	return true
}

func runnerPaintPartial(src uintptr, dirty RECT) bool {
	if runnerState != "playing" && runnerState != "over" {
		return false
	}
	r := runnerViewRect()
	if dirty.L != r.L || dirty.T != r.T || dirty.R != r.R || dirty.B != r.B {
		return false
	}
	w, h := r.R-r.L, r.B-r.T
	if !runnerEnsureBuffer(src, w, h) {
		return false
	}
	drawRunnerMiniGame(gameDC, 0, 0, w, h, false)
	call(pBitBlt, src, ut(r.L), ut(r.T), ut(w), ut(h), gameDC, 0, 0, SRCCOPY)
	return true
}
