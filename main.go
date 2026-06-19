package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// rainbow に使う ANSI カラーコード（赤→黄→緑→シアン→青→マゼンタ）
var rainbowColors = []string{
	"\033[31m", "\033[33m", "\033[32m", "\033[36m", "\033[34m", "\033[35m",
}

const (
	colorReset = "\033[0m"
	pig        = "🐖"
	track      = "." // 通常モードでブタが歩く地面
	band       = "█" // 虹モードの色帯
)

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
		if *rainbow {
			lines := renderRainbow(inner, *count, step)
			fmt.Print("\r" + strings.Join(lines, "\n"))
			time.Sleep(delay)
			// 描画した行数ぶんカーソルを戻して次フレームで上書きする
			fmt.Printf("\033[%dA\r", len(lines)-1)
		} else {
			fmt.Print("\r" + renderLine(inner, *count, step, false))
			time.Sleep(delay)
		}
	}
}

// pigRow は地面 ground の上をブタが等間隔で左へ歩く1行を返す。
// 🐖 は体が左向きなので、step が増えるとブタ全体が左へ進み、左端に到達すると右から出てくる。
// colored が真ならブタ自身を虹色に塗る。
func pigRow(inner, count, step int, ground string, colored bool) string {
	cells := make([]string, inner)
	for i := range cells {
		cells[i] = ground
	}

	// 🐖 は2セル幅なので、右端の1セットは空けておき pos+1 が常に存在するようにする。
	// これで端でも行幅が変わらず、折り返さずにシームレスに流れる。
	span := max(inner-1, 1)
	spacing := span / count // ブタ同士の間隔
	for n := range count {
		// 左方向へ動かす（負にならないよう span を足してから剰余を取る）
		pos := ((n*spacing-step)%span + span) % span
		body := pig
		if colored {
			body = rainbowColors[(step+n)%len(rainbowColors)] + pig + colorReset
		}
		cells[pos] = body
		cells[pos+1] = "" // 2セル目の地面を消して幅を揃える（pos+1 は必ず範囲内）
	}
	return strings.Join(cells, "")
}

// renderLine は |...🐖...🐖...| の1行を返す（通常モード／statusline用）。
func renderLine(inner, count, step int, colored bool) string {
	return "|" + pigRow(inner, count, step, track, colored) + "|"
}

// renderRainbow はブタの行と、その下に虹の色帯を重ねた複数行を返す。
// ブタは虹の上を歩いているように見える。
func renderRainbow(inner, count, step int) []string {
	lines := []string{pigRow(inner, count, step, " ", false)}
	for _, color := range rainbowColors {
		lines = append(lines, color+strings.Repeat(band, inner)+colorReset)
	}
	return lines
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
