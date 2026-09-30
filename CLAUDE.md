# CLAUDE.md — M5Stack CoreS3 Lite × TinyGo

このリポジトリでは、M5Stack CoreS3 Lite（ESP32-S3）の内蔵ハードウェアを TinyGo で動かし、
M5Stack 公式の CoreS3 Arduino チュートリアル（14本）を Go に移植する。

M5Unified / M5GFX を丸ごと移植することはしない。これらは **レジスタ値・初期化手順の参照資料** として読むだけにし、
CoreS3 専用の薄い Go パッケージを自作する。

---

## 1. 作業ルール（最重要）

1. **レジスタ値・初期化シーケンスを推測で書かない。** 必ず一次資料から取り、コメントに出典を書く。
   - 例: `// ref: M5Unified src/utility/Power_Class.cpp (CoreS3 branch)`
   - 参照資料の優先順位: データシート > esp-bsp(m5stack_core_s3) > M5Unified / M5GFX > その他ブログ等
   - 資料が見つからない値は `// TODO(verify):` を付けて、ユーザーに報告する。
2. **1マイルストーンずつ進める。** 次に進む前に、必ず `tinygo build` が通ることを確認する。
3. **実機の見た目はユーザーにしか確認できない。** 書き込み後は「画面に何が出るはずか」を具体的に書いて、ユーザーに確認を頼む。
   シリアルログで確認できることは、必ずログにも出す。
4. ドライバ層（`drivers/`）には CoreS3 固有のピン番号や電源事情を持ち込まない。ボード固有の処理は `cores3/` に閉じ込める。
5. ドライバは tinygo-org/drivers の流儀に合わせる（`New(bus) Device` → `Configure(cfg) error` → 読み書きメソッド）。
   将来、上流に PR を出せる形にしておく。
6. ループ内でのヒープ確保を避ける（バッファは事前に確保して使い回す）。`fmt` はなるべく避け、`println` を使う。
7. マイルストーンごとに git commit する。コミットメッセージは `M2: display tutorial ported` のように書く。

---

## 2. 開発環境

- ホスト: Linux（Omarchy / Arch 系）。シリアルポートが開けないときは、ユーザーが `uucp` グループに入っているか確認する。
- TinyGo 0.41 以降を想定（ESP32-S3 の I2C / SPI / PWM / USB / 割り込みが入っている版）。
- 最初に必ず環境を確認する:
  ```sh
  tinygo version
  tinygo targets | grep -i esp32s3
  ```
- ターゲット: CoreS3 専用ターゲットは TinyGo 本体にはない。汎用の `esp32s3-generic` を使い、
  ピン定義は自作パッケージ `cores3/pins.go` に定数で持つ（`machine` パッケージには手を入れない）。
- ビルド・書き込み・モニタ:
  ```sh
  tinygo build -target=esp32s3-generic -o out.bin ./examples/01_display
  tinygo flash -target=esp32s3-generic ./examples/01_display
  tinygo monitor
  ```
- 書き込みできないときは、ダウンロードモードに入れる: **リセットボタンを約3秒長押しし、緑LEDが点いたら離す**（LEDが消えれば待機状態）。

---

## 3. ディレクトリ構成（目標）

```
.
├── CLAUDE.md
├── go.mod
├── drivers/            # チップ単位のドライバ（ボード非依存）
│   ├── axp2101/        # 電源管理IC
│   ├── aw9523b/        # IOエキスパンダ
│   ├── ltr553/         # 近接/照度センサ
│   └── bmi270/         # 6軸IMU（BMM150は補助I2C経由）
├── cores3/             # CoreS3 ボードパッケージ（薄い層）
│   ├── pins.go         # ピン定義・I2Cアドレス
│   ├── init.go         # バス初期化・電源投入シーケンス
│   ├── display.go
│   ├── touch.go
│   ├── button.go       # BtnPWR（AXP2101経由）
│   └── power.go
├── examples/           # 公式チュートリアルの移植（1本1ディレクトリ）
│   ├── 00_i2c_scan/
│   ├── 01_display/
│   └── ...
└── docs/notes.md       # 調査メモ・ハマりどころの記録
```

既存の tinygo-org/drivers から流用するもの:
- `ili9341`（M5Stack Core2 向けに ILI9342 対応済み） → Display
- `ft6336` → Touch
- `pcf8563` → RTC（BM8563 は PCF8563 互換。**動作は要確認**）
- `sdcard` → microSD
- `axp192` → AXP2101 ドライバを書くときの **構成の参考**（レジスタは別物なので流用しない）
- 描画: `tinydraw` / `tinyfont`

---

## 4. ハードウェア情報（公式ドキュメントより）

出典: https://docs.m5stack.com/ja/core/CoreS3 （CoreS3 Lite も同じ回路。違いはバッテリー200mAhと裏蓋のみ）
回路図: https://m5stack-doc.oss-cn-shenzhen.aliyuncs.com/490/Sch_M5_CoreS3_v1.0.pdf

