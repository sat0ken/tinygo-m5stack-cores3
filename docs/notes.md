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

## Wakeup（M5）
- CoreS3 では RTC の INT が ESP32 に直結されていない（AXP2101 の IRQ 線と共有）ため、M5Unified の `timerSleep(5)` は
  ディープスリープではなく「RTC タイマー割り込みをセット → `Axp2101.powerOff()`（reg 0x10 bit0）」になる。
  RTC の INT で電源が入り直す。手順: LCD sleep（輝度 0 → SLPIN）→ RTC disableIRQ / clearIRQ → setTimerIRQ(5000) → powerOff。
- M5.begin は既定（cfg.disable_rtc_irq = true）で起動時に RTC の割り込みを無効化する。`cores3.InitRTC` も同じにした。
- 実機確認（2026-10-01, USB 接続のまま）: タッチ → USB ポート消失 → 4.5〜4.7 秒後に再列挙 → 起動。
  起動時の RTC CTRL2 = 0x05（TF=1, TIE=1）で、RTC タイマーによる起動と確認。2 回連続で成功。
  RTC のタイマーは 1Hz クロックで数えるため、最初の 1 カウントは 1 秒未満になり、5 秒より少し短くなる。
- 電源断から復帰した直後は、LCD 判別の E 用キーの読み戻しが 0x7f になる（ソフトリセットだけのときは 0x00 / 0x14）。
  どちらも ID4 で ILI9342C と判別できている。
- Claude Code からプロセスを止めるとき `pkill -f <パターン>` を使うと、そのパターンを含む自分のシェルまで終了する。PID を指定して止める。

## microSD / 画像表示（M6）
- SD: tinygo-org/drivers `sdcard` + tinyfs `fatfs`（ElmChan FatFs を cgo で組み込み。exFAT 無効なので FAT12/16/32 のみ）。
  `FS.Format()` はカードを消去するので呼ばない。
- **esp32s3 ターゲットは `--wrap=malloc/calloc/free` でリンクする**（espradio 用）。fatfs の C コードが malloc を呼ぶと
  `undefined symbol: __wrap_malloc` になる。`internal/libcwrap` で `__wrap_*` → `__real_*`（TinyGo runtime の malloc 等）に転送。
  espradio と一緒に使うと重複定義になる点に注意。
- GPIO35（LCD D/C と SD MISO の兼用）: M5GFX は LCD の CS を切り替えるたびに GPIO35 の出力信号を差し替える。
  ここでは SD のブロックアクセスのたびに `cores3/sd.go` の sdBegin/sdEnd で切り替える:
  - sdBegin: LCD_CS=High、SPI を SDI=GPIO35（FSPIQ 入力）・25MHz で再設定
  - sdEnd: SD_CS=High、SPI を SDI なし・40MHz で再設定し、GPIO35 を GPIO 出力（D/C）に戻す
  LCD 側のドライバには手を入れずに済む。sdcard ドライバは初期化後に自分で 4MHz に設定するが、sdBegin で 25MHz に上書きしている。
- PNG: image/png で 320x240 RGBA を展開すると約 300KB 必要（ヒープは約 389KB）。`internal/pngstream` で 1 行ずつ展開して
  そのまま LCD に送る（行バッファ 2 本 + zlib の窓）。8bit 深度・インターレースなし・色タイプ 0/2/3/4/6 に対応。
  ホストの `go test` で公式画像 2 枚と各色タイプを image/png と全ピクセル比較して一致。
- 公式テスト画像は https://m5stack-doc.oss-cn-shenzhen.aliyuncs.com/490/cores3_test_picture0{1,2}.png（320x240 RGBA、全画素不透明）。
  Image Files チュートリアルの TestPicture01/02.png は公開されていないので、同じ画像をその名前でも置いた。
  `tools/sdput` で端末から SD に書き込む（既存ファイルは上書きしない）。画像は git に入れず `tools/sdput/fetch.sh` で取得。
