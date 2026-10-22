//go:build windows

// Meu Kanban Nativo — Windows GUI application (no PowerShell, WebView or .NET).
package main

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"meukanban/nativo/internal/layout"
)

//go:embed assets/icone.ico
var iconData []byte

type Task struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Details  string `json:"details"`
	Project  string `json:"project"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Due      string `json:"due"`
	Created  string `json:"created"`
}
type Store struct {
	Version  int      `json:"version"`
	Projects []string `json:"projects"`
	Tasks    []Task   `json:"tasks"`
}
type RECT struct{ L, T, R, B int32 }
type POINT struct{ X, Y int32 }
type MSG struct {
	Hwnd           uintptr
	Message        uint32
	_              uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             POINT
	LPrivate       uint32
	_              uint32
}
type WNDCLASSEX struct {
	Size, Style                         uint32
	Proc                                uintptr
	ClassExtra, WindowExtra             int32
	Instance, Icon, Cursor, Brush, Menu uintptr
	ClassName                           *uint16
	IconSm                              uintptr
}
type PAINTSTRUCT struct {
	HDC          uintptr
	Erase        int32
	Rc           RECT
	Restore, Inc int32
	Reserved     [32]byte
}
type Hot struct {
	rect   RECT
	action string
}
type Dialog struct {
	hwnd                                           uintptr
	kind                                           string
	editingID                                      string
	original                                       string
	title, details, project, priority, status, due uintptr
}

var (
	u32                                                    = syscall.NewLazyDLL("user32.dll")
	g32                                                    = syscall.NewLazyDLL("gdi32.dll")
	k32                                                    = syscall.NewLazyDLL("kernel32.dll")
	a32                                                    = syscall.NewLazyDLL("advapi32.dll")
	pRegister                                              = u32.NewProc("RegisterClassExW")
	pCreate                                                = u32.NewProc("CreateWindowExW")
	pDef                                                   = u32.NewProc("DefWindowProcW")
	pShow                                                  = u32.NewProc("ShowWindow")
	pUpdate                                                = u32.NewProc("UpdateWindow")
	pGetMsg                                                = u32.NewProc("GetMessageW")
	pTranslate                                             = u32.NewProc("TranslateMessage")
	pDispatch                                              = u32.NewProc("DispatchMessageW")
	pPostQuit                                              = u32.NewProc("PostQuitMessage")
	pInvalidate                                            = u32.NewProc("InvalidateRect")
	pBegin                                                 = u32.NewProc("BeginPaint")
	pEnd                                                   = u32.NewProc("EndPaint")
	pRect                                                  = u32.NewProc("GetClientRect")
	pMessage                                               = u32.NewProc("MessageBoxW")
	pEnable                                                = u32.NewProc("EnableWindow")
	pDestroy                                               = u32.NewProc("DestroyWindow")
	pFocus                                                 = u32.NewProc("SetFocus")
	pSetText                                               = u32.NewProc("SetWindowTextW")
	pGetText                                               = u32.NewProc("GetWindowTextW")
	pGetTextLength                                         = u32.NewProc("GetWindowTextLengthW")
	pSend                                                  = u32.NewProc("SendMessageW")
	pSetWindowLong                                         = u32.NewProc("SetWindowLongPtrW")
	pScreenToClient                                        = u32.NewProc("ScreenToClient")
	pLoadCursor                                            = u32.NewProc("LoadCursorW")
	pSetDPI                                                = u32.NewProc("SetProcessDPIAware")
	pGetModule                                             = k32.NewProc("GetModuleHandleW")
	pMoveFileEx                                            = k32.NewProc("MoveFileExW")
	pSystemParams                                          = u32.NewProc("SystemParametersInfoW")
	pTrackMouse                                            = u32.NewProc("TrackMouseEvent")
	pSetCursor                                             = u32.NewProc("SetCursor")
	pCreateIcon                                            = u32.NewProc("CreateIconFromResourceEx")
	pDeleteIcon                                            = u32.NewProc("DestroyIcon")
	pGetKey                                                = u32.NewProc("GetKeyState")
	pSetFore                                               = u32.NewProc("SetForegroundWindow")
	pCreateDC                                              = g32.NewProc("CreateCompatibleDC")
	pCreateBitmap                                          = g32.NewProc("CreateCompatibleBitmap")
	pSelect                                                = g32.NewProc("SelectObject")
	pDeleteObject                                          = g32.NewProc("DeleteObject")
	pDeleteDC                                              = g32.NewProc("DeleteDC")
	pBitBlt                                                = g32.NewProc("BitBlt")
	pCreateBrush                                           = g32.NewProc("CreateSolidBrush")
	pFillRect                                              = u32.NewProc("FillRect")
	pRoundRect                                             = g32.NewProc("RoundRect")
	pCreatePen                                             = g32.NewProc("CreatePen")
	pDrawText                                              = u32.NewProc("DrawTextW")
	pSetTextColor                                          = g32.NewProc("SetTextColor")
	pSetBkMode                                             = g32.NewProc("SetBkMode")
	pCreateFont                                            = g32.NewProc("CreateFontW")
	pMoveTo                                                = g32.NewProc("MoveToEx")
	pSaveDC                                                = g32.NewProc("SaveDC")
	pRestoreDC                                             = g32.NewProc("RestoreDC")
	pIntersectClip                                         = g32.NewProc("IntersectClipRect")
	pLineTo                                                = g32.NewProc("LineTo")
	pGetSysColorBrush                                      = u32.NewProc("GetSysColorBrush")
	pGetCursor                                             = u32.NewProc("GetCursorPos")
	pRegOpen                                               = a32.NewProc("RegOpenKeyExW")
	pRegSet                                                = a32.NewProc("RegSetValueExW")
	pRegQuery                                              = a32.NewProc("RegQueryValueExW")
	pRegDelete                                             = a32.NewProc("RegDeleteValueW")
	pRegClose                                              = a32.NewProc("RegCloseKey")
	hwnd                                                   uintptr
	mainClass                                              *uint16
	mainCallback                                           uintptr
	dlgCallback                                            uintptr
	data                                                   Store
	dataFile, appDir                                       string
	currentProject                                         string
	page                                                   = "home"
	autos                                                  bool
	filterText                                             string
	hits                                                   []Hot
	scrollHome                                             int32
	scrollCols                                             [3]int32
	dlg                                                    *Dialog
	winW                                                   = int32(1320)
	winH                                                   = int32(850)
	fontRegular, fontSmall, fontTitle, fontMedium, fontBig uintptr
	iconHandle                                             uintptr
	activeClip                                             *RECT
	selectedPriority                                       = "Normal"
	reducedMotion                                          bool
	hoveredAction                                          string
	trackingMouse                                          bool
	brushCache                                             = make(map[uintptr]uintptr)
	penCache                                               = make(map[uintptr]uintptr)
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x80000
	WS_TABSTOP          = 0x10000
	WS_BORDER           = 0x800000
	WS_VSCROLL          = 0x200000
	WS_POPUP            = 0x80000000
	WS_CLIPCHILDREN     = 0x02000000
	ES_MULTILINE        = 4
	ES_AUTOVSCROLL      = 0x40
	ES_WANTRETURN       = 0x1000
	ES_AUTOHSCROLL      = 0x80
	BS_PUSHBUTTON       = 0
	CBS_DROPDOWNLIST    = 3
	CW_USEDEFAULT       = 0x80000000
	COLOR_WINDOW        = 5
	WM_CREATE           = 1
	WM_TIMER            = 275
	WM_DESTROY          = 2
	WM_SIZE             = 5
	WM_PAINT            = 15
	WM_CLOSE            = 16
	WM_COMMAND          = 273
	WM_KEYDOWN          = 256
	WM_MOUSEMOVE        = 512
	WM_MOUSELEAVE       = 675
	WM_ACTIVATEAPP      = 28
	WM_SETTINGCHANGE    = 26
	WM_LBUTTONUP        = 514
	WM_MOUSEWHEEL       = 522
	WM_ERASEBKGND       = 20
	WM_SETFONT          = 48
	WM_SETICON          = 128
	DT_LEFT             = 0
	DT_CENTER           = 1
	DT_VCENTER          = 4
	DT_SINGLELINE       = 32
	DT_END_ELLIPSIS     = 0x8000
	DT_WORDBREAK        = 16
	TRANSPARENT         = 1
	PS_SOLID            = 0
	SRCCOPY             = 0xcc0020
	MB_ICONQUESTION     = 0x20
	MB_ICONINFORMATION  = 0x40
	MB_ICONERROR        = 0x10
	MB_YESNO            = 4
	IDYES               = 6
	CB_ADDSTRING        = 0x143
	CB_SETCURSEL        = 0x14E
	CB_GETCURSEL        = 0x147
	CB_GETLBTEXT        = 0x148
	EM_SETLIMITTEXT     = 0xC5
	SW_SHOW             = 5
	WS_EX_CLIENTEDGE    = 0x200
	GWL_STYLE           = -16
)

func ptr(s string) *uint16                              { return syscall.StringToUTF16Ptr(s) }
func call(p *syscall.LazyProc, args ...uintptr) uintptr { r, _, _ := p.Call(args...); return r }
func rgb(r, g, b uint8) uintptr                         { return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16) }
func ut(i int32) uintptr                                { return uintptr(uint32(i)) }
func rect(x, y, w, h int32) RECT                        { return RECT{x, y, x + w, y + h} }
func inside(r RECT, x, y int32) bool                    { return x >= r.L && x < r.R && y >= r.T && y < r.B }
func hit(r RECT, act string) {
	if activeClip != nil {
		if r.L < activeClip.L {
			r.L = activeClip.L
		}
		if r.R > activeClip.R {
			r.R = activeClip.R
		}
		if r.T < activeClip.T {
			r.T = activeClip.T
		}
		if r.B > activeClip.B {
			r.B = activeClip.B
		}
		if r.R <= r.L || r.B <= r.T {
			return
		}
	}
	hits = append(hits, Hot{r, act})
}
func alert(message string) {
	call(pMessage, hwnd, uintptr(unsafe.Pointer(ptr(message))), uintptr(unsafe.Pointer(ptr("Meu Kanban"))), MB_ICONINFORMATION)
}
func confirm(message string) bool {
	return call(pMessage, hwnd, uintptr(unsafe.Pointer(ptr(message))), uintptr(unsafe.Pointer(ptr("Meu Kanban"))), MB_YESNO|MB_ICONQUESTION) == IDYES
}
func invalidate() { call(pInvalidate, hwnd, 0, 0) }
func refreshMotionPreference() {
	var animation int32 = 1
	if call(pSystemParams, 0x1042, 0, uintptr(unsafe.Pointer(&animation)), 0) != 0 {
		reducedMotion = animation == 0
	}
}
func atomicReplaceFile(src, dst string) error {
	// MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH, same-directory files.
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return os.Rename(src, dst)
	}
	ok := call(pMoveFileEx, uintptr(unsafe.Pointer(ptr(src))), uintptr(unsafe.Pointer(ptr(dst))), 1|8)
	if ok == 0 {
		return fmt.Errorf("o Windows não pôde substituir %s; o original foi preservado", filepath.Base(dst))
	}
	return nil
}
func freeGdiCache() {
	for _, h := range brushCache {
		call(pDeleteObject, h)
	}
	for _, h := range penCache {
		call(pDeleteObject, h)
	}
	brushCache = make(map[uintptr]uintptr)
	penCache = make(map[uintptr]uintptr)
	for _, f := range []uintptr{fontRegular, fontSmall, fontTitle, fontMedium, fontBig} {
		if f != 0 {
			call(pDeleteObject, f)
		}
	}
	if iconHandle != 0 {
		call(pDeleteIcon, iconHandle)
	}
}
func shaded(c uintptr) uintptr {
	r, g, b := int(c&255), int((c>>8)&255), int((c>>16)&255)
	r += (255 - r) / 9
	g += (255 - g) / 9
	b += (255 - b) / 9
	return rgb(uint8(r), uint8(g), uint8(b))
}

func main() {
	runtime.LockOSThread()
	call(pSetDPI)
	appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "MeuKanban")
	if appDir == "MeuKanban" {
		home, _ := os.UserHomeDir()
		appDir = filepath.Join(home, "AppData", "Local", "MeuKanban")
	}
	dataFile = filepath.Join(appDir, "kanban.json")
	_ = os.MkdirAll(appDir, 0755)
	readStore()
	refreshMotionPreference()
	loadPomPrefs()
	runnerLoad()
	prepareIcon()
	inst := call(pGetModule, 0)
	mainClass = ptr("MeuKanbanNativoWin32")
	mainCallback = syscall.NewCallback(windowProc)
	dlgCallback = syscall.NewCallback(dialogProc)
	wc := WNDCLASSEX{Size: uint32(unsafe.Sizeof(WNDCLASSEX{})), Style: 3, Proc: uintptr(mainCallback), Instance: inst, Icon: iconHandle, IconSm: iconHandle, Cursor: call(pLoadCursor, 0, 32512), Brush: call(pGetSysColorBrush, COLOR_WINDOW), ClassName: mainClass}
	if call(pRegister, uintptr(unsafe.Pointer(&wc))) == 0 {
		alert("Não foi possível iniciar a interface do Windows.")
		return
	}
	wc2 := wc
	wc2.ClassName = ptr("MeuKanbanEditorWin32")
	wc2.Proc = uintptr(dlgCallback)
	_ = call(pRegister, uintptr(unsafe.Pointer(&wc2)))
	hwnd = call(pCreate, 0, uintptr(unsafe.Pointer(mainClass)), uintptr(unsafe.Pointer(ptr("Meu Kanban — Projetos"))), WS_OVERLAPPEDWINDOW|WS_VISIBLE|WS_CLIPCHILDREN, uintptr(100), uintptr(70), uintptr(1320), uintptr(850), 0, 0, inst, 0)
	if hwnd == 0 {
		return
	}
	initFonts()
	call(pSend, hwnd, WM_SETICON, 1, iconHandle)
	migrateStartup()
	call(pShow, hwnd, SW_SHOW)
	call(pSetTimer, hwnd, 1, 60, 0)
	invalidate()
	call(pUpdate, hwnd)
	var msg MSG
	for call(pGetMsg, uintptr(unsafe.Pointer(&msg)), 0, 0, 0) != 0 {
		call(pTranslate, uintptr(unsafe.Pointer(&msg)))
		call(pDispatch, uintptr(unsafe.Pointer(&msg)))
	}
}
func initFonts() {
	fontRegular = makeFont(16, 400)
	fontSmall = makeFont(13, 400)
	fontTitle = makeFont(26, 700)
	fontMedium = makeFont(18, 600)
	fontBig = makeFont(37, 750)
}
func makeFont(px, weight int32) uintptr {
	return call(pCreateFont, ut(-px), 0, 0, 0, ut(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(ptr("Segoe UI"))))
}
func windowProc(w uintptr, m uint32, wp, lp uintptr) uintptr {
	switch m {
	case 0x24: // WM_GETMINMAXINFO
		// minimum window size to keep the three columns legible
		(*[5]POINT)(unsafe.Pointer(lp))[3] = POINT{1160, 700}
		return 0
	case WM_SIZE:
		runnerReleaseBuffer()
		if wp == 1 {
			runnerPause()
		} // minimized, avoid consuming CPU
		winW = int32(uint16(lp))
		winH = int32(uint16(lp >> 16))
		if runnerState == "playing" {
			runnerLastFrame = time.Now()
		}
		invalidate()
		return 0
	case WM_ACTIVATEAPP:
		if wp == 0 {
			runnerPause()
		}
		return 0
	case WM_SETTINGCHANGE:
		old := reducedMotion
		refreshMotionPreference()
		if reducedMotion != old && runnerState == "playing" {
			call(pKillTimer, hwnd, 2)
			delay := uintptr(16)
			if reducedMotion {
				delay = 40
			}
			call(pSetTimer, hwnd, 2, delay, 0)
			runnerLastFrame = time.Now()
		}
		invalidate()
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_TIMER:
		if wp == 1 {
			pomTick()
		} else if wp == 2 {
			runnerTick()
		}
		return 0
	case WM_PAINT:
		paint()
		return 0
	case WM_MOUSEMOVE:
		var next string
		x, y := int32(int16(uint16(lp))), int32(int16(uint16(lp>>16)))
		for i := len(hits) - 1; i >= 0; i-- {
			if inside(hits[i].rect, x, y) {
				next = hits[i].action
				break
			}
		}
		if next != hoveredAction {
			hoveredAction = next
			invalidate()
		}
		if next != "" {
			call(pSetCursor, call(pLoadCursor, 0, 32649))
		} else {
			call(pSetCursor, call(pLoadCursor, 0, 32512))
		}
		if !trackingMouse {
			ev := struct {
				Size, Flags uint32
				Hwnd        uintptr
				HoverTime   uint32
			}{
				uint32(unsafe.Sizeof(struct {
					Size, Flags uint32
					Hwnd        uintptr
					HoverTime   uint32
				}{})), 2, w, 0,
			}
			trackingMouse = call(pTrackMouse, uintptr(unsafe.Pointer(&ev))) != 0
		}
		return 0
	case WM_MOUSELEAVE:
		trackingMouse = false
		if hoveredAction != "" {
			hoveredAction = ""
			invalidate()
		}
		return 0
	case WM_LBUTTONUP:
		x, y := int32(int16(uint16(lp))), int32(int16(uint16(lp>>16)))
		for i := len(hits) - 1; i >= 0; i-- {
			if inside(hits[i].rect, x, y) {
				doAction(hits[i].action)
				break
			}
		}
		return 0
	case WM_MOUSEWHEEL:
		pt := POINT{int32(int16(uint16(lp))), int32(int16(uint16(lp >> 16)))}
		call(pScreenToClient, w, uintptr(unsafe.Pointer(&pt)))
		delta := int16(uint16(wp >> 16))
		step := int32(74)
		if delta > 0 {
			step = -74
		}
		if page == "home" {
			scrollHome += step
			if scrollHome < 0 {
				scrollHome = 0
			}
			w := winW - 246 - 70
			cols := 1
			if w >= 790 {
				cols = 2
			}
			max := layout.ProjectScrollLimit(len(data.Projects), int32(cols), winH)
			if scrollHome > max {
				scrollHome = max
			}
		} else if page == "board" {
			idx := int((pt.X - 264) / ((winW - 296) / 3))
			if idx >= 0 && idx < 3 {
				scrollCols[idx] += step
				if scrollCols[idx] < 0 {
					scrollCols[idx] = 0
				}
				st := []string{"todo", "doing", "done"}[idx]
				var total int32
				for _, t := range data.Tasks {
					if t.Project == currentProject && t.Status == st && matchesFilter(t) {
						h := int32(181)
						if t.Details != "" {
							h = 206
						}
						total += h
					}
				}
				max := int32(186+182) + total - winH
				if max < 0 {
					max = 0
				}
				if scrollCols[idx] > max {
					scrollCols[idx] = max
				}
			}
		}
		invalidate()
		return 0
	case WM_KEYDOWN:
		if page == "home" && wp == 13 && runnerState != "playing" {
			runnerStart()
			return 0
		}
		if page == "home" && wp == 'P' && runnerState == "playing" {
			runnerPause()
			return 0
		}
		if page == "home" && wp == 'P' && runnerState == "paused" {
			runnerStart()
			return 0
		}
		if page == "home" && wp == 'R' {
			runnerReady()
			invalidate()
			return 0
		}
		if wp == 27 {
			runnerPause()
			page = "home"
			currentProject = ""
			invalidate()
			return 0
		}
		if page == "home" && (wp == 32 || wp == 38) && runnerState == "playing" {
			runnerJump()
			return 0
		}
		if wp == 'N' && (int16(call(pGetKey, 0x11)) < 0) {
			runnerPause()
			if page != "board" {
				openProjectDialog("", false)
			} else {
				openTaskDialog("", "todo")
			}
			return 0
		}
	case WM_DESTROY:
		call(pKillTimer, w, 1)
		call(pKillTimer, w, 2)
		runnerReleaseBuffer()
		freeGdiCache()
		runnerSave()
		noiseStop()
		savePomPrefs()
		call(pPostQuit, 0)
		return 0
	}
	return call(pDef, w, uintptr(m), wp, lp)
}

func readStore() {
	data = Store{Version: 1, Projects: []string{"Geral"}, Tasks: []Task{}}
	b, err := os.ReadFile(dataFile)
	if err != nil {
		return
	}
	var incoming Store
	if err = json.Unmarshal(b, &incoming); err != nil {
		alert("Seu arquivo kanban.json não pôde ser lido. Ele não será substituído até você salvar novos dados.")
		return
	}
	data = incoming
	if data.Projects == nil {
		data.Projects = []string{}
	}
	if data.Tasks == nil {
		data.Tasks = []Task{}
	}
	found := map[string]bool{}
	projects := []string{"Geral"}
	found["Geral"] = true
	for _, p := range data.Projects {
		p = strings.TrimSpace(p)
		if p != "" && !found[p] {
			projects = append(projects, p)
			found[p] = true
		}
	}
	for i := range data.Tasks {
		t := &data.Tasks[i]
		if t.Project == "" {
			t.Project = "Geral"
		}
		if t.Status != "todo" && t.Status != "doing" && t.Status != "done" {
			t.Status = "todo"
		}
		if t.Priority == "" {
			t.Priority = "Normal"
		}
		if !found[t.Project] {
			projects = append(projects, t.Project)
			found[t.Project] = true
		}
	}
	data.Projects = projects
}
func saveStore() {
	data.Version = 1
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		alert("Erro ao preparar dados: " + err.Error())
		return
	}
	tmp := dataFile + ".novo"
	if err = os.WriteFile(tmp, b, 0644); err != nil {
		alert("Erro ao salvar: " + err.Error())
		return
	}
	if _, err = os.Stat(dataFile); err == nil {
		backup := filepath.Join(appDir, "kanban-backup-anterior.json")
		if _, e := os.Stat(backup); os.IsNotExist(e) {
			if old, e := os.ReadFile(dataFile); e == nil {
				_ = os.WriteFile(backup, old, 0644)
			}
		}
	}
	if err = atomicReplaceFile(tmp, dataFile); err != nil {
		alert(err.Error() + "\nO arquivo temporário foi mantido para recuperação.")
		return
	}
	invalidate()
}
func prepareIcon() {
	if len(iconData) < 6 {
		return
	}
	n := int(binary.LittleEndian.Uint16(iconData[4:6]))
	bestSize := 0
	for i := 0; i < n; i++ {
		idx := 6 + i*16
		if idx+16 > len(iconData) {
			break
		}
		sz := int(binary.LittleEndian.Uint32(iconData[idx+8:]))
		off := int(binary.LittleEndian.Uint32(iconData[idx+12:]))
		if off+sz > len(iconData) || sz < bestSize {
			continue
		}
		bestSize = sz
		iconHandle = call(pCreateIcon, uintptr(unsafe.Pointer(&iconData[off])), uintptr(sz), 1, 0x30000, 64, 64, 0)
	}
}

func cachedBrush(color uintptr) uintptr {
	if b, ok := brushCache[color]; ok {
		return b
	}
	b := call(pCreateBrush, color)
	if b != 0 {
		brushCache[color] = b
	}
	return b
}
func cachedPen(color uintptr) uintptr {
	if p, ok := penCache[color]; ok {
		return p
	}
	p := call(pCreatePen, PS_SOLID, 1, color)
	if p != 0 {
		penCache[color] = p
	}
	return p
}
func fill(dc uintptr, r RECT, color uintptr) {
	if b := cachedBrush(color); b != 0 {
		call(pFillRect, dc, uintptr(unsafe.Pointer(&r)), b)
	}
}
func rounded(dc uintptr, r RECT, color uintptr, rad int32) {
	b, p := cachedBrush(color), cachedPen(color)
	if b == 0 || p == 0 {
		return
	}
	ob := call(pSelect, dc, b)
	op := call(pSelect, dc, p)
	call(pRoundRect, dc, ut(r.L), ut(r.T), ut(r.R), ut(r.B), ut(rad), ut(rad))
	call(pSelect, dc, ob)
	call(pSelect, dc, op)
}
func textDraw(dc uintptr, txt string, r RECT, color uintptr, font uintptr, flags uintptr) {
	if txt == "" {
		return
	}
	call(pSelect, dc, font)
	call(pSetTextColor, dc, color)
	call(pSetBkMode, dc, TRANSPARENT)
	call(pDrawText, dc, uintptr(unsafe.Pointer(ptr(txt))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
}
func label(dc uintptr, s string, x, y, w, h int32, color uintptr, font uintptr) {
	textDraw(dc, s, rect(x, y, w, h), color, font, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}
func button(dc uintptr, s string, r RECT, fg, bg uintptr, action string) {
	if hoveredAction == action && action != "" {
		bg = shaded(bg)
	}
	rounded(dc, r, bg, 16)
	textDraw(dc, s, r, fg, fontRegular, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	hit(r, action)
}
func line(dc uintptr, x1, y1, x2, y2 int32, c uintptr) {
	pen := cachedPen(c)
	if pen == 0 {
		return
	}
	old := call(pSelect, dc, pen)
	call(pMoveTo, dc, ut(x1), ut(y1), 0)
	call(pLineTo, dc, ut(x2), ut(y2))
	call(pSelect, dc, old)
}
func drawGridBackground(dc uintptr, r RECT) {
	fill(dc, r, rgb(201, 207, 229))
	step := int32(58)
	for x := r.L; x < r.R; x += step {
		line(dc, x, r.T, x, r.B, rgb(109, 117, 150))
	}
	for y := r.T; y < r.B; y += step {
		line(dc, r.L, y, r.R, y, rgb(109, 117, 150))
	}
}

// drawProjectArtwork uses Win32 primitives rather than a photograph or image file.
func drawProjectArtwork(dc uintptr, x, y, w, h int32) {
	rounded(dc, rect(x, y, w, h), rgb(24, 26, 43), 22)
	gameH := h - 100
	drawRunnerMiniGame(dc, x+12, y+12, w-24, gameH, true)
	panelY := y + h - 74
	panelW := (w - 42) / 2
	taskDone := 0
	for _, t := range data.Tasks {
		if t.Status == "done" {
			taskDone++
		}
	}
	// Focus bar represents progress toward a session goal of four Pomodoros.
	focusPct := int32(pomPrefs.Sessions % 4 * 100 / 4)
	if pomPrefs.Sessions > 0 && pomPrefs.Sessions%4 == 0 {
		focusPct = 100
	}
	volume := int32(pomPrefs.Volume)
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	rounded(dc, rect(x+12, panelY, panelW, 62), rgb(16, 18, 30), 12)
	label(dc, "FOCO", x+23, panelY+5, panelW-22, 16, rgb(238, 206, 101), fontSmall)
	label(dc, fmt.Sprintf("%d pom. • %d feitas", pomPrefs.Sessions, taskDone), x+23, panelY+22, panelW-22, 16, rgb(182, 189, 218), fontSmall)
	rounded(dc, rect(x+23, panelY+45, panelW-22, 7), rgb(43, 47, 70), 7)
	if focusPct > 0 {
		rounded(dc, rect(x+23, panelY+45, (panelW-22)*focusPct/100, 7), rgb(147, 125, 244), 7)
	}
	px := x + 30 + panelW
	rounded(dc, rect(px, panelY, panelW, 62), rgb(16, 18, 30), 12)
	label(dc, "AMBIENTE", px+11, panelY+5, panelW-22, 16, rgb(143, 219, 199), fontSmall)
	status := "Som desligado"
	if noiseOn {
		status = "Som ligado"
	}
	label(dc, fmt.Sprintf("%s | %d%%", status, pomPrefs.Volume), px+11, panelY+22, panelW-22, 16, rgb(182, 189, 218), fontSmall)
	rounded(dc, rect(px+11, panelY+45, panelW-22, 7), rgb(43, 47, 70), 7)
	if noiseOn && volume > 0 {
		rounded(dc, rect(px+11, panelY+45, (panelW-22)*volume/100, 7), rgb(95, 205, 228), 7)
	}
}

func paint() {
	var ps PAINTSTRUCT
	src := call(pBegin, hwnd, uintptr(unsafe.Pointer(&ps)))
	if src == 0 {
		return
	}
	defer call(pEnd, hwnd, uintptr(unsafe.Pointer(&ps)))
	if page == "home" && runnerPaintPartial(src, ps.Rc) {
		return
	}
	if winW < 750 || winH < 450 {
		fill(src, rect(0, 0, winW, winH), rgb(201, 207, 229))
		return
	}
	dc := call(pCreateDC, src)
	if dc == 0 {
		return
	}
	bmp := call(pCreateBitmap, src, ut(winW), ut(winH))
	if bmp == 0 {
		call(pDeleteDC, dc)
		return
	}
	oldBmp := call(pSelect, dc, bmp)
	defer func() {
		call(pBitBlt, src, 0, 0, ut(winW), ut(winH), dc, 0, 0, SRCCOPY)
		call(pSelect, dc, oldBmp)
		call(pDeleteObject, bmp)
		call(pDeleteDC, dc)
	}()
	hits = hits[:0]
	drawGridBackground(dc, rect(0, 0, winW, winH))
	drawSidebar(dc)
	if page == "home" {
		drawHome(dc)
	} else if page == "pomodoro" {
		drawPomodoro(dc)
	} else {
		drawBoard(dc)
	}
}

func drawSidebar(dc uintptr) {
	railW := int32(72)
	panelW := int32(174)
	totalW := railW + panelW
	fill(dc, rect(0, 0, railW, winH), rgb(20, 22, 35))
	fill(dc, rect(railW, 0, panelW, winH), rgb(25, 28, 44))
	rounded(dc, rect(18, 18, 36, 36), rgb(84, 180, 244), 12)
	textDraw(dc, "✦", rect(18, 18, 36, 36), rgb(255, 255, 255), fontMedium, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	label(dc, "MEU KANBAN", railW+16, 22, panelW-24, 28, rgb(231, 235, 255), fontMedium)
	label(dc, "Pomodoro aesthetic", railW+16, 50, panelW-24, 20, rgb(144, 151, 184), fontSmall)

	icons := []struct {
		y   int32
		txt string
	}{{92, "⌂"}, {140, "▣"}, {188, "◷"}}
	for _, it := range icons {
		rounded(dc, rect(17, it.y, 38, 38), rgb(38, 42, 64), 12)
		textDraw(dc, it.txt, rect(17, it.y, 38, 38), rgb(210, 216, 245), fontMedium, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	}

	homeBg := rgb(37, 41, 63)
	if page == "home" {
		homeBg = rgb(92, 77, 185)
	}
	button(dc, "⌂   Início", rect(railW+14, 108, panelW-28, 38), rgb(239, 243, 255), homeBg, "home")
	button(dc, "+   Novo projeto", rect(railW+14, 154, panelW-28, 38), rgb(255, 255, 255), rgb(84, 180, 244), "newproject")
	pomBg := rgb(37, 41, 63)
	if page == "pomodoro" {
		pomBg = rgb(182, 118, 248)
	}
	button(dc, "◷   Pomodoro", rect(railW+14, 200, panelW-28, 38), rgb(247, 248, 255), pomBg, "pomopen")

	label(dc, "PROJETOS", railW+16, 258, panelW-28, 22, rgb(143, 149, 182), fontSmall)
	for i, p := range data.Projects {
		if i >= 10 {
			label(dc, "+ projetos na tela inicial", railW+16, 290+int32(i)*40, panelW-28, 22, rgb(135, 141, 176), fontSmall)
			break
		}
		y := int32(290 + i*40)
		if y > winH-145 {
			break
		}
		bg := rgb(37, 41, 63)
		fg := rgb(225, 229, 248)
		if page == "board" && currentProject == p {
			bg = rgb(92, 77, 185)
			fg = rgb(255, 255, 255)
		}
		rounded(dc, rect(railW+14, y, panelW-28, 32), bg, 11)
		label(dc, p, railW+28, y+3, panelW-54, 24, fg, fontSmall)
		hit(rect(railW+14, y, panelW-28, 32), "open|"+p)
	}
	line(dc, railW+14, winH-104, totalW-14, winH-104, rgb(63, 68, 95))
	t := "○ Iniciar com Windows"
	if autos {
		t = "● Iniciar com Windows"
	}
	label(dc, t, railW+16, winH-92, panelW-28, 28, rgb(221, 226, 248), fontSmall)
	hit(rect(railW+12, winH-94, panelW-24, 31), "autostart")
	label(dc, "offline • visual dark", railW+16, winH-42, panelW-28, 20, rgb(120, 127, 162), fontSmall)
}

func countProject(name string) (total, done int) {
	for _, t := range data.Tasks {
		if t.Project == name {
			total++
			if t.Status == "done" {
				done++
			}
		}
	}
	return
}
func drawHome(dc uintptr) {
	x := int32(246)
	w := winW - x
	margin := int32(32)
	geo := layout.Home(winW)
	label(dc, "WELCOME BACK", x+margin, 24, 320, 24, rgb(65, 70, 98), fontSmall)
	label(dc, "Workspace dos seus projetos.", x+margin, 52, w-2*margin, 45, rgb(28, 30, 43), fontTitle)

	bannerY := int32(114)
	bannerH := int32(332)
	rounded(dc, rect(x+margin, bannerY, w-2*margin, bannerH), rgb(28, 31, 49), 24)
	art := geo.Artwork
	drawProjectArtwork(dc, art.X, art.Y, art.W, art.H)
	leftW := geo.Intro.W
	rounded(dc, rect(x+margin+18, bannerY+18, leftW, bannerH-36), rgb(18, 20, 34), 18)
	label(dc, "AESTHETIC PLANNING", x+margin+38, bannerY+40, leftW-38, 22, rgb(132, 141, 192), fontSmall)
	textDraw(dc, "Bem-vindo. Escolha um projeto ou crie um novo.", rect(x+margin+38, bannerY+76, leftW-46, 62), rgb(225, 229, 248), fontTitle, DT_LEFT|DT_WORDBREAK)
	label(dc, "Jogo: Enter jogar · Espaço pular · P pausar · R reiniciar", x+margin+38, bannerY+142, leftW-46, 25, rgb(157, 165, 201), fontSmall)
	button(dc, "+  Novo projeto", rect(x+margin+38, bannerY+182, 184, 42), rgb(255, 255, 255), rgb(84, 180, 244), "newproject")
	button(dc, "◷  Abrir pomodoro", rect(x+margin+232, bannerY+182, 176, 42), rgb(255, 255, 255), rgb(182, 118, 248), "pomopen")

	total := len(data.Tasks)
	done := 0
	for _, t := range data.Tasks {
		if t.Status == "done" {
			done++
		}
	}
	label(dc, "PROJETOS", x+margin, 466, w/2, 30, rgb(27, 31, 48), fontMedium)
	button(dc, "Exportar backup", rect(winW-margin-154, 464, 154, 36), rgb(241, 244, 255), rgb(28, 31, 49), "export")
	label(dc, fmt.Sprintf("%d projeto(s)  •  %d tarefas  •  %d concluída(s)", len(data.Projects), total, done), x+margin, 496, w-2*margin, 24, rgb(83, 89, 122), fontSmall)

	cardsX := x + margin
	cardsW := w - 2*margin
	cols := 2
	if geo.CardsColumns == 1 {
		cols = 1
	}
	gap := int32(18)
	cardW := (cardsW - int32(cols-1)*gap) / int32(cols)
	cardH := int32(150)
	saved := call(pSaveDC, dc)
	clip := RECT{cardsX, 526, cardsX + cardsW, winH - 16}
	call(pIntersectClip, dc, ut(clip.L), ut(clip.T), ut(clip.R), ut(clip.B))
	activeClip = &clip
	for i, p := range data.Projects {
		row, col := int32(i/cols), int32(i%cols)
		cx := cardsX + col*(cardW+gap)
		cy := int32(536) + row*164 - scrollHome
		if cy > winH-15 || cy+cardH < 526 {
			continue
		}
		rounded(dc, rect(cx, cy, cardW, cardH), rgb(25, 28, 45), 22)
		rounded(dc, rect(cx+16, cy+16, 38, 38), rgb(39, 43, 67), 12)
		label(dc, "▣", cx+28, cy+18, 20, 30, rgb(137, 125, 244), fontMedium)
		label(dc, p, cx+68, cy+18, cardW-110, 30, rgb(229, 233, 251), fontMedium)
		cnt, completed := countProject(p)
		percent := 0
		if cnt > 0 {
			percent = completed * 100 / cnt
		}
		label(dc, fmt.Sprintf("%d tarefa(s) · %d%% concluído", cnt, percent), cx+18, cy+66, cardW-26, 21, rgb(151, 158, 191), fontSmall)
		rounded(dc, rect(cx+18, cy+100, cardW-36, 8), rgb(46, 51, 77), 8)
		if percent > 0 {
			rounded(dc, rect(cx+18, cy+100, (cardW-36)*int32(percent)/100, 8), rgb(84, 180, 244), 8)
		}
		hit(rect(cx, cy, cardW, 112), "open|"+p)
		button(dc, "Abrir", rect(cx+18, cy+116, 68, 24), rgb(255, 255, 255), rgb(92, 77, 185), "open|"+p)
		if p != "Geral" {
			button(dc, "Renomear", rect(cx+cardW-178, cy+116, 96, 24), rgb(229, 233, 251), rgb(39, 43, 67), "rename|"+p)
			button(dc, "Excluir", rect(cx+cardW-78, cy+116, 66, 24), rgb(255, 216, 223), rgb(90, 41, 55), "remove|"+p)
		}
	}
	activeClip = nil
	call(pRestoreDC, dc, saved)
	maxScroll := layout.ProjectScrollLimit(len(data.Projects), int32(cols), winH)
	if maxScroll > 0 {
		trackY := int32(526)
		trackH := winH - trackY - 22
		if trackH > 36 {
			trackX := cardsX + cardsW + 8
			rounded(dc, rect(trackX, trackY, 5, trackH), rgb(104, 111, 147), 5)
			thumb := trackH * trackH / (trackH + maxScroll)
			if thumb < 30 {
				thumb = 30
			}
			if thumb > trackH {
				thumb = trackH
			}
			yy := trackY + scrollHome*(trackH-thumb)/maxScroll
			rounded(dc, rect(trackX, yy, 5, thumb), rgb(62, 68, 100), 5)
		}
	}
	if len(data.Projects) == 1 {
		label(dc, "Crie um novo projeto para começar!", cardsX, winH-44, cardsW, 25, rgb(68, 73, 103), fontSmall)
	}
}

func matchesFilter(t Task) bool {
	return filterText == "" || strings.Contains(strings.ToLower(t.Title+" "+t.Details), strings.ToLower(filterText))
}
func drawBoard(dc uintptr) {
	x := int32(264)
	wd := winW - x - 26
	button(dc, "‹  Projetos", rect(x, 26, 126, 38), rgb(255, 255, 255), rgb(28, 31, 49), "home")
	button(dc, "◷  Pomodoro", rect(x+136, 26, 148, 38), rgb(255, 255, 255), rgb(92, 77, 185), "pomopen")
	button(dc, "+  Nova tarefa", rect(winW-214, 26, 180, 42), rgb(255, 255, 255), rgb(84, 180, 244), "newtask|todo")
	label(dc, currentProject, x, 84, wd-210, 46, rgb(20, 23, 35), fontBig)
	count, done := countProject(currentProject)
	label(dc, fmt.Sprintf("%d tarefas  •  %d concluídas  •  organize seu fluxo", count, done), x, 132, wd-220, 26, rgb(83, 89, 122), fontSmall)
	button(dc, "⌕  Pesquisar", rect(winW-180, 120, 146, 36), rgb(255, 255, 255), rgb(28, 31, 49), "search")
	if filterText != "" {
		button(dc, "Limpar filtro", rect(x, 162, 112, 29), rgb(255, 255, 255), rgb(92, 77, 185), "clearsearch")
		label(dc, "Filtro: "+filterText, x+130, 162, 360, 29, rgb(73, 80, 112), fontSmall)
	}
	titles := []string{"A FAZER", "EM ANDAMENTO", "CONCLUÍDO"}
	help := []string{"Planejado", "Em execução", "Finalizado"}
	colors := []uintptr{rgb(84, 180, 244), rgb(182, 118, 248), rgb(126, 209, 171)}
	gap := int32(14)
	colW := layout.KanbanColumnWidth(winW)
	if colW < 200 {
		return
	}
	statuses := []string{"todo", "doing", "done"}
	for j, status := range statuses {
		cx := x + int32(j)*(colW+gap)
		top := int32(196)
		rounded(dc, rect(cx, top, colW, winH-top-22), rgb(28, 31, 49), 19)
		rounded(dc, rect(cx+15, top+16, 9, 25), colors[j], 6)
		label(dc, titles[j], cx+33, top+13, colW-90, 29, rgb(236, 239, 255), fontMedium)
		n := 0
		for _, t := range data.Tasks {
			if t.Project == currentProject && t.Status == status && matchesFilter(t) {
				n++
			}
		}
		label(dc, fmt.Sprintf("%d", n), cx+colW-36, top+13, 30, 29, colors[j], fontMedium)
		label(dc, help[j], cx+19, top+48, colW-36, 21, rgb(138, 146, 181), fontSmall)
		saved := call(pSaveDC, dc)
		clip := RECT{cx + 5, top + 75, cx + colW - 5, winH - 92}
		call(pIntersectClip, dc, ut(clip.L), ut(clip.T), ut(clip.R), ut(clip.B))
		activeClip = &clip
		cy := top + 85 - scrollCols[j]
		if n == 0 {
			label(dc, "Nenhuma tarefa nesta etapa", cx+20, top+99, colW-40, 50, rgb(138, 146, 181), fontSmall)
		}
		for _, t := range data.Tasks {
			if t.Project != currentProject || t.Status != status || !matchesFilter(t) {
				continue
			}
			cardH := int32(172)
			if len(t.Details) > 0 {
				cardH = 198
			}
			if cy+cardH < top+72 {
				cy += cardH + 12
				continue
			}
			if cy > winH-26 {
				break
			}
			r := rect(cx+12, cy, colW-24, cardH)
			rounded(dc, r, rgb(18, 20, 34), 15)
			label(dc, t.Title, r.L+13, r.T+12, r.R-r.L-26, 28, rgb(229, 233, 251), fontMedium)
			prBg := rgb(39, 43, 67)
			prFg := rgb(195, 199, 223)
			if t.Priority == "Alta" {
				prBg = rgb(84, 49, 40)
				prFg = rgb(255, 212, 173)
			} else if t.Priority == "Baixa" {
				prBg = rgb(28, 63, 54)
				prFg = rgb(186, 243, 220)
			}
			rounded(dc, rect(r.L+13, r.T+46, 94, 26), prBg, 10)
			label(dc, t.Priority, r.L+24, r.T+47, 74, 22, prFg, fontSmall)
			if t.Due != "" {
				label(dc, "Prazo: "+t.Due, r.L+116, r.T+47, r.R-r.L-130, 23, rgb(150, 156, 187), fontSmall)
			}
			if t.Details != "" {
				textDraw(dc, t.Details, rect(r.L+14, r.T+84, r.R-r.L-28, 48), rgb(156, 163, 194), fontSmall, DT_WORDBREAK|DT_END_ELLIPSIS)
			}
			editY := r.B - 44
			if status == "doing" {
				// Three actions share the available width without overlaps.
				gap := int32(6)
				bw := layout.ThreeActionWidth(r.R - r.L)
				button(dc, "Editar", rect(r.L+12, editY, bw, 30), rgb(255, 255, 255), rgb(84, 180, 244), "edit|"+t.ID)
				button(dc, "‹ Voltar", rect(r.L+12+bw+gap, editY, bw, 30), rgb(235, 239, 255), rgb(39, 43, 67), "back|"+t.ID)
				button(dc, "Concluir ›", rect(r.L+12+2*(bw+gap), editY, bw, 30), rgb(255, 255, 255), colors[j], "next|"+t.ID)
			} else {
				button(dc, "Editar", rect(r.L+12, editY, 76, 30), rgb(255, 255, 255), rgb(84, 180, 244), "edit|"+t.ID)
				if status == "done" {
					button(dc, "‹ Voltar", rect(r.L+94, editY, 88, 30), rgb(235, 239, 255), rgb(39, 43, 67), "back|"+t.ID)
				} else {
					button(dc, "Iniciar ›", rect(r.R-108, editY, 96, 30), rgb(255, 255, 255), colors[j], "next|"+t.ID)
				}
			}
			hit(rect(r.L, r.T, r.R-r.L, 43), "edit|"+t.ID)
			cy += cardH + 12
		}
		activeClip = nil
		call(pRestoreDC, dc, saved)
	}
}

func doAction(action string) {
	parts := strings.SplitN(action, "|", 2)
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	switch parts[0] {
	case "pomopen", "pomtoggle", "pomreset", "pomskip", "pomfocus", "pomshort", "pomlong", "noise", "volminus", "volplus", "focusminus", "focusplus", "shortminus", "shortplus", "longminus", "longplus":
		if parts[0] == "pomopen" {
			runnerPause()
		}
		pomAction(parts[0])
	case "home":
		runnerPause()
		page = "home"
		currentProject = ""
		scrollHome = 0
		filterText = ""
		invalidate()
	case "open":
		runnerPause()
		page = "board"
		currentProject = arg
		scrollCols = [3]int32{}
		filterText = ""
		invalidate()
	case "runnerjump":
		runnerJump()
	case "runnerstart":
		runnerStart()
	case "runnerpause":
		runnerPause()
	case "runnerchar":
		runnerSelect(arg)
	case "newproject":
		runnerPause()
		openProjectDialog("", false)
	case "rename":
		runnerPause()
		openProjectDialog(arg, true)
	case "remove":
		runnerPause()
		if arg == "Geral" {
			return
		}
		if !confirm("Excluir o projeto '" + arg + "'?\n\nAs tarefas serão movidas para o projeto Geral.") {
			return
		}
		out := make([]string, 0, len(data.Projects))
		for _, p := range data.Projects {
			if p != arg {
				out = append(out, p)
			}
		}
		data.Projects = out
		for i := range data.Tasks {
			if data.Tasks[i].Project == arg {
				data.Tasks[i].Project = "Geral"
			}
		}
		if currentProject == arg {
			page = "home"
			currentProject = ""
		}
		saveStore()
	case "newtask":
		openTaskDialog("", arg)
	case "search":
		openSearchDialog()
	case "clearsearch":
		filterText = ""
		scrollCols = [3]int32{}
		invalidate()
	case "edit":
		openTaskDialog(arg, "todo")
	case "back", "next":
		for i := range data.Tasks {
			if data.Tasks[i].ID != arg {
				continue
			}
			t := &data.Tasks[i]
			if parts[0] == "back" {
				if t.Status == "done" {
					t.Status = "doing"
				} else {
					t.Status = "todo"
				}
			} else {
				if t.Status == "todo" {
					t.Status = "doing"
				} else {
					t.Status = "done"
				}
			}
			saveStore()
			break
		}
	case "autostart":
		setStartup(!autos)
	case "export":
		runnerPause()
		exportBackup()
	}
}
func uniqueProject(s, old string) bool {
	for _, p := range data.Projects {
		if strings.EqualFold(p, s) && p != old {
			return false
		}
	}
	return true
}
func openProjectDialog(original string, rename bool) {
	if dlg != nil {
		return
	}
	kind := "project"
	caption := "Novo projeto"
	if rename {
		kind = "rename"
		caption = "Renomear projeto"
	}
	d := newDialog(kind, caption, 530, 255)
	if d == nil {
		return
	}
	d.original = original
	createLabel(d.hwnd, "NOME DO PROJETO", 27, 24, 435, 28)
	d.title = createControl("EDIT", original, WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL, 27, 63, 468, 34, d.hwnd, 801)
	call(pSend, d.title, EM_SETLIMITTEXT, 100, 0)
	createLabel(d.hwnd, "Organize suas tarefas por temas, objetivos ou equipes.", 27, 107, 475, 35)
	createButton(d.hwnd, "Cancelar", 262, 174, 105, 39, 501)
	createButton(d.hwnd, "Salvar projeto", 375, 174, 122, 39, 500)
	showDialog(d)
}
func openSearchDialog() {
	if dlg != nil {
		return
	}
	d := newDialog("search", "Pesquisar tarefas", 515, 245)
	if d == nil {
		return
	}
	createLabel(d.hwnd, "BUSQUE POR TÍTULO OU ANOTAÇÕES", 27, 25, 451, 25)
	d.title = createControl("EDIT", filterText, WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL, 27, 63, 448, 34, d.hwnd, 808)
	createLabel(d.hwnd, "Deixe vazio para visualizar todas as tarefas.", 27, 107, 452, 30)
	createButton(d.hwnd, "Cancelar", 247, 166, 102, 39, 501)
	createButton(d.hwnd, "Pesquisar", 359, 166, 116, 39, 500)
	showDialog(d)
}
func openTaskDialog(id, status string) {
	if dlg != nil {
		return
	}
	t := Task{Priority: "Normal", Status: status, Project: currentProject}
	if t.Project == "" {
		t.Project = "Geral"
	}
	kind := "newtask"
	title := "Nova tarefa"
	for _, v := range data.Tasks {
		if v.ID == id {
			t = v
			kind = "edittask"
			title = "Editar tarefa"
			break
		}
	}
	d := newDialog(kind, title, 650, 603)
	if d == nil {
		return
	}
	d.editingID = id
	createLabel(d.hwnd, "TÍTULO DA TAREFA", 27, 13, 570, 25)
	d.title = createControl("EDIT", t.Title, WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL, 27, 46, 590, 31, d.hwnd, 802)
	createLabel(d.hwnd, "ANOTAÇÕES", 27, 89, 570, 25)
	d.details = createControl("EDIT", t.Details, WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_BORDER|ES_MULTILINE|ES_AUTOVSCROLL|ES_WANTRETURN|WS_VSCROLL, 27, 117, 590, 123, d.hwnd, 803)
	createLabel(d.hwnd, "PROJETO", 27, 256, 276, 25)
	createLabel(d.hwnd, "ETAPA", 339, 256, 276, 25)
	d.project = createControl("COMBOBOX", "", WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST, 27, 283, 276, 340, d.hwnd, 804)
	for i, p := range data.Projects {
		comboAdd(d.project, p)
		if p == t.Project {
			comboSelect(d.project, i)
		}
	}
	d.status = createControl("COMBOBOX", "", WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST, 339, 283, 278, 340, d.hwnd, 805)
	for i, v := range []string{"A fazer", "Em andamento", "Concluído"} {
		comboAdd(d.status, v)
		if []string{"todo", "doing", "done"}[i] == t.Status {
			comboSelect(d.status, i)
		}
	}
	createLabel(d.hwnd, "PRIORIDADE", 27, 335, 276, 25)
	createLabel(d.hwnd, "PRAZO (AAAA-MM-DD)", 339, 335, 278, 25)
	d.priority = createControl("COMBOBOX", "", WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST, 27, 363, 276, 260, d.hwnd, 806)
	for i, p := range []string{"Baixa", "Normal", "Alta"} {
		comboAdd(d.priority, p)
		if p == t.Priority {
			comboSelect(d.priority, i)
		}
	}
	d.due = createControl("EDIT", t.Due, WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL, 339, 363, 278, 30, d.hwnd, 807)
	createLabel(d.hwnd, "Suas alterações são salvas automaticamente neste computador.", 27, 417, 588, 27)
	if kind == "edittask" {
		createButton(d.hwnd, "Excluir tarefa", 27, 485, 126, 40, 502)
	}
	createButton(d.hwnd, "Cancelar", 352, 485, 113, 40, 501)
	createButton(d.hwnd, "Salvar tarefa", 476, 485, 142, 40, 500)
	showDialog(d)
}
func createLabel(parent uintptr, s string, x, y, w, h int32) {
	ctl := createControl("STATIC", s, WS_CHILD|WS_VISIBLE, x, y, w, h, parent, 0)
	if fontRegular != 0 {
		call(pSend, ctl, WM_SETFONT, fontRegular, 1)
	}
}
func createButton(parent uintptr, s string, x, y, w, h int32, id uintptr) uintptr {
	return createControl("BUTTON", s, WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_PUSHBUTTON, x, y, w, h, parent, id)
}
func createControl(class, title string, style uintptr, x, y, w, h int32, parent, id uintptr) uintptr {
	ctl := call(pCreate, 0, uintptr(unsafe.Pointer(ptr(class))), uintptr(unsafe.Pointer(ptr(title))), style, ut(x), ut(y), ut(w), ut(h), parent, id, call(pGetModule, 0), 0)
	if fontRegular != 0 {
		call(pSend, ctl, WM_SETFONT, fontRegular, 1)
	}
	return ctl
}
func comboAdd(ctl uintptr, text string) {
	call(pSend, ctl, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(ptr(text))))
}
func comboSelect(ctl uintptr, i int) { call(pSend, ctl, CB_SETCURSEL, uintptr(i), 0) }
func comboText(ctl uintptr) string {
	pos := int(call(pSend, ctl, CB_GETCURSEL, 0, 0))
	if pos < 0 || pos > 100 {
		return ""
	}
	buf := make([]uint16, 256)
	call(pSend, ctl, CB_GETLBTEXT, uintptr(pos), uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}
func value(ctl uintptr) string {
	n := call(pGetTextLength, ctl)
	if n > 50000 {
		n = 50000
	}
	buf := make([]uint16, n+1)
	call(pGetText, ctl, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}
func newDialog(kind, title string, w, h int32) *Dialog {
	if dlg != nil {
		return nil
	}
	d := &Dialog{kind: kind}
	x := (winW-w)/2 + 115
	if x < 100 {
		x = 100
	}
	y := (winH-h)/2 + 45
	if y < 40 {
		y = 40
	}
	class := ptr("MeuKanbanEditorWin32")
	d.hwnd = call(pCreate, 0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(ptr(title))), WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_CLIPCHILDREN, ut(x), ut(y), ut(w), ut(h), hwnd, 0, call(pGetModule, 0), 0)
	if d.hwnd == 0 {
		alert("Não foi possível abrir a janela de edição.")
		return nil
	}
	dlg = d
	return d
}
func showDialog(d *Dialog) {
	call(pEnable, hwnd, 0)
	call(pShow, d.hwnd, SW_SHOW)
	call(pUpdate, d.hwnd)
	if d.title != 0 {
		call(pFocus, d.title)
	}
}
func closeDialog() {
	if dlg == nil {
		return
	}
	old := dlg.hwnd
	dlg = nil
	call(pDestroy, old)
	call(pEnable, hwnd, 1)
	call(pSetFore, hwnd)
	call(pFocus, hwnd)
	invalidate()
}
func dialogProc(w uintptr, m uint32, wp, lp uintptr) uintptr {
	switch m {
	case WM_CLOSE:
		closeDialog()
		return 0
	case WM_COMMAND:
		if dlg == nil {
			return 0
		}
		id := wp & 0xffff
		if id == 501 {
			closeDialog()
			return 0
		}
		if id == 500 {
			submitDialog()
			return 0
		}
		if id == 502 {
			deleteTaskDialog()
			return 0
		}
	case WM_KEYDOWN:
		if wp == 27 {
			closeDialog()
			return 0
		}
	}
	return call(pDef, w, uintptr(m), wp, lp)
}
func submitDialog() {
	if dlg == nil {
		return
	}
	d := dlg
	if d.kind == "search" {
		filterText = strings.TrimSpace(value(d.title))
		scrollCols = [3]int32{}
		closeDialog()
		invalidate()
		return
	}
	if d.kind == "project" || d.kind == "rename" {
		name := strings.TrimSpace(value(d.title))
		if name == "" {
			alert("Informe um nome para o projeto.")
			return
		}
		if !uniqueProject(name, d.original) {
			alert("Já existe um projeto com esse nome.")
			return
		}
		if d.kind == "rename" {
			if name == "Geral" {
				alert("Use outro nome.")
				return
			}
			for i, p := range data.Projects {
				if p == d.original {
					data.Projects[i] = name
				}
			}
			for i := range data.Tasks {
				if data.Tasks[i].Project == d.original {
					data.Tasks[i].Project = name
				}
			}
			if currentProject == d.original {
				currentProject = name
			}
		} else {
			data.Projects = append(data.Projects, name)
		}
		closeDialog()
		saveStore()
		if d.kind == "project" {
			page = "board"
			currentProject = name
			scrollCols = [3]int32{}
		}
		invalidate()
		return
	}
	title := strings.TrimSpace(value(d.title))
	if title == "" {
		alert("Digite um título para a tarefa.")
		return
	}
	status := map[string]string{"A fazer": "todo", "Em andamento": "doing", "Concluído": "done"}[comboText(d.status)]
	if status == "" {
		status = "todo"
	}
	due := strings.TrimSpace(value(d.due))
	if due != "" {
		if _, err := time.Parse("2006-01-02", due); err != nil {
			if date, er := time.Parse("02/01/2006", due); er == nil {
				due = date.Format("2006-01-02")
			} else {
				alert("Digite o prazo como AAAA-MM-DD ou DD/MM/AAAA.")
				return
			}
		}
	}
	t := Task{Title: title, Details: value(d.details), Project: comboText(d.project), Priority: comboText(d.priority), Status: status, Due: due, Created: time.Now().Format(time.RFC3339)}
	if t.Priority == "" {
		t.Priority = "Normal"
	}
	if t.Project == "" {
		t.Project = "Geral"
	}
	if d.kind == "edittask" {
		updated := false
		for i := range data.Tasks {
			if data.Tasks[i].ID == d.editingID {
				t.ID = data.Tasks[i].ID
				t.Created = data.Tasks[i].Created
				data.Tasks[i] = t
				updated = true
				break
			}
		}
		if !updated {
			alert("Essa tarefa não foi encontrada.")
			return
		}
	} else {
		t.ID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), len(data.Tasks))
		data.Tasks = append(data.Tasks, t)
	}
	closeDialog()
	saveStore()
}
func deleteTaskDialog() {
	if dlg == nil || dlg.kind != "edittask" {
		return
	}
	if !confirm("Deseja excluir esta tarefa definitivamente?") {
		return
	}
	id := dlg.editingID
	out := make([]Task, 0, len(data.Tasks))
	for _, t := range data.Tasks {
		if t.ID != id {
			out = append(out, t)
		}
	}
	data.Tasks = out
	closeDialog()
	saveStore()
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runName = "MeuKanbanProjetos"

func registryKey() (uintptr, bool) {
	var h uintptr
	res := call(pRegOpen, 0x80000001, uintptr(unsafe.Pointer(ptr(runKey))), 0, 0x20003, uintptr(unsafe.Pointer(&h)))
	return h, res == 0
}
func getStartupValue(key uintptr) string {
	typ := uint32(0)
	n := uint32(4096)
	buf := make([]byte, n)
	res := call(pRegQuery, key, uintptr(unsafe.Pointer(ptr(runName))), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if res != 0 || n < 2 {
		return ""
	}
	u := make([]uint16, n/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(buf[i*2:])
	}
	return syscall.UTF16ToString(u)
}
func migrateStartup() {
	key, ok := registryKey()
	if !ok {
		return
	}
	defer call(pRegClose, key)
	val := getStartupValue(key)
	autos = val != ""
	if autos {
		exe, e := os.Executable()
		if e == nil && strings.Trim(val, "\"") != exe {
			_ = writeStartup(key, exe)
		}
	}
}
func writeStartup(key uintptr, exe string) bool {
	encoded := syscall.StringToUTF16(`"` + exe + `"`)
	return call(pRegSet, key, uintptr(unsafe.Pointer(ptr(runName))), 0, 1, uintptr(unsafe.Pointer(&encoded[0])), uintptr(len(encoded)*2)) == 0
}
func setStartup(on bool) {
	key, ok := registryKey()
	if !ok {
		alert("Não foi possível acessar a inicialização automática do Windows.")
		return
	}
	defer call(pRegClose, key)
	if on {
		exe, err := os.Executable()
		if err != nil || !writeStartup(key, exe) {
			alert("Não foi possível habilitar a inicialização.")
			return
		}
		autos = true
	} else {
		if call(pRegDelete, key, uintptr(unsafe.Pointer(ptr(runName)))) != 0 {
			alert("Não foi possível desativar a inicialização.")
			return
		}
		autos = false
	}
	invalidate()
}
func exportBackup() {
	src, err := os.Open(dataFile)
	if err != nil {
		alert("Não foi possível ler as tarefas para exportar.")
		return
	}
	defer src.Close()
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Documents")
	if s, e := os.Stat(dir); e != nil || !s.IsDir() {
		dir = home
	}
	target := filepath.Join(dir, "MeuKanban_backup_"+time.Now().Format("2006-01-02_150405")+".json")
	dst, err := os.Create(target)
	if err != nil {
		alert("Não foi possível salvar o backup: " + err.Error())
		return
	}
	_, err = io.Copy(dst, src)
	_ = dst.Close()
	if err != nil {
		alert("Erro ao exportar o backup: " + err.Error())
	} else {
		alert("Backup exportado para:\n" + target)
	}
}