### 内部I2C（全センサ共通）
- SDA = GPIO12、SCL = GPIO11

| チップ | 用途 | アドレス |
|---|---|---|
| AXP2101 | 電源管理 | 0x34 |
| AW9523B | IOエキスパンダ | 0x58 |
| FT6336U | タッチ | 0x38 |
| BM8563 | RTC | 0x51 |
| BMI270 | 6軸IMU | 0x69 |
| BMM150 | 地磁気（BMI270の補助I2C経由。内部I2Cには直接出てこない） | 0x10 |
| LTR-553ALS-WA | 近接/照度 | 0x23 |
| GC0308 | カメラ(SCCB) | 0x21 |
| ES7210 | マイク用コーデック | 0x40 |
| AW88298 | スピーカーアンプ | 0x36 |

### LCD（ILI9342C または ILI9342E、320x240、SPI）
- MOSI = GPIO37、SCK = GPIO36、CS = GPIO3、DC = GPIO35
- RST = AW9523B P1_1
- バックライト = AXP2101 DLDO1、電源 = AXP2101 LX1
- ⚠ **2026-08-07 以降の出荷品は ILI9342E に変更**（M5GFX v0.2.27 以上が必要とされている）。
  画面が映らない・色がおかしいときは、M5GFX の ILI9342E 用の初期化シーケンスを確認すること。

### microSD（LCDとSPIバス共有）
- MISO = GPIO35、MOSI = GPIO37、SCK = GPIO36、CS = GPIO4、公式サンプルは 25MHz
- ⚠ **GPIO35 は LCD の DC と SD の MISO を兼用している。** SD にアクセスするときはピンの役割切り替えが必要。
  M5GFX / M5Unified でどう扱っているかを調べてから実装する。

### タッチ
- FT6336U、RST = AW9523B P0_0、INT = AW9523B P1_2

### オーディオ（I2S）
- 共通: BCK = GPIO34、WCK(LRCK) = GPIO33、MCLK = GPIO0
- スピーカー: AW88298、DATA_OUT = GPIO13、RST = AW9523B P0_2、INT = AW9523B P1_3
- マイク: ES7210、DATA_IN = GPIO14
- マイクとスピーカーは同時に使えない（切り替えて使う）

### カメラ GC0308（DVP）
- VSYNC = GPIO46、HREF = GPIO38、PCLK = GPIO45、XCLK なし(-1)
- D0..D7 = GPIO39, 40, 41, 42, 15, 16, 48, 47
- RST = AW9523B P1_0
- 公式サンプルの設定: RGB565、QVGA、フレームバッファ2枚をPSRAMに確保

### その他
- RTC の割り込み出力は AXP2101 のウェイク入力に接続されている → 「RTCタイマー設定 → PMICで電源オフ → 時間が来たら復帰」を I2C だけで実現できる
- 電源ボタン(BtnPWR)は GPIO ではなく AXP2101 経由で読む。公式でも wasClicked / wasHold 相当しか使えないとされている
- 5V出力（Grove/BUS）と USB OTG の電源方向は、AXP2101 と AW9523B の `BUS_OUT_EN` / `USB_OTG_EN` で制御する
  - 参考: https://docs.m5stack.com/ja/arduino/m5cores3/power
- Grove: PORT.A = G2(SDA)/G1(SCL)、PORT.B = G9/G8、PORT.C = G17/G18

### 参照すべきソースコード
- M5Unified: https://github.com/m5stack/M5Unified （電源・ボタン・IMU・タッチ・RTC）
- M5GFX: https://github.com/m5stack/M5GFX （LCDの初期化・AW9523B経由のリセット）
- esp-bsp (CoreS3): https://github.com/espressif/esp-bsp/tree/master/bsp/m5stack_core_s3
- M5CoreS3: https://github.com/m5stack/M5CoreS3
- 各データシートは CoreS3 公式ページの「データシート」節にリンクあり

---

## 5. マイルストーン

各マイルストーンの「完了条件」を満たしたら、コミットしてユーザーに報告する。

### M0: 環境確認と I2C スキャン
- `examples/00_i2c_scan`: 内部I2C（G12/G11）をスキャンし、見つかったアドレスをシリアルに出力する
- 完了条件: 少なくとも 0x34, 0x58, 0x38, 0x51, 0x69, 0x23, 0x40, 0x36 が見える
  （0x21 のカメラは電源・クロック状態によって出ないことがある。0x10 は出ないのが正常）

### M1: 電源投入シーケンス（最重要・最初の関門）
- `drivers/axp2101`、`drivers/aw9523b` を最小限実装する（必要なレジスタだけでよい）
- `cores3.Init()` で M5Unified / M5GFX の CoreS3 初期化と同じ順番・同じ値を書き込む
- 完了条件: バックライトが点灯する（LCDが白っぽく光る）。書き込んだレジスタ値をシリアルにダンプして確認できる