- 実機（2026-10-01）: 4GB SDHC（FAT）。25MHz で読み書きでき、PNG 1 枚の表示に約 0.5 秒。既存ファイル（JPEG 13 個）が残っていることも確認。
- 実機確認（2026-10-01）: 10_pic の文字表示と、2 枚の画像（電車・雪山）の交互表示をユーザーが目視で確認。M6 完了。

## スピーカー（M7-1）
- TinyGo に ESP32 系の I2S は無い（machine/i2s.go は SAMD21 用）。さらに **ESP32-S3 の I2S には CPU からデータを書く FIFO レジスタが無く**、
  GDMA 経由でしか送受信できない（ESP-IDF i2s_ll.h に FIFO 書き込み関数が無く、FIFO はリセットのみ）。
- `internal/i2s`: I2S1 + GDMA OUT ch0 をレジスタ直叩きで実装。参照は ESP-IDF の i2s_ll.h / i2s_hal.c / gdma_ll.h / dma_types.h / gpio_sig_map.h。
  - クロック: PLL_F160M（S3 の既定）、MCLK = 256fs、BCK = fs×32。48kHz では 160MHz / 12.288MHz = 13 + 1/48。
  - スロット: Philips（msb_shift=1, ws_width=16）、16bit×2、モノラルは tx_mono + tx_chan_equal で左スロットを複製（ESP-IDF の is_copy_mono）。
  - DMA: 1KB のバッファ × 8 をディスクリプタで輪にし、各ディスクリプタに suc_eof。OUT_EOF_DES_ADDR をポーリングして再生済みスロットを数え、
    再生済みのバッファはゼロで埋める（アンダーラン時に古い音を繰り返さない）。
- AW88298: M5Unified `_speaker_enabled_cb_cores3` と同じ値（0x61=0x0673, 0x04=0x4040, 0x05=0x0008, 0x06=0x14C8@48kHz, 0x0C=0x0064）。
  I2S のクロックを出してからアンプを有効にしている。
- 実機確認（2026-10-01）: 10kHz / 4kHz のトーンが鳴ることをユーザーが確認。レジスタの読み戻しも一致。
  TODO(verify): 音量は M5Unified と合わせていない（ToneAmplitude で近似）。

## マイク（M7-2）
- `internal/i2s` に RX（GDMA IN ch0）を追加。RX は rx_eof_num = バッファのバイト数（ESP-IDF i2s_common.c と同じ）にして、
  IN_SUC_EOF_DES_ADDR をポーリングして受信済みのディスクリプタを数える。読むのが遅れて上書きされた分は Overruns に数える。
- クロック: M5Unified の mic_task と同じく PLL_240M（rx_clk_sel=1）、MCLK = 256fs（G0 に出力、mclk_sel=1 で RX にバインド）、
  BCK = MCLK/8（"CoreS3 のマイクは div_m >= 8"）。M5Unified の PLL_D2 = 120MHz と bits=16 の計算は、240MHz と 32bit/フレームの計算と同じ。
- ES7210: M5Unified `_microphone_enabled_cb_cores3` の初期化列（RESET_CTL=0xFF のあと 29 レジスタ）。MCLK を出してから設定する。
- 受信データ処理は M5Unified mic_task と同じ（ステレオ入力、自動ゼロレベル、noise_filter_level、ゲイン = magnification/(os<<1) = 1、L/R 平均）。
- 再生は TX を 17kHz で直接設定して鳴らす（M5 は 48kHz にリサンプリング）。AW88298 の 0x06 はレートから再計算（17kHz → 0x14C3）。
- 実機確認（2026-10-01）: 録音中の波形表示、タッチで直前 4.8 秒の再生、電源ボタンでノイズフィルタ切り替えをユーザーが確認。
  320 サンプル（18.8ms）ごとの描画で録音に追従できている（100 フレーム ≒ 1.9 秒）。

## WAV 再生（M7-3）
- 公式は /sample-12s.wav を再生するが、ファイルは公開されていない。`tools/sdput/wavgen.go` で本体上でテスト音を生成して SD に書く
  （12 秒、44.1kHz、16bit ステレオ。0〜4 秒は左だけ・右だけを交互に鳴らすアルペジオ、4〜12 秒はハ長調の音階）。
