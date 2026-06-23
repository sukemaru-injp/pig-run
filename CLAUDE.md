# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`pig-run` is a single-file Go CLI (`main.go`) that animates pigs (🐖) walking leftward
across a terminal-width track. With `--rainbow`, the ground is a flowing HSV gradient and
pigs walk on top of it. The CLI also doubles as a Claude Code statusline command via `--once`.

Code comments and the README are written in Japanese; match that convention when editing.

## Commands

```sh
go build -o pig-run .   # build the binary (output is gitignored)
go run . --rainbow      # run without building
go vet ./...            # static checks
gofmt -w main.go        # format
```

There are no tests in this repo. Requires Go 1.26+.

## Architecture

Everything lives in `main.go`. Three distinct run modes branch off `main`:

- **`--farm` mode**: instead of a single track, draws a box (`farmRows`=8 rows tall × terminal
  width) where `count` pigs wander freely. `runFarm` redraws the whole box each frame by moving
  the cursor up `farmRows+2` lines (`\033[NA`). Pigs (`farmPig`: position + `dx/dy` direction)
  bounce off walls in `stepFarmPigs` and occasionally change direction at random. `newFarmGrass`
  sprinkles static 🌱 tufts; `renderFarm` draws pigs first, then grass only where no pig overlaps
  (both glyphs are 2 cells wide and blank `x+1`, so overlapping spans would corrupt the line width).
  `--once` ignores `--farm` (statusline must stay one line).


- **Continuous mode** (default): an infinite loop prints one line per `step`, prefixing each
  frame with `\r` to overwrite the previous line, sleeping `delay` between frames. Hides the
  cursor and restores it on `Ctrl+C` via `handleSignals`/`cursorVisible`.
- **`--once` mode**: derives `step` from wall-clock time (`UnixMilli / delayMs`), prints a
  single frame with no cursor control, and exits. This is what makes it safe to embed in a
  statusline (Claude Code re-runs the command on each redraw, so the pig appears to move).

Frame rendering pipeline:

- `pigPositions(inner, count, step)` — the core animation math. Returns each pig's left-cell
  index, evenly spaced and shifted left as `step` grows, wrapping modulo `span`. Pigs are
  2 cells wide, so positions stay within `inner-1` and `pos+1` is always in range.
- `pigRow` / `rainbowGround` — build the cell slice. Both blank out `cells[pos+1]` (set to `""`)
  to compensate for the pig glyph's 2-cell width so total line width stays constant and the
  track never wraps.
- `renderLine` — wraps a row in `|...|`; dispatches to `rainbowGround` or `pigRow`.
- `hueAt` → `fgColor`/`bgColor` → `colorEscape` → `hsvToRGB` — color path for rainbow mode.
  `colorEscape` emits 24-bit truecolor when `COLORTERM` is `truecolor`/`24bit`, otherwise
  quantizes to the 256-color cube (the `trueColor` package var caches this detection).

Width handling: `terminalWidth()` uses a `TIOCGWINSZ` ioctl on stdout, falling back to the
`COLUMNS` env var then `80` when stdout isn't a terminal (e.g. piped). `inner` is the usable
cell count after reserving the `|` borders plus margin.

## Notes / gotchas

- The README's "仕組みのメモ" section is stale — it references `buildPositions` and a multi-line
  back-and-forth animation that no longer exist. The current code uses `pigPositions` and renders
  a single left-scrolling line. Don't treat that section as ground truth.
- `delay` is `1200/speed` ms (min 1); `speed=10` ≈ 120ms/frame.
- The committed `pig-run` binary in the repo root is a build artifact; rebuild rather than trust it.
</content>
</invoke>
