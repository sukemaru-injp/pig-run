package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
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
	farmRows   = 8   // --farm の牧場の縦のマス数
	grass      = "🌱" // 牧場にところどころ生える草
	grassDens  = 14  // 草の密度（マス grassDens 個につき約1株）
)

// farmDirs はブタが進む方向の候補（-1, 0, +1）。
var farmDirs = []int{-1, 0, 1}

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
	farm := flag.Bool("farm", false, "縦8マスの牧場の中をブタたちが自由に動き回る")
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
	delayMs := max(1200 / *speed, 1)
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

	// --farm: 縦 farmRows・横 inner の牧場の中をブタたちが歩き回る
	if *farm {
		runFarm(inner, *count, *rainbow, delay)
		return
	}

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

// farmPig は牧場の中を歩き回る1頭のブタの状態。
// x は左端セル位置（🐖 は2セル幅なので右隣 x+1 も占有する）、y は行。
// dx/dy は現在向かっている方向（-1/0/+1）。
type farmPig struct {
	x, y   int
	dx, dy int
}

// farmPos は牧場内の固定位置（草など、動かないもの）を表す。
type farmPos struct {
	x, y int
}

// runFarm は牧場アニメーションのメインループ。
// 牧場は border + farmRows 行 + border の計 farmRows+2 行を占有するので、
// 2フレーム目以降はカーソルをその行数だけ上に戻してから描き直す。
func runFarm(inner, count int, rainbow bool, delay time.Duration) {
	rows := farmRows
	pigs := newFarmPigs(count, inner, rows)
	grasses := newFarmGrass(inner, rows) // 草は最初に生やしたら動かさない
	totalLines := rows + 2               // 上下の枠

	for step := 0; ; step++ {
		if step > 0 {
			fmt.Printf("\033[%dA\r", totalLines) // 牧場の先頭行までカーソルを戻す
		}
		fmt.Println(renderFarm(pigs, grasses, inner, rows, step, rainbow))
		stepFarmPigs(pigs, inner, rows)
		time.Sleep(delay)
	}
}

// newFarmGrass は牧場のあちこちに草をランダムに（重ならないように）生やす。
// 🌱 も2セル幅なので、隣のセルと合わせて2マス分を確保する。
func newFarmGrass(inner, rows int) []farmPos {
	maxX := max(inner-2, 0)
	occupied := make([][]bool, rows)
	for r := range occupied {
		occupied[r] = make([]bool, inner)
	}
	target := max(inner*rows/grassDens, 1)
	grasses := make([]farmPos, 0, target)
	for attempt := 0; attempt < target*5 && len(grasses) < target; attempt++ {
		x := rand.Intn(maxX + 1)
		y := rand.Intn(rows)
		if occupied[y][x] || occupied[y][x+1] {
			continue
		}
		occupied[y][x], occupied[y][x+1] = true, true
		grasses = append(grasses, farmPos{x: x, y: y})
	}
	return grasses
}

// newFarmPigs は牧場内のランダムな位置・向きにブタを配置する。
func newFarmPigs(count, inner, rows int) []farmPig {
	maxX := max(inner-2, 0) // x+1 が範囲内に収まる右端
	pigs := make([]farmPig, count)
	for n := range pigs {
		pigs[n] = farmPig{
			x:  rand.Intn(maxX + 1),
			y:  rand.Intn(rows),
			dx: farmDirs[rand.Intn(len(farmDirs))],
			dy: farmDirs[rand.Intn(len(farmDirs))],
		}
	}
	return pigs
}

// stepFarmPigs は各ブタを1フレーム分動かす。
// たまに向きをランダムに変え、壁にぶつかると跳ね返る。
func stepFarmPigs(pigs []farmPig, inner, rows int) {
	maxX := max(inner-2, 0)
	maxY := rows - 1
	for i := range pigs {
		p := &pigs[i]
		if rand.Intn(8) == 0 { // 約1/8の確率で気まぐれに方向転換
			p.dx = farmDirs[rand.Intn(len(farmDirs))]
			p.dy = farmDirs[rand.Intn(len(farmDirs))]
		}
		if nx := p.x + p.dx; nx < 0 || nx > maxX {
			p.dx = -p.dx // 左右の壁で反射
		}
		if ny := p.y + p.dy; ny < 0 || ny > maxY {
			p.dy = -p.dy // 上下の壁で反射
		}
		p.x = clamp(p.x+p.dx, 0, maxX)
		p.y = clamp(p.y+p.dy, 0, maxY)
	}
}

// renderFarm は牧場全体（枠＋中身）の複数行文字列を返す。
// rainbow が真なら地面を虹のグラデーションにし、ブタはその上に立つ。
func renderFarm(pigs []farmPig, grasses []farmPos, inner, rows, step int, rainbow bool) string {
	grid := make([][]string, rows)
	for r := range grid {
		row := make([]string, inner)
		for i := range row {
			if rainbow {
				row[i] = fgColor(hueAt(i, step)) + band + colorReset
			} else {
				row[i] = track
			}
		}
		grid[r] = row
	}

	// ブタを置き、占有しているセル（本体 x と幅合わせの x+1）を記録する。
	occupied := make([][]bool, rows)
	for r := range occupied {
		occupied[r] = make([]bool, inner)
	}
	for _, p := range pigs {
		if rainbow {
			grid[p.y][p.x] = bgColor(hueAt(p.x, step)) + pig + colorReset
		} else {
			grid[p.y][p.x] = pig
		}
		grid[p.y][p.x+1] = "" // 2セル目を消して幅を揃える（x+1 は必ず範囲内）
		occupied[p.y][p.x], occupied[p.y][p.x+1] = true, true
	}

	// 草はブタと重ならない場所にだけ描く（重なると2セル幅が崩れて行幅が狂うため）。
	for _, g := range grasses {
		if occupied[g.y][g.x] || occupied[g.y][g.x+1] {
			continue
		}
		grid[g.y][g.x] = grass
		grid[g.y][g.x+1] = ""
	}

	border := "+" + strings.Repeat("-", inner) + "+"
	var b strings.Builder
	b.WriteString(border + "\n")
	for _, row := range grid {
		b.WriteString("|" + strings.Join(row, "") + "|\n")
	}
	b.WriteString(border)
	return b.String()
}

// clamp は v を [lo, hi] の範囲に収める。
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