- **大きなデータを `//go:embed` で埋め込むと起動できない**: `[]byte` だと RAM（.data）に置かれて 2MB が入らない。`string` にすると
  リンクは通るが、ROM ローダが "Invalid image block, can't boot" で起動を繰り返す（TinyGo の ESP32-S3 イメージは 2nd stage
  ブートローダを使わず、ROM ローダが DROM セグメントを丸ごと読み込むため）。数百 KB（PNG 2 枚）までは `[]byte` で動いた。
- 起動ループの後、SD の初期化が `SD_CARD_ERROR_ACMD41` で失敗し続けた。ESP32 のリセットや ALDO4（カード電源）の 200ms 断では直らず、
  RTC タイマー + AXP2101 での基板全体の電源断（cores3.TimerSleep）で回復した。信号線経由でカードに電気が回り込んでいる可能性。
  sdput は SD 初期化に失敗したとき、1 回だけ基板の電源を入れ直す。
- **原因（と思われるもの）: SD カードが SD モードのまま LCD の通信を受けていた。** 電源投入直後のカードは SD モードで、
  共有している SCK/MOSI 上の LCD 向けデータを CMD 線のコマンドとして解釈し得る（休止状態に入ると電源を切るまで初期化できない）。
  M5GFX は autodetect の中で LCD を触る前に `_set_sd_spimode()`（CS High で空クロック → CMD58、応答が無ければ CMD0）で
  カードを SPI モードにしている。これを `cores3.InitDisplay` の先頭に移植した（sdSPIMode）。
  2 回目の発生（音の確認用プログラムが LCD 描画を繰り返したあと）は、この対策の前に起きた。対策後は様子見。
- 13_wav は M5 のようにファイル全体やセグメントをメモリに読み込まず、4KB ずつ読んで I2S の DMA リングに流す。
  実機: 44.1kHz ステレオの 12 秒を 12006ms で再生（SD 読み出しが間に合っている）。
- 最初は WAV のレート（44.1kHz）・ステレオのまま TX を設定したが、無音だった。切り分け（2kHz -12dBFS、2026-10-01、ユーザーが聴取）:
  | テスト | 結果 |
  |---|---|
  | モノラル 48kHz / ステレオ 48kHz L+R / ステレオ 48kHz L のみ | 鳴る |
  | モノラル 44.1kHz / ステレオ 44.1kHz | 鳴らない |
  | ステレオ 48kHz R のみ | 鳴らない |
  → **AW88298 は左スロットだけを鳴らす**。**44.1kHz では鳴らない**（17kHz + AW88298 16kHz 設定は鳴っていた）。
  原因は未特定（TODO: AW88298 のレート設定 0x06=0x14C7 か、160MHz からの分数分周の影響か）。
- 対策: M5Unified と同じく、スピーカーは常に 48kHz・モノラルで動かし、データ側を変換する（`cores3.QueuePCMResampled`、線形補間）。
  ステレオは (L+R)/2 でモノラルにする。12_mic の再生（17kHz）も同じ方式にした。
- テスト音は 2 オクターブ上げ（C6 = 1047Hz〜）、振幅 -6dBFS にした。CoreS3 の小型スピーカーは 260〜520Hz がほとんど鳴らない。
  再生時の音量は 128/256。TODO(verify): M5 の master volume 64 との対応は未確認。
- 実機確認（2026-10-01）: 48kHz モノラル＋リサンプリング方式で 13_wav の音が正しく鳴ることをユーザーが確認。M7 完了。
  TODO(verify): 12_mic の再生もリサンプリング方式に変えたが、変更後は実機で聴いていない。

## 未解決・要確認の一覧（2026-10-01 時点）
- タッチ上端で Y が 17 未満にならない（M3）
- BMM150 の Y 軸が常にオーバーフロー値（M4）
- AW88298 が 44.1kHz で鳴らない原因（M7）
- 音量を M5Unified と合わせていない（トーン・WAV）（M7）
- 12_mic の再生（リサンプリング版）の実機確認（M7）
- TinyGo 本体への報告候補: ESP32-S3 I2C の読み出し完了前 return / NACK 後に復帰しない、[N]string 添字アクセスでの OOM、
  大きな go:embed で起動不能。drivers への報告候補: pcf8563 の月マスク

