#!/bin/bash
set -euo pipefail

# Setup GNOME custom keybindings for workshell
# super+r  → rotate workspaces
# super+u  → unfocus (send current workspace back to home position)

WS_BIN=$(command -v ws 2>/dev/null || echo "$HOME/.local/bin/ws")

if [ ! -f "$WS_BIN" ]; then
    echo "Error: ws not found — run 'bash build.sh install' first"
    exit 1
fi

SCHEMA="org.gnome.settings-daemon.plugins.media-keys"
KEY_PATH="/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings"

# Read existing custom keybindings
existing=$(gsettings get $SCHEMA custom-keybindings)

# Find next available slot (avoid collicting with existing bindings)
slot=0
while echo "$existing" | grep -q "custom${slot}"; do
    slot=$((slot + 1))
done
rotate_slot=$slot

slot=$((slot + 1))
while echo "$existing" | grep -q "custom${slot}"; do
    slot=$((slot + 1))
done
unfocus_slot=$slot

ROTATE_PATH="${KEY_PATH}/custom${rotate_slot}/"
UNFOCUS_PATH="${KEY_PATH}/custom${unfocus_slot}/"

# Set up rotate: super+r
gsettings set "${SCHEMA}.custom-keybinding:${ROTATE_PATH}" name "ws rotate"
gsettings set "${SCHEMA}.custom-keybinding:${ROTATE_PATH}" command "$WS_BIN rotate"
gsettings set "${SCHEMA}.custom-keybinding:${ROTATE_PATH}" binding "<super>r"

# Set up unfocus: super+u
gsettings set "${SCHEMA}.custom-keybinding:${UNFOCUS_PATH}" name "ws unfocus"
gsettings set "${SCHEMA}.custom-keybinding:${UNFOCUS_PATH}" command "$WS_BIN unfocus"
gsettings set "${SCHEMA}.custom-keybinding:${UNFOCUS_PATH}" binding "<super>u"

# Register the new keybindings
if [ "$existing" = "@as []" ]; then
    new_list="['${ROTATE_PATH}', '${UNFOCUS_PATH}']"
else
    # Strip trailing ] and append
    trimmed="${existing%]}"
    new_list="${trimmed}, '${ROTATE_PATH}', '${UNFOCUS_PATH}']"
fi

gsettings set $SCHEMA custom-keybindings "$new_list"

echo "Keybindings configured:"
echo "  super+r → ws rotate"
echo "  super+u → ws unfocus"