### M2: Display チュートリアル
- 公式: https://docs.m5stack.com/ja/arduino/m5cores3/display
- 内容: ランダムな位置にランダムな色の円と矩形を描き続ける
- `ili9341` ドライバを ILI9342 モードで使う。LCDリセットは AW9523B P1_1 で行う
- 完了条件: 公式と同じ見た目になる。向き・色順（RGB/BGR）・色反転が正しい

### M3: Touch / Button
- Touch 公式: https://docs.m5stack.com/ja/arduino/m5cores3/touch
  - タッチ状態（touch / hold / flick / drag とその begin・end）と座標を表示する
  - FT6336 からは座標しか取れないので、状態遷移の判定は Go で実装する（閾値は M5Unified の Touch_Class を参照）
- Button 公式: https://docs.m5stack.com/ja/arduino/m5cores3/button
  - BtnPWR のクリックを AXP2101 の割り込みステータスで検出し、画面に表示する
- 完了条件: タッチ座標が画面の四隅で正しい値になる。電源ボタンの短押しを検出できる（長押し6秒は電源オフなので注意）

### M4: I2C センサ群
- RTC: https://docs.m5stack.com/ja/arduino/m5cores3/rtc
  - まずは NTP なしで、直接日時をセットする版にする（公式にもこの方法がコメントで書かれている）
- LTR553: https://docs.m5stack.com/ja/arduino/m5cores3/ltr553
  - 公式設定: LEDパルス 40kHz、測定周期 50ms、ALSゲイン 48x、PS と ALS を両方アクティブにする
- Power: https://docs.m5stack.com/ja/arduino/m5cores3/power
  - 充電電流を200mAに設定し、充電状態・バッテリー電圧・残量%・VBUS電圧を1秒ごとに表示する
- IMU: https://docs.m5stack.com/ja/arduino/m5cores3/imu
  - 加速度・ジャイロ・地磁気の9値を100msごとに表示する
  - BMI270 は起動時に設定ファイル（約8KB）の書き込みが必要。BMM150 は BMI270 の補助I2C経由で読む
- 完了条件: それぞれ公式サンプル相当の表示ができる

### M5: Wakeup
- 公式: https://docs.m5stack.com/ja/arduino/m5cores3/wakeup
- 内容: 画面タッチでスリープし、5秒後に自動で復帰する
- 方針: BM8563 のタイマーを設定してから AXP2101 で電源オフにする（M5Unified Power_Class の timerSleep 実装を参照）
- 完了条件: タッチしてから約5秒で再起動する

### M6: microSD / Image Files
- microSD: https://docs.m5stack.com/ja/arduino/m5cores3/sdcard
- Image Files: https://docs.m5stack.com/en/arduino/m5cores3/pic
- 内容: WriteTest.txt への書き込みと、320x240 の PNG 2枚の交互表示
- GPIO35 の兼用問題を先に解決すること
- PNG デコーダは TinyGo で使えるものを調査する（見つからなければ、まず RGB565 の生データ表示で代替する）

### M7（発展）: Speaker / MIC / Audio Files
- Speaker: https://docs.m5stack.com/ja/arduino/m5cores3/speaker （10kHz / 4kHz のトーン）
- MIC: https://docs.m5stack.com/ja/arduino/m5cores3/mic （17kHz で録音して波形表示、タップで再生）
- Audio Files: https://docs.m5stack.com/ja/arduino/m5cores3/wav （SD の WAV を再生）
- TinyGo は ESP32 系の I2S に未対応の可能性が高い。まず TinyGo の最新状況を調べ、なければ
  ESP32-S3 テクニカルリファレンスマニュアルの I2S 章を元にレジスタ直叩きで最小実装する。
- AW88298 / ES7210 の I2C 初期化は M5Unified を参照する

### M8（発展）: Camera
- 公式: https://docs.m5stack.com/ja/arduino/m5cores3/camera
- DVP入力（LCD_CAM ペリフェラル）+ DMA + PSRAM が必要。M7 まで終わってから着手の可否をユーザーと相談する。

---

## 6. 困ったときの確認リスト

- 画面が真っ暗 → M1 の電源シーケンスを疑う（DLDO1 のバックライト、AW9523B のリセット解除）
- 画面は光るが何も映らない → LCDリセットのタイミング、ILI9342C/E の違い、SPIモード・クロック
- I2C が NACK → 内部I2Cのプルアップ・周波数（まず100kHzで試す）、AXP2101 の電源が入っているか
- 書き込めない → ダウンロードモード（リセット3秒長押し）、シリアルポートの権限
- 調べて分かったこと・ハマったことは `docs/notes.md` に追記する