## カメラ（M8）の事前調査（2026-10-01、未着手）
- 公式: esp32-camera、RGB565 / QVGA、fb_count=2 を PSRAM に置く、pin_xclk=-1（GC0308 側に発振器）、SCCB は内部 I2C（G12/G11）。
- TinyGo: **PSRAM 未対応**（runtime / machine / targets に実装なし。リンカの DRAM は 416KB）。LCD_CAM のレジスタ定義は device/esp にある。
- QVGA RGB565 は 1 枚 153.6KB。PSRAM なしでも内部 RAM に 1 枚なら置ける（ヒープ約 389KB）。2 枚は厳しい。VGA は不可。
- 取り込み（参照: esp32-camera target/esp32s3/ll_cam.c、ESP-IDF gdma_channel.h / gpio_sig_map.h）:
  - LCD_CAM の cam 部 + GDMA IN（周辺番号 5 = SOC_GDMA_TRIG_PERIPH_CAM0）。I2S RX と同じ作り（ディスクリプタ輪 + EOF ポーリング）で書ける。
  - cam_clk_sel=3（クロック出力なし、PCLK で取り込む）、cam_vsync_filter_en=1、rec_data_bytelen でディスクリプタ単位の EOF。
  - 入力信号: CAM_DATA_IN0..7 = 133..140、CAM_PCLK = 149、CAM_H_ENABLE = 150、CAM_V_SYNC = 152。
  - esp32-camera は VSYNC 割り込みでフレームの区切りを取る。割り込みなしでは VSYNC（G46）のポーリングで代用する。
- GC0308 の初期化: esp32-camera sensors/private_include/gc0308_settings.h の 247 レジスタ + set_pixformat(RGB565) + set_framesize（QVGA はサブサンプル設定）。
- 表示: 153.6KB を SPI 40MHz で送ると理論値 31ms。TinyGo の SPI は 64 バイトずつのポーリングなので実測は 40〜100ms 程度の見込み → 5〜10fps。

## カメラ（M8）: 写真の撮影と BMP 保存
- 映像の連続表示ではなく、ユーザーと相談して「タップで 1 枚撮って表示、SD に BMP 保存」にした（PSRAM なしで 1 フレーム 153.6KB を内部 RAM に置く）。
- `drivers/gc0308`: esp32-camera の reset（ソフトリセット + 既定 247 レジスタ）、set_pixformat(RGB565)、set_framesize（QVGA は 1/2 サブサンプル）。
  チップ ID 0x9b（M5GFX の判定と同じ）。
- `internal/cam`: LCD_CAM（cam_clk_sel=3、VSYNC は反転入力）+ GDMA IN ch1。40 ディスクリプタ × 3840 バイト（6 ライン）をフレームバッファに直接割り当て、
  rec_data_bytelen = 3840-1 で各ディスクリプタに EOF。LCD_CAM の CAM_VSYNC_INT_RAW をポーリングしてフレーム開始を待ち、最後のディスクリプタの EOF で完了。
- ハマりどころ:
  - **VSYNC フラグはカメラ部が動いている（cam_start=1）間しか立たない**。esp32-camera も ll_cam_config の最後で cam_start=1 にしている。
  - **GDMA のリセット（SYSTEM PERIP_RST_EN1.DMA_RST）は起動直後に立っている**。I2S 側の初期化で解除していたので気付かなかった。
    カメラ単体のプログラムでは DMA が全く動かなかった（IN_DSCR=0）。
- 信号の実測: VSYNC 20fps、1 フレーム 240 ライン（QVGA 設定どおり）。
- 実機（2026-10-01）: 取り込み 49〜132ms（VSYNC 待ちを含む）、LCD 表示 36ms、BMP 保存 1.3 秒（230,454 バイト）。
  ユーザーが写真の撮影と SD への保存を確認。画素は上位バイトが先の RGB565（そのまま LCD に送って正しく表示される）。
- プレビュー（連続表示）は、取り込み + 表示で 1 枚 90〜170ms → 6〜10fps 程度の見込み（未実装）。
