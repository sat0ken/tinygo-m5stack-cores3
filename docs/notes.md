# 調査メモ・ハマりどころ

## 環境 (2026-09-30)
- TinyGo 0.42.0 (go1.27.1, LLVM 22.1.4)、ターゲット `esp32s3-generic`
- tinygo.org/x/drivers v0.36.0
- 参照したソースのリビジョン: M5GFX `cd363dd`、M5Unified `44d0c52`、esp-bsp master (bsp/m5stack_core_s3)

## TinyGo ESP32-S3 I2C の癖
- `machine_esp32xx_i2c.go` の `transmit()` は、最後の読み出しバイト（`readLast`）のときに NACK 割り込みを無視する。
  そのため **読み出しだけの `Tx(addr, nil, buf)` では、デバイスがいなくてもエラーにならない**。
  また `Tx(addr, nil, nil)` はアドレスを送出しない。
- → I2C スキャンは「レジスタポインタ 0x00 を 1 バイト書く」`Tx(addr, []byte{0}, nil)` で行う（examples/00_i2c_scan）。
- `Tx(addr, []byte{reg}, buf)`（書いてから読む）も同じ理由で、デバイス不在を検出できない。
  ドライバの `Configure()` ではチップ ID レジスタ（AXP2101: 0x03=0x4A、AW9523B: 0x10=0x23）を読んで存在確認している。
- **NACK を一度受けると、以降の通信がすべて NACK エラーになる**（`Configure` し直すと復帰）。
  スキャンでは NACK のたびに再設定している。
- **読み出しを含む `Tx` は TRANS_COMPLETE（STOP 送出完了）より前に戻る。** 直後にすき間なく次の通信をすると、
  エラーなしで前回の値が返り、さらに次の通信が NACK になる（実機 2026-10-01 に再現: 読み→読み で 2 回目が化ける）。
  - SR の `SCL_MAIN_STATE_LAST` / `BUS_BUSY` では完了を判定できなかった（"last" の状態で、現在の状態ではない）。
  - 回避策 `cores3/i2c.go`: 読み出しの後に INT_RAW の TRANS_COMPLETE を最大 2ms 待つ（ドライバが戻り際に割り込みビットを
    クリアするので、先に完了していた場合はタイムアウトで抜ける。1400 回中 1 回）。エラー時は再設定する。
  - 1400 回の連続読み書きで値化け・エラーなし。400kHz でも動作。
  - TODO: TinyGo 本体に報告・修正 PR を出す（`machine` には手を入れない方針なので、ここでは回避策のみ）。

## 電源投入シーケンス（M1）
`cores3.Init()` は次の 2 つをこの順に再現している。
1. M5GFX `M5GFX::autodetect()` の CoreS3 分岐（AW9523B の出力・方向・GCR・LEDMODE、AXP2101 の 0x23/0x90/0x94/0x95）
2. M5Unified `Power_Class::begin()` の CoreS3 分岐（BOOST_EN、ALDO1〜4、0x27/0x69/0x10/0x30）
- バックライトは DLDO1。M5GFX の `Light_M5StackCoreS3::setBrightness` と同じ式 `(b + 641) >> 5` を 0x99 に書く。既定値 127 → 0x18（2.9V）。
- M5GFX は G35/G36/G37 をプルアップ入力で読み、全部 Low のときだけ BUS_OUT_EN と BOOST_EN も立てる。これも再現した。
- 電源ボタンの長押し時間: M5Unified は 0x27=0x00（Hold=1秒 / PowerOff=4秒）を書く。CLAUDE.md の「長押し6秒」は
  AXP2101 のリセット既定値の話と思われる。Init 後は **4秒で電源オフ** になる点に注意。

## LCD（M2）: tinygo-org/drivers の ili9341 を使わなかった理由
- `ili9341.Configure()` は ILI9341 用の初期化列（0xEF, 0xCF, 0xED, 0xE8, 0xCB, 0xF7, 0xEA …）を送る。
  M5GFX の Panel_ILI9342.hpp には「ILI9342C のリストを ILI9342E に送ってはいけない」とあり、
  特に 0xCB は ILI9342E では Power Control 6 に当たるため、ILI9341 の値を送るのは危険。
