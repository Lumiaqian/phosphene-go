#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
# A failed build stops before touching the installed app.
scripts/test.sh
scripts/build-local.sh
source_app="$PWD/dist/darwin-arm64/Phosphene Go.app"
installed_app="$HOME/Applications/Phosphene Go.app"
codesign --verify --deep --strict "$source_app"
# Stop the manager only. Killing the extension behind WallpaperAgent's back
# keeps WallpaperAgent from launching it again for minutes; restarting
# WallpaperAgent below retires it the way the system expects.
python3 - "$installed_app" <<'PY'
import os, signal, subprocess, sys, time
app = sys.argv[1]
executables = {app + '/Contents/MacOS/Phosphene Go'}
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
# Sync into the installed bundle, deleting the files the new build dropped:
# merging (ditto) would keep them and break the signature, and replacing the
# bundle drops the extension's PlugInKit registration, after which
# WallpaperAgent may not launch it again for a while.
rsync -a --delete "$source_app/" "$installed_app/"
codesign --verify --deep --strict "$installed_app"
registrar='/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister'
"$registrar" -u "$source_app" || true
"$registrar" -f "$installed_app"
# A first install, or one whose registration went missing, registers it.
extension="$installed_app/Contents/Extensions/PhospheneExtension.appex"
pluginkit -m -i local.lumiaqian.phosphenego.extension | grep -q . || pluginkit -a "$extension"
# Restarting WallpaperAgent retires the running extension and launches the new
# one, rebuilding every display's wallpaper. Wait for the new one's report;
# should it stay silent, restart again, waiting longer each time. A locked
# screen also keeps it silent; then this only warns.
runtime="$HOME/Library/Containers/local.lumiaqian.phosphenego.extension/Data/Documents/runtime.json"
extension_reports() { # since, seconds: the extension wrote its report after since
	local since=$1
	for _ in $(seq 1 "$2"); do
		sleep 1
		[ -f "$runtime" ] && [ "$(stat -f %m "$runtime")" -gt "$since" ] && return 0
	done
	return 1
}
sleep 2
reported=
for wait in 30 30 45 45; do
	restarted=$(date +%s)
	killall -TERM WallpaperAgent 2>/dev/null || true
	if extension_reports "$restarted" "$wait"; then
		reported=1
		break
	fi
	echo "wallpaper extension silent ${wait}s after restarting WallpaperAgent; retrying" >&2
done
if [ -z "$reported" ]; then
	echo 'warning: the wallpaper extension has not reported since WallpaperAgent restarted; use 修复壁纸 in the app once the screen is unlocked' >&2
fi
printf '%s\n' "$installed_app"
