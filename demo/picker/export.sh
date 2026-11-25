#!/bin/bash
set -euo pipefail
cd "${1:?usage: export.sh <take-directory>}"
ffmpeg -nostdin -v error -i master.mkv -filter_complex \
  'fps=15,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3:diff_mode=rectangle' \
  -loop 0 picker-demo.gif
frames=$(ffprobe -v error -select_streams v:0 -count_frames -show_entries stream=nb_read_frames -of csv=p=0 picker-demo.gif)
samples=$(awk -v n="$frames" 'BEGIN {for (i=0;i<9;i++) printf "%seq(n,%d)", (i?"+":""), int((n-1)*i/8)}')
ffmpeg -nostdin -v error -i picker-demo.gif -vf "select='$samples',scale=468:-2,tile=3x3:padding=4:color=black" -frames:v 1 contact-sheet.png