- 2026-08-07 以降の出荷品は ILI9342E。今日（2026-09-30）時点で買った個体は E の可能性が高い。
- → `drivers/ili9342` を新規に作り、M5GFX の C / E それぞれの初期化列を移植した。
- 初期化の流れ（M5GFX と同じ）: AW9523B P1_1 で LCD_RST を Low 8ms → High 64ms → 初期化列 → INVON → COLMOD=0x55 → MADCTL。
- 向き: CoreS3 は `offset_rotation = 3`、既定 `rotation = 1` → コントローラ回転 0 → MADCTL = 0x08（BGR のみ）、320x240。

### ILI9342C / E の判別
- M5GFX は D9h 経由で DDh/CBh を読み戻して判別する（E なら 01h/1Ch）。読み出しは SDA 線の双方向（3-wire）で行う。
- TinyGo の SPI ドライバは半二重（SIO）読み出しに対応していないため、判別時だけ 4 本のピンを GPIO にしてビットバンする
  （`ili9342.ProbeVariant`）。その後 `machine.SPI0.Configure` でピンを SPI に戻す。
- M5GFX は読み出し値を `(v >> 1) & 0x7F` に正規化して比較している（ESP32-S3 でダミー1bitを入れる都合）。同じ手順を再現した。
- 実機（2026-10-01）: `RDDID=0xe3 DDh=0x7f CBh=0x7f ID4=0x1342 -> ILI9342C`。手元の個体は C だった。
  E での判別は未検証。判別できないときは M5GFX と同じく ILI9342C とみなす。

## GPIO35（LCD D/C と SD MISO の兼用）
- M5GFX `Panel_M5StackCoreS3_T::cs_control()` は、LCD の CS を上げ下げするたびに GPIO35 の出力信号を切り替えている:
  - CS High（LCD 非選択）: `GPIO_FUNC35_OUT_SEL_CFG = FSPIQ_OUT_IDX`、出力無効 → MISO 入力として使える
  - CS Low（LCD 選択）: `GPIO_FUNC35_OUT_SEL_CFG = SIG_GPIO_OUT_IDX`、出力有効 → D/C として使える
- M2 時点では SD を使わないので、GPIO35 は GPIO 出力（D/C）固定。M6 で切り替えを実装する。

## SPI
- LCD は 40MHz（M5GFX と同じ）。G36/G37 は FSPI の IOMUX ピンではないので GPIO マトリクス経由。
  表示が乱れる場合は `cores3.LCDFrequency` を 20MHz に下げて試す。
- 01_display の描画速度: 円+正方形 500 組で約 1.32 秒（TinyGo の SPI.Tx は 64 バイトずつのポーリング転送、DMA なし）。

## タッチ（M3）
- tinygo-org/drivers の `ft6336` は使わなかった。理由:
  - `Configure()` が INT ピンを **プルダウン** にする。CoreS3 の INT（GPIO21）は AW9523B のオープンドレイン INTN なので、プルアップが必要。
  - `Read()` のたびに `[]byte{0x02}` をヒープに確保する（ループで呼ぶと規則6に反する）。
  - `ReadTouchPoint()` は Core2 前提の係数（/320, /270）で 16bit にスケーリングしていて、生の座標が取れない。
- → `cores3/touch.go` で M5GFX と同じ手順で読む:
  - GPIO21 が Low のときだけ FT6336U を読む。0 点なら AW9523B の 0x00/0x01 を読んで INT を解除する（`Touch_M5StackCoreS3::getTouchRaw`）。
  - 2 回連続で同じ値が読めるまで読み直す（最大5回）（`Touch_FT5x06::getTouchRaw`）。
- 状態遷移は M5Unified `Touch_Class::update` / `update_detail` を移植（flick 閾値 8px、hold 500ms、更新間隔 4ms）。
  drag は「hold のあとに動かした」状態（hold 0101 | flick_begin 1011 = drag_begin 1111）。
