#!/bin/sh
# Download the test images of the M5Stack CoreS3 microSD tutorial into assets/.
# ref: https://docs.m5stack.com/en/arduino/m5cores3/sdcard
set -e
cd "$(dirname "$0")/assets"
for n in 01 02; do
  curl -sSfL -o cores3_test_picture$n.png https://m5stack-doc.oss-cn-shenzhen.aliyuncs.com/490/cores3_test_picture$n.png
done
ls -l
