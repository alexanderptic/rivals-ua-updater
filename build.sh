#!/usr/bin/env bash
# Збірка RivalsUA.exe під Windows. Виконується будь-де, де є Go — Windows
# для збірки не потрібна, лише для перевірки.
set -euo pipefail
cd "$(dirname "$0")"

# Два обходи заблокованих хостів: golang.org/x/sys і gopkg.in віддзеркалені
# на GitHub. Якщо ваша мережа їх пускає — обидва replace можна прибрати.
export GOFLAGS=-mod=mod GOPROXY=direct GOSUMDB=off

# Ресурси: іконка (7 розмірів), маніфест (DPI + Common Controls v6) і
# VERSIONINFO (те, що Windows показує у властивостях файлу).
if ! command -v goversioninfo >/dev/null; then
  GOBIN="$PWD/.bin" go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
  export PATH="$PWD/.bin:$PATH"
fi
goversioninfo -64 -icon=RivalsUA.ico -manifest=RivalsUA.manifest \
  -o=rsrc_windows_amd64.syso versioninfo.json

# -H windowsgui — без вікна консолі за спиною.
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-H windowsgui -s -w" -o RivalsUA.exe .

# Архів: .exe і завантажувач поруч. Вшивати завантажувач усередину не можна —
# програма, що дістає з себе DLL і кладе її біля чужої гри, це поведінковий
# портрет дроппера.
if [ ! -f loader/dsound.dll ]; then
  cat >&2 <<'MSG'
Немає теки loader/ — .exe зібрано, але архів без неї складати немає сенсу:
гра не завантажить пак без завантажувача модів.

Візьміть теку loader/ з архіву останнього релізу й покладіть поруч:
  https://github.com/alexanderptic/rivals-ua-updater/releases/latest/download/RivalsUA.zip

Контрольні суми, які програма звіряє перед установкою, вшиті в loader.go.
MSG
  exit 1
fi

rm -rf pkg && mkdir -p pkg/RivalsUA
cp RivalsUA.exe pkg/RivalsUA/
cp -r loader pkg/RivalsUA/
( cd pkg && zip -r -q RivalsUA.zip RivalsUA )

echo "готово: $(cd pkg && sha256sum RivalsUA.zip)"