- 座標変換: Touch_M5StackCoreS3 は x 0..319 / y 0..239、offset_rotation 0。既定の画面向きではコントローラ回転 0 なので、生座標＝画面座標。
- 実機: `FT6336U CIPHER=0x64 FIRMID=0x10 VENDID=0x11`。
- 実機確認（2026-10-01）: touch / hold / flick / drag の begin・本体・end がすべて期待どおりに遷移。
  四隅は 左上(0,29) 右上(319,17) 左下(3,239) 右下(319,239)。X は 0〜319 の全域、Y は下端 239 まで出る。
  TODO(verify): 上端で Y が 17 未満にならなかった（指が上端まで届いていなかった可能性あり）。

## 電源ボタン（M3）
- AXP2101 の IRQ ステータス 0x49 の bit3（短押し）/ bit2（長押し）を 4ms ごとに読み、1 を書いてクリアする（M5Unified `getPekPress`）。
  押している最中の状態は取れない（公式でも wasClicked / wasHold 相当のみ）。
- 実機確認（2026-10-01）: 短押し 1 回につき clicked が 1 回（取りこぼし・二重検出なし）、約 1.5 秒押しで hold を検出。

## シリアルログの取り方（Claude Code から）
- `tinygo monitor` は TTY が必要なので、非対話環境では `stty -F /dev/ttyACM0 115200 raw -echo; timeout N cat /dev/ttyACM0` で読む。
- USB Serial/JTAG はホストが読んでいない間の出力を捨てる。人が操作する確認は、先にログ記録を開始しておく。

## 文字表示
- tinyfont の `WriteLine` は 1 ピクセルずつ `SetPixel` を呼ぶので、SPI LCD ではウィンドウ設定のコストが大きい。
  `internal/gfx` でグリフのビットマップを横方向のランにまとめて `FillRectangle565` で描く。
- `Font.GetGlyph` はインタフェース（Glypher）を返すため文字ごとにヒープ確保が起きる。`gfx` では `Font.Glyphs` を直接二分探索する。
- 公式の `fonts::Orbitron_Light_24` は tinyfont に無いので FreeSans 12pt で代用。

## RTC（M4）
- tinygo-org/drivers の `pcf8563`（v0.36.0）は使わず、`drivers/bm8563` を自作した。理由:
  - **`ReadTime` が月を `& 0x0F` で取り出す**ため、10〜12 月（BCD 0x10〜0x12）を読み間違える。
    実機で 2021-12-31 をセットすると 2021-03-03（2/31 が正規化）と読めた。正しくは `& 0x1F`（M5Unified も 0x1f）。
  - 分を `% 0x7F` で取り出している（`& 0x7F` の誤り）。
  - 曜日を 1〜7 で書く（M5Unified / データシートは 0〜6）。
  - TODO: 上流に修正 PR を出す。
- RTC は UTC で持つ。04_rtc は `-ldflags "-X main.setTime=..."` で日時を渡したとき、または VL フラグが立っているとき
  （公式コメントの 2021-12-31 12:34:56）だけセットする。
- ビルド開始から実際にセットされるまで約 8.5 秒（書き込み約 6 秒 + 起動待ち 2 秒 + 初期化）。現在時刻に +9 秒して渡すとホストと 1 秒未満で一致した:
  `tinygo flash -target=esp32s3-generic -ldflags="-X main.setTime=$(date -u -d '+9 sec' +%Y-%m-%dT%H:%M:%SZ)" ./examples/04_rtc`
- ESP32 の時計は `runtime.AdjustTimeOffset` で RTC に合わせる（`cores3.SyncSystemTime`）。

## LTR-553（M4）
- 公式は M5CoreS3 リポジトリの `src/utility/LTR5XX.{h,cpp}`（LTR55X 分岐）を使う。レジスタ・ビット位置・既定値はそこから移植（`drivers/ltr553`）。
- ALS の値は 2 チャンネルの平均 `(CH0 + CH1) >> 1`（LTR5XX::getAlsValue）。PS は 11bit。
- 実機: PART_ID=0x92, MANUFAC_ID=0x05。室内で ALS≈1479（ゲイン48x）、手を近づけない状態で PS=0〜6。
- 実機確認（2026-10-01）: 手を近づけると PS が最大 965、覆うと ALS が 0〜11 まで下がる。

