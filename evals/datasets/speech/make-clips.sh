#!/usr/bin/env bash
# Regenerates the speech fixtures from manifest.jsonl with Microsoft Edge's
# neural TTS (edge-tts, no credentials), then re-encodes each clip to mono
# 16 kHz 32 kbps MP3 with ffmpeg (duration_s in manifest.jsonl is ffprobe of the result).
set -euo pipefail
cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
python3 -c 'import json,sys
for l in open("manifest.jsonl"):
    r=json.loads(l); print(r["file"]+"\t"+r["voice"]+"\t"+r["spoken_text"])' | while IFS=$'\t' read -r file voice text; do
  if command -v edge-tts >/dev/null; then tts=(edge-tts); else tts=(uv tool run --from edge-tts==7.3.1 edge-tts); fi
  "${tts[@]}" --voice "$voice" --text "$text" --write-media "$tmp/$file" >/dev/null </dev/null
  ffmpeg -nostdin -hide_banner -loglevel error -y -i "$tmp/$file" -ac 1 -ar 16000 -b:a 32k -af "apad=pad_dur=0.3" "$file"
  echo "$file $(ffprobe -v error -show_entries format=duration -of csv=p=0 "$file")s $(wc -c <"$file")B"
done
