# fake-soc-app

ターミナル app から完全に独立した GUI 版。Fyne で全画面ウィンドウを開いて、独自の配色・フォント・レイアウトでダッシュボードを描画する。**ホストのターミナル設定（色テーマ・フォント・透明度など）は一切影響しない。**

## 配布物

| ファイル | 用途 |
|---|---|
| `fake-soc-app` | Mach-O 64bit arm64 バイナリ。`./fake-soc-app` で起動 |
| `fake-soc.app` | macOS アプリバンドル。Finder からダブルクリック起動 |

## 起動

### ダブルクリック

```
~/Downloads/fake-soc-app/fake-soc.app
```

Finder で `fake-soc.app` をダブルクリックすれば全画面ウィンドウが開く。

初回起動で macOS の Gatekeeper に弾かれる場合は:
1. システム設定 → プライバシーとセキュリティ → 「fake-soc を開く」を許可
2. または右クリック → 開く

### コマンド経由

```bash
~/Downloads/fake-soc-app/fake-soc-app
```

## 終了

- `Esc` キー
- `Q` キー
- `Cmd-Q`

## 機能

shell 版 / bubbletea 版と同じシナリオサイクル:

- normal 50s → degraded 10s → incident 20s → recovery 10s
- 左ペイン: LLM 風 streaming（金融コンサル文脈、シナリオ別 prompts/responses）
- 右上: 22 metric の sparkline + 中央ダイアログ + spike
- 右下: 色つき JSON-ish ログ + ALERT bar

## 配色

`main.go` の冒頭に定数として定義:

```go
bgColor      = #0E0E10  // background
fgColor      = #C8C8CA  // body text
dimColor     = #6A6A6E  // dim text / labels
cyanColor    = #6EA8C8  // service names
greenColor   = #88C090  // INFO
blueColor    = #7090C0  // DEBUG
amberColor   = #D9A04E  // WARN
redColor     = #D86060  // ERROR / spike metrics
redBgColor   = #6E1618  // FATAL background / ALERT bar
```

色を変えたければここを編集して `go build -o fake-soc-app .`、または `~/go/bin/fyne package -os darwin -name "fake-soc" -appID "com.fakesoc.app"` で .app 再ビルド。

## フォント

現状は Fyne 内蔵の monospace（Roboto Mono 系）。撮影で別フォント（IBM Plex Mono / JetBrains Mono / Berkeley Mono など）を使いたければ:

1. TrueType ファイルを `assets/font.ttf` に配置
2. `//go:embed assets/font.ttf` で埋め込み
3. theme `Font()` メソッドで `fyne.NewStaticResource("font.ttf", data)` を返す

## 既知の制約

- 起動直後の 1〜2 秒はサイズ計算が暫定値で、sparkline 列数が変動して見える
- フルスクリーンは macOS の native fullscreen（別 space）。dock やメニューバーが消える
- 文字幅・行高は近似（7.5px / 16px）で sparkline columns を計算しているので、実フォントと完全一致はしない。撮影には支障ないレベル
- ホットリロードはなし（コード変更後は再 build）

## ファイル構成

```
~/Downloads/fake-soc-app/
├── main.go         # Fyne app: window, theme, ticker, render
├── content.go      # prompts / responses / metrics / log vocab
├── go.mod / go.sum
├── Icon.png        # アプリアイコン (512x512)
├── fake-soc-app    # ビルド済みバイナリ
└── fake-soc.app    # macOS アプリバンドル
```

## 既存版との関係

| 版 | パス | 起動方法 | 用途 |
|---|---|---|---|
| shell + tmux | `~/Downloads/fake-soc/` | `./start.sh` | コンテンツ即編集したい時 |
| bubbletea ターミナル | `~/Downloads/fake-soc-go/` | `./fake-soc-go` | ターミナル内動作の Go 版 |
| **Fyne GUI** | `~/Downloads/fake-soc-app/` | **`fake-soc.app` ダブルクリック** | **撮影本番（独立ウィンドウ、配色完全制御）** |
