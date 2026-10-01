# TinyGo × M5Stack CoreS3 / CoreS3 Lite

M5Stack CoreS3 / CoreS3 Lite（ESP32-S3）の内蔵ハードウェアを [TinyGo](https://tinygo.org/) で動かし、
M5Stack 公式の [CoreS3 Arduino チュートリアル](https://docs.m5stack.com/ja/arduino/m5cores3/display)を Go に移植したものです。

M5Unified / M5GFX は移植していません。レジスタ値や初期化の手順を参照資料として読み、CoreS3 用の薄い Go パッケージを自作しています。
参照した箇所は、ソースコードのコメントに `ref:` として書いてあります。

| ドキュメント | 内容 |
|---|---|
| [docs/architecture.md](docs/architecture.md) | コード解説（層の構成、起動の流れ、SPI 共有、DMA など。図付き） |
| [docs/notes.md](docs/notes.md) | 調査メモ・ハマりどころの記録 |

## 動作状況

すべて CoreS3 Lite の実機で確認済みです（2026-10-01）。

| サンプル | 内容 | 公式チュートリアル |
|---|---|---|
| [00_i2c_scan](examples/00_i2c_scan) | 内部 I2C バスのスキャン | — |
| [00_power_on](examples/00_power_on) | 電源投入シーケンスとレジスタのダンプ | — |
| [01_display](examples/01_display) | ランダムな円と四角の描画 | [Display](https://docs.m5stack.com/ja/arduino/m5cores3/display) |
| [02_touch](examples/02_touch) | タッチ状態（touch / hold / flick / drag）と座標 | [Touch](https://docs.m5stack.com/ja/arduino/m5cores3/touch) |
| [03_button](examples/03_button) | 電源ボタンのクリック検出 | [Button](https://docs.m5stack.com/ja/arduino/m5cores3/button) |
| [04_rtc](examples/04_rtc) | RTC の日時表示（NTP なし、直接セット） | [RTC](https://docs.m5stack.com/ja/arduino/m5cores3/rtc) |
| [05_ltr553](examples/05_ltr553) | 近接・照度センサ | [LTR553](https://docs.m5stack.com/ja/arduino/m5cores3/ltr553) |
| [06_power](examples/06_power) | 充電状態・電池電圧・残量・VBUS 電圧 | [Power](https://docs.m5stack.com/ja/arduino/m5cores3/power) |
| [07_imu](examples/07_imu) | 加速度・ジャイロ・地磁気の 9 値 | [IMU](https://docs.m5stack.com/ja/arduino/m5cores3/imu) |
| [08_wakeup](examples/08_wakeup) | タッチで電源オフ、5 秒後に RTC で復帰 | [Wakeup](https://docs.m5stack.com/ja/arduino/m5cores3/wakeup) |
| [09_sdcard](examples/09_sdcard) | microSD の書き込みと PNG の交互表示 | [microSD](https://docs.m5stack.com/ja/arduino/m5cores3/sdcard) |
| [10_pic](examples/10_pic) | microSD の PNG 画像の表示 | [Image Files](https://docs.m5stack.com/en/arduino/m5cores3/pic) |
| [11_speaker](examples/11_speaker) | 10kHz / 4kHz のトーン | [Speaker](https://docs.m5stack.com/ja/arduino/m5cores3/speaker) |
| [12_mic](examples/12_mic) | 録音・波形表示・タップで再生 | [MIC](https://docs.m5stack.com/ja/arduino/m5cores3/mic) |
| [13_wav](examples/13_wav) | microSD の WAV 再生 | [Audio Files](https://docs.m5stack.com/ja/arduino/m5cores3/wav) |
| [14_camera](examples/14_camera) | カメラのプレビューと写真撮影（BMP で microSD に保存） | [Camera](https://docs.m5stack.com/ja/arduino/m5cores3/camera) |

公式との違いは「[公式チュートリアルとの違い](#公式チュートリアルとの違い)」を見てください。

## 必要なもの

- M5Stack CoreS3 または CoreS3 Lite
- TinyGo 0.42 以降（0.42.0 で確認）。ターゲットは汎用の `esp32s3-generic` を使います
- Go 1.27 以降（TinyGo が使う Go）
- 09_sdcard / 10_pic / 13_wav / 14_camera には microSD カード（**FAT32 / FAT16**。exFAT は不可）

```sh
tinygo version
tinygo targets | grep esp32s3-generic
```

Linux でシリアルポートが開けないときは、ユーザーが `uucp` グループ（ディストリビューションによっては `dialout`）に入っているか確認してください。

## 使い方

```sh
# ビルドだけ
tinygo build -target=esp32s3-generic -o out.bin ./examples/01_display

# 書き込み
tinygo flash -target=esp32s3-generic ./examples/01_display

# シリアル出力を見る
tinygo monitor
```

どのサンプルも、起動後 2 秒待ってから処理を始めます（シリアルモニタをつなぐ時間を取るため）。
書き込めないときは、**リセットボタンを約 3 秒長押しし、緑の LED が点いたら離して**ダウンロードモードにしてください。

### RTC の時刻を合わせる（04_rtc）

RTC は UTC で時刻を持ちます。ビルド時に日時を渡すと、その時刻をセットします（書き込みと起動の時間を見込んで約 9 秒進めています）。

```sh
tinygo flash -target=esp32s3-generic \
  -ldflags="-X main.setTime=$(date -u -d '+9 sec' +%Y-%m-%dT%H:%M:%SZ)" ./examples/04_rtc
```

日時を渡さず、RTC の時刻が無効（電源が完全に切れていた）な場合は、公式サンプルと同じ 2021-12-31 12:34:56 をセットします。

### microSD に素材を置く（09_sdcard / 10_pic / 13_wav）

公式チュートリアルが使う画像と音声を、カードを抜かずに本体から書き込むツールがあります。
**同じ名前のファイルが既にあれば上書きしません**（例外: sdput 自身が以前の版で生成した `/sample-12s.wav` は新しい版に置き換えます）。

```sh
./tools/sdput/fetch.sh                                # 公式のテスト画像 2 枚をダウンロード
tinygo flash -target=esp32s3-generic ./tools/sdput   # カードに書き込む
```

書き込まれるファイル:

| ファイル | 内容 |
|---|---|
| `/cores3_test_picture01.png`, `/cores3_test_picture02.png` | 公式のテスト画像（320×240） |
| `/TestPicture01.png`, `/TestPicture02.png` | 上と同じ画像（Image Files チュートリアル用の名前。公式には配布されていない） |
| `/sample-12s.wav` | 本体上で生成する 12 秒のテスト音（公式には配布されていない） |

## ディレクトリ構成

```
.
├── cores3/      CoreS3 ボードパッケージ（ピン定義、電源投入、LCD、タッチ、ボタン、RTC、IMU、SD、音声、カメラ）
├── drivers/     チップ単位のドライバ（ボード非依存。tinygo-org/drivers の流儀 New → Configure → 読み書き）
├── internal/    ESP32-S3 のペリフェラル（I2S、カメラ）と小物（描画、PNG、WAV、BMP）
├── examples/    公式チュートリアルの移植（1 本 1 ディレクトリ）
├── tools/sdput/ microSD に素材を置くツール
└── docs/        コード解説（architecture.md）、調査メモ（notes.md）
```

### ドライバ

| パッケージ | チップ | 用途 |
|---|---|---|
| [drivers/axp2101](drivers/axp2101) | X-Powers AXP2101 | 電源管理（LDO、充電、電池、電源ボタン、電源オフ） |
| [drivers/aw9523b](drivers/aw9523b) | Awinic AW9523B | I/O エキスパンダ（各種リセット線、5V 出力） |
| [drivers/ili9342](drivers/ili9342) | ILI9342C / ILI9342E | LCD（C と E を自動判別） |
| [drivers/bm8563](drivers/bm8563) | BM8563（PCF8563 互換） | RTC（タイマー割り込み） |
| [drivers/ltr553](drivers/ltr553) | Lite-On LTR-553ALS | 近接・照度センサ |
| [drivers/bmi270](drivers/bmi270) | Bosch BMI270 | 6 軸 IMU（補助 I2C 経由の地磁気センサ） |
| [drivers/aw88298](drivers/aw88298) | Awinic AW88298 | スピーカーアンプ |
| [drivers/es7210](drivers/es7210) | Everest ES7210 | マイク用 ADC |
| [drivers/gc0308](drivers/gc0308) | GalaxyCore GC0308 | カメラ |

tinygo-org/drivers から `sdcard`（microSD）、tinyfs から `fatfs`（FAT）、tinyfont（文字）を使っています。

### ESP32-S3 のペリフェラル

TinyGo（0.42）に実装がないものを、ESP-IDF の低レベルドライバを参照してレジスタ直接操作で書いています。

| パッケージ | 内容 |
|---|---|
| [internal/i2s](internal/i2s) | I2S1 の送受信（GDMA のディスクリプタ輪をポーリング） |
| [internal/cam](internal/cam) | LCD_CAM によるカメラの 1 フレーム取り込み（GDMA） |

## 公式チュートリアルとの違い

| サンプル | 違い | 理由 |
|---|---|---|
| 全般 | フォントは tinyfont で近いものに置き換え（Orbitron → FreeSans など） | tinyfont に同じフォントがない |
| 01_display | LCD ドライバは tinygo-org/drivers の `ili9341` ではなく自作の `ili9342` | `ili9341` は ILI9341 用の初期化を送る。ILI9342E には送ってはいけないコマンドが含まれる |
| 04_rtc | NTP を使わず、ビルド時に日時を渡す | Wi-Fi を使わないため |
| 06_power | 5V 出力（Grove / M-BUS）を有効にしない | M5Unified の保護処理（外部 5V 入力時の判定）を移植していないため |
| 12_mic / 13_wav | 再生は常に 48kHz・モノラルに変換して鳴らす | 手元の CoreS3 のアンプが 44.1kHz で鳴らず、左チャンネルしか鳴らないため（M5Unified も 48kHz 固定） |
| 13_wav | ファイル全体を読み込まず、少しずつ読みながら再生する | メモリ節約 |
| 14_camera | 映像の連続表示の代わりに、約 8fps のプレビューとカウントダウン撮影（BMP 保存） | TinyGo が PSRAM に対応しておらず、フレームバッファを 1 枚しか持てないため |

音量は公式と合わせていません。

## 既知の問題

詳しくは [docs/notes.md](docs/notes.md) の末尾の一覧を見てください。

- **microSD カードが応答しなくなることがある**（開発中に 3 回発生）。`cores3.InitSD` は失敗すると基板の電源を 1 回だけ入れ直して（約 2 秒）回復を試みます
- タッチの上端で Y が 17 より小さくならない（指が端まで届いていない可能性もあり未確定）
- 地磁気センサ（BMM150）の Y 軸が常にオーバーフロー値になる（センサー出力そのもの。周囲の磁石の影響の可能性）
- 12_mic の再生を 48kHz 変換方式に変えた後の音は未確認

### TinyGo で見つかった問題と回避策

| 問題 | 回避策 |
|---|---|
| ESP32-S3 の I2C: 読み出しが完了前に戻る／NACK 後に復帰しない | [cores3/i2c.go](cores3/i2c.go) で完了待ちと再設定 |
| 文字列の配列をループ変数で参照すると `out of memory` になる場合がある | 書き方を変えて回避（07_imu） |
| 大きなデータ（数 MB）を `//go:embed` すると起動できない | WAV は本体上で生成（tools/sdput） |
| esp32s3 ターゲットの `--wrap=malloc` で C コード（fatfs）がリンクできない | [internal/libcwrap](internal/libcwrap) |
| tinygo-org/drivers の `pcf8563` が 10〜12 月を読み間違える | 自作の `bm8563` を使用 |

## シリアルログについて

ESP32-S3 の USB シリアルは、PC 側でポートを開いていない間の出力を捨てます。
操作に合わせてログを見るときは、先に `tinygo monitor` を起動しておいてください。

## サードパーティのコード

| 対象 | 出典 | ライセンス |
|---|---|---|
| [drivers/bmi270/config.go](drivers/bmi270/config.go)（BMI270 設定ファイル） | Bosch Sensortec BMI270_SensorAPI | BSD-3-Clause（ファイル内に表記） |
| [drivers/gc0308/settings.go](drivers/gc0308/settings.go)（GC0308 初期化テーブル） | espressif/esp32-camera | Apache License 2.0 |
| レジスタ値・初期化手順の参照 | M5Unified / M5GFX（MIT）、ESP-IDF・esp32-camera（Apache 2.0） | 各ソースのコメントに出典を記載 |

## 参考資料

- [M5Stack CoreS3 ドキュメント](https://docs.m5stack.com/ja/core/CoreS3)
- [M5Unified](https://github.com/m5stack/M5Unified) / [M5GFX](https://github.com/m5stack/M5GFX) / [M5CoreS3](https://github.com/m5stack/M5CoreS3)
- [ESP-IDF](https://github.com/espressif/esp-idf) / [esp-bsp (m5stack_core_s3)](https://github.com/espressif/esp-bsp/tree/master/bsp/m5stack_core_s3) / [esp32-camera](https://github.com/espressif/esp32-camera)
