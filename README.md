# pig-run 🐖💨

ターミナルでブタが走り続ける、小さなアニメーション CLI です。
コマンドとして単体で動かせるほか、Claude Code のステータスラインに組み込んで
「エージェント実行中にブタが走っている」状態にもできます。

## インストール

Go 1.26 以降が必要です。

```sh
# GitHub から直接インストール（誰でもこれで入る）
go install github.com/sukemaru-injp/pig-run@latest

# リポジトリを clone してローカルにビルドする場合
git clone https://github.com/sukemaru-injp/pig-run.git
cd pig-run
go build -o pig-run .
```

`go install` で入れた場合、`$GOPATH/bin`（通常 `~/go/bin`）に PATH が通っている必要があります。
通っていなければシェルの設定に追記してください。

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

## CLI として使う

```sh
pig-run                                  # 等速で1匹
pig-run --speed 50                       # 速く走らせる
pig-run --rainbow                        # カラフルに
pig-run --count 10                       # 10匹を一斉に
pig-run --speed 30 --count 5 --rainbow --width 8   # 自由に組み合わせ
```

`Ctrl+C` で終了します（カーソル表示も元に戻ります）。

### オプション一覧

| フラグ | デフォルト | 説明 |
| --- | --- | --- |
| `--speed` | `10` | アニメーション速度。大きいほど速い（待ち時間 = `1200/speed` ms。`10` で従来の 120ms 相当） |
| `--count` | `1` | 走らせるブタの数。各ブタは位相をずらして走る |
| `--rainbow` | `false` | フレームごとに色を巡回させてカラフルに表示 |
| `--width` | `4` | 走るトラックの幅（往復する距離） |
| `--once` | `false` | 1フレームだけ出力して即終了。statusline などから繰り返し呼ぶ用 |

## Claude Code に組み込む

Claude Code は **ステータスラインの更新のたびに登録コマンドを1回実行**し、その標準出力を
画面下部に表示します。`--once` モードは現在時刻からコマ位置を決めて1フレームだけ出力するので、
更新のたびに次のコマが表示され、エージェント稼働中にブタが走っているように見えます。

`~/.claude/settings.json`（プロジェクト単位なら `.claude/settings.json`）に追記します。

```json
{
  "statusLine": {
    "type": "command",
    "command": "pig-run --once --rainbow"
  }
}
```

速度や匹数も渡せます。

```json
{
  "statusLine": {
    "type": "command",
    "command": "pig-run --once --speed 30 --count 2"
  }
}
```

> [!NOTE]
> ステータスラインはエージェントの活動（メッセージ更新など）に合わせて再描画されます。
> 入力待ちなどで更新が止まっている間はコマも止まりますが、ツール実行中などは
> 頻繁に更新されるため、ちょうど「動いている間だけ走る」挙動になります。

## 仕組みのメモ

- フレーム位置は「左→右→左」の往復として生成（`buildPositions`）。
- 常駐モードは複数行をカーソル制御（`\033[nA`）で再描画してアニメーションする。
- `--once` モードはカーソル制御をせず1行を出力するだけなので、statusline に安全に流し込める。
