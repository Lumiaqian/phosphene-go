#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
# A failed build stops before touching the installed app.
scripts/test.sh
scripts/build-local.sh
source_app="$PWD/dist/darwin-arm64/Phosphene Go.app"
installed_app="$HOME/Applications/Phosphene Go.app"
codesign --verify --deep --strict "$source_app"
python3 - "$installed_app" <<'PY'
import os, signal, subprocess, sys, time
app = sys.argv[1]
executables = {app + '/Contents/MacOS/Phosphene Go',
               app + '/Contents/Extensions/PhospheneExtension.appex/Contents/MacOS/PhospheneExtension'}
pids = []
for line in subprocess.check_output(['ps', '-axo', 'pid=,comm='], text=True).splitlines():
    pid, path = line.strip().split(None, 1)
    if path in executables:
        pids.append(int(pid))
        try: os.kill(int(pid), signal.SIGTERM)
        except ProcessLookupError: pass
for _ in range(50):
    alive = []
    for pid in pids:
        try: os.kill(pid, 0); alive.append(pid)
        except ProcessLookupError: pass
    if not alive: break
    time.sleep(.1)
else: raise SystemExit('App is still exiting; installation canceled.')
PY
mkdir -p "$HOME/Applications"
ditto "$source_app" "$installed_app"
codesign --verify --deep --strict "$installed_app"
registrar='/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister'
"$registrar" -u "$source_app" || true
"$registrar" -f "$installed_app"
printf '%s\n' "$installed_app"
