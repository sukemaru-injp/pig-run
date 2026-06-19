package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	colorReset = "\033[0m"
	pig        = "🐖"
	track      = "." // 通常モードでブタが歩く地面
	band       = "█" // 虹モードの色帯（1セル分の塗り）
	huePeriod  = 28  // 虹が1周するまでの桁数。小さいほど色の変化が急になる
)

// trueColor はターミナルが24bitカラーに対応していれば真。
// 非対応（Apple Terminal 等）では256色にフォールバックする。
var trueColor = func() bool {
	ct := os.Getenv("COLORTERM")
	return ct == "truecolor" || ct == "24bit"
}()

func main() {
	speed := flag.Int("speed", 10, "アニメーション速度（大きいほど速い）")
	rainbow := flag.Bool("rainbow", false, "カラフルに表示する")
	count := flag.Int("count", 3, "歩かせるブタの数")
	width := flag.Int("width", 0, "トラックの幅（0でターミナル幅いっぱい）")
	once := flag.Bool("once", false, "1フレームだけ出力して終了（statusline等から呼ぶ用）")
	flag.Parse()

	if *speed < 1 {
		*speed = 1
	}
	if *count < 1 {
		*count = 1
	}

	// トラックの内側の幅（両端の | を除いたセル数）。
	// 自動幅では右端で折り返さないよう1桁余裕を持たせる。
	inner := *width - 2
	if *width <= 0 {
		inner = terminalWidth() - 3
	}
	if inner < *count*2 {
		inner = *count * 2 // 最低でもブタが並べる幅は確保する
	}

	// speed が大きいほど待ち時間が短くなる（speed=10 で 120ms 相当）
	delayMs := max(1200/ *speed, 1)
	delay := time.Duration(delayMs) * time.Millisecond

	// --once: 現在時刻からフレームを決めて出力し終了する。
	// statusline は1行が前提なので、虹モードでも1行（色付きブタ）で返す。
	if *once {
		step := int(time.Now().UnixMilli() / int64(delayMs))
		fmt.Print(renderLine(inner, *count, step, *rainbow))
		return
	}

	// Ctrl+C で抜けたときにカーソルを元に戻して終了する
	defer cursorVisible(true)
	cursorVisible(false)
	handleSignals()

	for step := 0; ; step++ {
		fmt.Print("\r" + renderLine(inner, *count, step, *rainbow))
		time.Sleep(delay)
	}
}

// pigPositions はブタ各個体の左端セル位置を返す（左方向へ流れる）。
// 🐖 は2セル幅なので、右端の1セットは空けておき pos+1 が常に範囲内に収まるようにする。
// これで端でも行幅が変わらず、折り返さずにシームレスに流れる。
func pigPositions(inner, count, step int) []int {
	span := max(inner-1, 1)
	spacing := span / count // ブタ同士の間隔
	pos := make([]int, count)
	for n := range count {
		// 左方向へ動かす（負にならないよう span を足してから剰余を取る）
		pos[n] = ((n*spacing-step)%span + span) % span
	}
	return pos
}

// pigRow は地面 ground の上をブタが等間隔で左へ歩く1行を返す。
// 🐖 は体が左向きなので、step が増えるとブタ全体が左へ進み、左端に到達すると右から出てくる。
func pigRow(inner, count, step int, ground string) string {
	cells := make([]string, inner)
	for i := range cells {
		cells[i] = ground
	}
	for _, pos := range pigPositions(inner, count, step) {
		cells[pos] = pig
		cells[pos+1] = "" // 2セル目の地面を消して幅を揃える（pos+1 は必ず範囲内）
	}
	return strings.Join(cells, "")
}

// renderLine は |...🐖...🐖...| の1行を返す（通常モード／statusline用）。
// rainbow が真なら地面をなめらかな虹のグラデーションにする。
func renderLine(inner, count, step int, rainbow bool) string {
	if rainbow {
		return "|" + rainbowGround(inner, count, step) + "|"
	}
	return "|" + pigRow(inner, count, step, track) + "|"
}

// rainbowGround は1セルごとに色相をずらした虹のグラデーション地面の上を、
// ブタが歩く1行を返す。step とともに色帯が右へ流れる。
func rainbowGround(inner, count, step int) string {
	cells := make([]string, inner)
	for i := range cells {
		cells[i] = fgColor(hueAt(i, step)) + band + colorReset
	}
	// ブタは足元の色を背景に敷いて、虹の上に立っているように見せる
	for _, pos := range pigPositions(inner, count, step) {
		cells[pos] = bgColor(hueAt(pos, step)) + pig + colorReset
		cells[pos+1] = ""
	}
	return strings.Join(cells, "")
}

// hueAt は桁 col・フレーム step における色相（0〜360度）を返す。
// step が増えるほどパターンが右へ流れる。
func hueAt(col, step int) float64 {
	h := math.Mod(float64(col-step)/huePeriod*360, 360)
	if h < 0 {
		h += 360
	}
	return h
}

// fgColor / bgColor は色相から前景色・背景色の ANSI エスケープを返す。
func fgColor(hue float64) string { return colorEscape(hue, 38) }
func bgColor(hue float64) string { return colorEscape(hue, 48) }

// colorEscape は色相を前景(38)/背景(48)レイヤーの ANSI エスケープに変換する。
// truecolor 対応端末では24bit、非対応端末では256色 color cube に量子化する。
func colorEscape(hue float64, layer int) string {
	r, g, b := hsvToRGB(hue)
	if trueColor {
		return fmt.Sprintf("\033[%d;2;%d;%d;%dm", layer, r, g, b)
	}
	q := func(v int) int { return (v*5 + 127) / 255 } // 0-255 を 0-5 に丸める
	idx := 16 + 36*q(r) + 6*q(g) + q(b)
	return fmt.Sprintf("\033[%d;5;%dm", layer, idx)
}

// hsvToRGB は彩度・明度を最大とした色相 hue（度）を RGB(0-255) に変換する。
func hsvToRGB(hue float64) (int, int, int) {
	x := 1 - math.Abs(math.Mod(hue/60, 2)-1)
	var r, g, b float64
	switch {
	case hue < 60:
		r, g, b = 1, x, 0
	case hue < 120:
		r, g, b = x, 1, 0
	case hue < 180:
		r, g, b = 0, 1, x
	case hue < 240:
		r, g, b = 0, x, 1
	case hue < 300:
		r, g, b = x, 0, 1
	default:
		r, g, b = 1, 0, x
	}
	return int(r * 255), int(g * 255), int(b * 255)
}

// terminalWidth は標準出力に繋がった端末の桁数を返す。取得できなければ 80。
func terminalWidth() int {
	type winsize struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	ws := &winsize{}
	ret, _, _ := syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)
	if int(ret) == 0 && ws.Col > 0 {
		return int(ws.Col)
	}
	// 端末でない場合（パイプ等）は COLUMNS を見て、無ければ 80
	if c, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && c > 0 {
		return c
	}
	return 80
}

// cursorVisible はターミナルのカーソル表示/非表示を切り替える
func cursorVisible(visible bool) {
	if visible {
		fmt.Print("\033[?25h")
	} else {
		fmt.Print("\033[?25l")
	}
}

// handleSignals は Ctrl+C 等を受けたときにカーソルを戻して綺麗に終了する
func handleSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cursorVisible(true)
		fmt.Println()
		os.Exit(0)
	}()
}
