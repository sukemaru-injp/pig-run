package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// rainbow に使う ANSI カラーコード（赤→黄→緑→シアン→青→マゼンタ）
var rainbowColors = []string{
	"\033[31m", "\033[33m", "\033[32m", "\033[36m", "\033[34m", "\033[35m",
}

const colorReset = "\033[0m"

func main() {
	speed := flag.Int("speed", 10, "アニメーション速度（大きいほど速い）")
	rainbow := flag.Bool("rainbow", false, "カラフルに表示する")
	count := flag.Int("count", 1, "走らせるブタの数")
	width := flag.Int("width", 4, "走るトラックの幅")
	once := flag.Bool("once", false, "1フレームだけ出力して終了（statusline等から呼ぶ用）")
	flag.Parse()

	if *speed < 1 {
		*speed = 1
	}
	if *count < 1 {
		*count = 1
	}
	if *width < 1 {
		*width = 1
	}

	// speed が大きいほど待ち時間が短くなる（speed=10 で 120ms 相当）
	delayMs := max(1200/ *speed, 1)
	delay := time.Duration(delayMs) * time.Millisecond

	// アニメーションのフレーム位置（左→右→左の往復）
	positions := buildPositions(*width)

	// --once: 現在時刻からフレームを決めて1行だけ出力し終了する。
	// statusline が更新するたびに次のコマが表示され、結果的にアニメして見える。
	if *once {
		step := int(time.Now().UnixMilli() / int64(delayMs))
		var parts []string
		for pig := 0; pig < *count; pig++ {
			pos := positions[(step+pig*2)%len(positions)]
			line := strings.Repeat(" ", pos) + "🐖" + strings.Repeat(" ", *width-pos) + "💨"
			if *rainbow {
				color := rainbowColors[(step+pig)%len(rainbowColors)]
				line = color + line + colorReset
			}
			parts = append(parts, line)
		}
		fmt.Print(strings.Join(parts, " "))
		return
	}

	// Ctrl+C で抜けたときにカーソルを元に戻して終了する
	cursorVisible(true)
	defer cursorVisible(true)
	cursorVisible(false)
	handleSignals()

	step := 0
	for {
		var b strings.Builder
		for pig := 0; pig < *count; pig++ {
			// ブタごとに位置の位相をずらして、バラけて走るようにする
			pos := positions[(step+pig*2)%len(positions)]
			line := strings.Repeat(" ", pos) + "🐖" + strings.Repeat(" ", *width-pos) + "💨"

			if *rainbow {
				color := rainbowColors[(step+pig)%len(rainbowColors)]
				line = color + line + colorReset
			}
			b.WriteString(line)
			if pig < *count-1 {
				b.WriteByte('\n')
			}
		}

		fmt.Print(b.String())
		time.Sleep(delay)

		// 次のフレーム描画前にカーソルを先頭へ戻す
		if *count > 1 {
			fmt.Printf("\033[%dA\r", *count-1)
		} else {
			fmt.Print("\r")
		}
		step++
	}
}

// buildPositions は左→右→左の往復となる位置の並びを生成する
func buildPositions(width int) []int {
	positions := make([]int, 0, width*2)
	for i := 0; i <= width; i++ {
		positions = append(positions, i)
	}
	for i := width - 1; i > 0; i-- {
		positions = append(positions, i)
	}
	return positions
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
