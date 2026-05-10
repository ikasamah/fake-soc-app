# fake-soc-app

映像撮影の小道具用、macOS 単体で動く GUI フェイクダッシュボード。
**ホストのターミナル設定（色・フォント・透明度）の影響を一切受けず**、独自の配色・フォント・レイアウトで全画面に描画する。

ジャンル感: コンサル × 金融 SOC（監視センター）/ Bloomberg ターミナル風。
起動 → 90 秒サイクルで `normal → degraded → incident → recovery` のドラマが流れる。

## 起動

```bash
open fake-soc.app
# または
./fake-soc-app
```

終了: `Esc` / `Q` / `Cmd-Q`

## 配布物

ビルド成果物は git 管理外（`.gitignore`）。各自でビルドする想定:

```bash
go build -o fake-soc-app .
~/go/bin/fyne package -os darwin -name "fake-soc" -appID "com.fakesoc.app"
```

`fyne` CLI が無い場合:
```bash
go install fyne.io/tools/cmd/fyne@latest
```

ビルド成果物:
- `fake-soc-app` — Mach-O arm64 single binary
- `fake-soc.app` — macOS app bundle（ダブルクリック起動）

配布する時は zip:
```bash
zip -r fake-soc.app.zip fake-soc.app
```

受け取った側は unzip → ダブルクリック。Gatekeeper 警告は右クリック → 開く で通る。

## 動作要件

- **macOS Apple Silicon**（M1 / M2 / M3 / M4）
- macOS Big Sur (11) 以降
- ネットワーク不要、追加 install 不要

## 機能

### 3 ペイン構成

```
+-----------------------------+-----------------------------+
|                             |  Sparkline (22 metric)      |
|                             |  + 中央ダイアログ overlay   |
|  LLM 風 streaming           |  + scenario 連動 spike      |
|  (金融コンサル prompt/resp) |  + session stats block      |
|                             +-----------------------------+
|                             |  色つき JSON ログ feed       |
|                             |  + ALERT bar (incident)     |
+-----------------------------+-----------------------------+
```

### シナリオサイクル（90 秒）

| シナリオ | 時間 | 全ペインの挙動 |
|---|---:|---|
| `normal` | 50s | 通常分析（revenue / AUM / NIM）/ 平和な sparkline / INFO 中心ログ |
| `degraded` | 10s | latency 上昇調査 / spike metrics 黄色化 / WARN 増 |
| `incident` | 20s | settlement 異常 / FX breach 分析 / spike metrics 赤で 80-100% / ERROR・FATAL・ALERT bar 流れる |
| `recovery` | 10s | drain progress / FAILOVER COMPLETED / INFO 復帰 |

### 解像度逆算（auto）

`--font-size` 未指定なら、起動時のキャンバスサイズから自動算出。target は左ペイン 80 cols / 右下 28 行に収まる font size。

```bash
./fake-soc-app                    # auto（推奨）
./fake-soc-app --font-size 24     # 手動指定
FAKESOC_FONT_SIZE=24 ./fake-soc-app
```

### 日本語対応

[Cica](https://github.com/miiton/Cica)（OFL）を `//go:embed` でバイナリに埋め込み。半角:全角 = 1:2 の monospace なので英日混在でグリッド整列が崩れない。

### 配色

`main.go` の冒頭定数で完全制御。撮影に合わせて変えるならここを編集:

```go
bgColor    = #0E0E10  // background
fgColor    = #C8C8CA  // body text
dimColor   = #6A6A6E  // labels
cyanColor  = #6EA8C8  // service names
greenColor = #88C090  // INFO / 上昇
amberColor = #D9A04E  // WARN
redColor   = #D86060  // ERROR / spike / 下降
```

## カスタマイズ

撮影シーンに合わせて差し替えるなら:

| 変えるもの | 編集場所 |
|---|---|
| LLM の prompts / responses | `content.go` の `responsesNormal()` 等 |
| Sparkline の metric 名 | `content.go` の `Metrics` |
| Service / Event 語彙 | `content.go` の `Services` / `Events*` |
| ダイアログ文言 | `content.go` の `Dialogs*` |
| ALERT 文言 | `content.go` の `AlertLines` |
| Stats 行（pnl / fx / etc.） | `main.go` の `stepStats()` / `buildStatsRows()` |
| シナリオ持続時間 | `main.go` の `scenarioCycle` |

差し替え後 `go build -o fake-soc-app . && ~/go/bin/fyne package -os darwin -name "fake-soc" -appID "com.fakesoc.app"`。

## ファイル構成

```
.
├── main.go             # window / theme / ticker / 各ペイン render
├── content.go          # prompts / responses / metric 名 / ログ vocab
├── go.mod / go.sum
├── Icon.png            # アプリアイコン (512x512)
├── assets/
│   ├── font.ttf            # Cica Regular (OFL)
│   └── cica-LICENSE.txt    # Cica の OFL ライセンス
└── README.md
```

## ライセンス

- このプロジェクト本体: 個人用途の撮影プロップ。気軽に fork / 改変して使ってください
- 同梱の Cica フォント: SIL Open Font License 1.1 — `assets/cica-LICENSE.txt` 参照。再配布時は同梱必須

## 既知の制約

- arm64 専用（Intel Mac は別途 universal binary ビルドが必要）
- Gatekeeper の「開発元未確認」警告は ad-hoc 署名のため発生 — 右クリック開くで通る。完全に黙らせるには Apple Developer 署名（年 $99）必要
- フルスクリーンは macOS native fullscreen（別 space）— dock とメニューバーが消える
- ホットリロードなし（コード変更後は再ビルド）