## 電源（M4）
- 公式は M5.begin の既定 `cfg.output_power = true` で Grove/M-BUS への 5V 出力（BUS_OUT_EN）も有効にするが、
  M5Unified はバッテリー無し＋外部 5V 入力時に取り消す保護ロジック（TS 電圧の判定）を持っている。
  表示内容には影響しないので、06_power では 5V 出力を有効にしていない（Init の状態のまま）。
- 実機: 充電中=1、4055mV、45%、VBUS 4917mV。充電電流レジスタ 0x62=8（200mA）。

## IMU（M4）
- BMI270 の設定ブロブ（8192 バイト）は Bosch 公式 BMI270_SensorAPI（commit 41129fc）の `bmi270_config_file[]` から生成。
  M5Unified 同梱のものとバイト単位で一致することを確認済み。BSD-3-Clause のライセンス表記を `drivers/bmi270/config.go` に残した。
- ロード手順は Bosch に合わせた（INIT_CTRL=0 → INIT_ADDR=index/2 でチャンク書き込み → INIT_CTRL=1 → INTERNAL_STATUS&0x0F==1 を待つ）。
  M5Unified は INIT_CTRL=0 を省略し、8KB を 1 回で書き、INTERNAL_STATUS は 0 以外かだけを見る。
- BMM150 は BMI270 の AUX I2C 経由（M5Unified と同じ: 0x4B=0x83, WhoAmI 0x40=0x32, 0x4C=0x38, AUX_IF_CONF=0x4F, AUX_RD_ADDR=0x42, PWR_CTRL=0x0F）。
- CoreS3 では地磁気の Y・Z を反転（M5Unified IMU_Class: board_M5StackCoreS3 && addr 0x69）。
- 実機: INTERNAL_STATUS=0x01、BMM150 WhoAmI=0x32。平置きで az≈0.995G、ジャイロ ≈0 dps。
- TODO(verify): **BMM150 の Y が常に -4096（13bit のオーバーフロー値）**。AUX 手動モードで 0x42〜0x49 を直接読んでも
  同じ（x13=879, y13=-4096, z15=-3776）なので、読み出し・解釈ではなくセンサー出力そのもの。設定は M5Unified と同一なので
  公式版でも同じ値になるはず。近くの磁石（スピーカー等）による飽和の可能性。
- 実機確認（2026-10-01）: 傾け・裏返しで加速度が ±1G 付近を動き（az は -1.27〜+2.02）、ジャイロは最大 ±700 dps 程度まで反応。
  地磁気は水平に 1 回転させて mx が 2470〜3028、mz が 11167〜11578 の範囲で変化（大きなオフセットに地磁気分が乗っている形）。
  my は終始 12282（オーバーフロー）のまま。センサーは動作しているが、Y 軸は飽和していて使えない。
  TODO: 公式 Arduino 版（M5Unified）で同じ個体の値を比べられれば、ハードウェア要因と確定できる。

## TinyGo 0.42 / Xtensa のコード生成の疑い
- 07_imu で `labels := [3]string{l0, l1, l2}` をループ変数で `labels[i]` と参照し `append(b[:0], labels[i]...)` すると、
  ヒープ使用量 12KB / 389KB の状態で `fatal error: out of memory` になった（`-print-allocs` ではその関数にヒープ確保なし、
  スタックを 32KB にしても再現）。文字列を 1 つずつ引数で渡す形にしたら解消。
  TODO: 最小再現コードを作って TinyGo に報告する。

## M4 完了（2026-10-01）
- 04_rtc / 05_ltr553 / 06_power / 07_imu の画面表示をユーザーが目視で確認済み（時刻・センサー値の変化・電源情報・IMU 9 値）。
- 未解決: 地磁気 Y 軸の飽和（上記 IMU の TODO）、タッチ上端で Y が 17 未満にならない件（M3 の TODO）。
