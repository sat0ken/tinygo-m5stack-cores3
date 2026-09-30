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
- TODO(verify): ビットバン版の判別結果が実機で正しく出るか（ログの `RDDID=0xe3` と `-> ILI9342E/C`）。
  判別できないときは M5GFX と同じく ILI9342C とみなす。

## GPIO35（LCD D/C と SD MISO の兼用）
- M5GFX `Panel_M5StackCoreS3_T::cs_control()` は、LCD の CS を上げ下げするたびに GPIO35 の出力信号を切り替えている:
  - CS High（LCD 非選択）: `GPIO_FUNC35_OUT_SEL_CFG = FSPIQ_OUT_IDX`、出力無効 → MISO 入力として使える
  - CS Low（LCD 選択）: `GPIO_FUNC35_OUT_SEL_CFG = SIG_GPIO_OUT_IDX`、出力有効 → D/C として使える
- M2 時点では SD を使わないので、GPIO35 は GPIO 出力（D/C）固定。M6 で切り替えを実装する。

## SPI
- LCD は 40MHz（M5GFX と同じ）。G36/G37 は FSPI の IOMUX ピンではないので GPIO マトリクス経由。
  TODO(verify): 表示が乱れる場合は `cores3.LCDFrequency` を 20MHz に下げて試す。
