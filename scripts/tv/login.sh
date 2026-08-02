#!/usr/bin/env bash
# tv-login - get the app from wherever it is to a usable, signed-in state.
#
# Three screens can stand between a fresh launch and the home, and a script
# that only knew about one of them reported "already signed in" while the app
# sat on the server-connect form:
#   1. "Connect to your server"  - after tv-reset, which wipes the stored URL
#   2. "Se connecter"            - after any force-stop, the session is gone
#   3. anything else             - already in, nothing to do
#
# Credentials and server URL come from the environment or scripts/tv/.env
# (gitignored), never from the repo. Missing credentials is not an error: an
# unattended tv-up must not fail on a machine with no dev account.
set -euo pipefail
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# localhost from the device is the device itself; the reverse tunnel is what
# makes this reach the API on the host (see tv-up).
: "${EVEY_TV_SERVER:=http://localhost:$TV_API_PORT}"

# screen_text dumps the visible labels. Retried: right after a launch the view
# hierarchy is briefly empty, and reading it too early is what produced the
# false "already signed in".
screen_text() {
  local i
  for i in 1 2 3 4 5 6; do
    if adb_ shell uiautomator dump /sdcard/evey-ui.xml >/dev/null 2>&1; then
      local txt
      txt=$(adb_ shell cat /sdcard/evey-ui.xml 2>/dev/null | grep -oE 'text="[^"]*"' || true)
      [[ -n "$txt" ]] && { printf '%s' "$txt"; return 0; }
    fi
    sleep 2
  done
  return 1
}

# BACK closes the soft keyboard without losing what was typed - but ONLY if a
# keyboard is open. Sent unconditionally it is the "leave the app" key, and
# this script spent a while quietly kicking itself out to the TV launcher and
# then reporting that the app was not in the foreground.
keyboard_shown() {
  adb_ shell dumpsys input_method 2>/dev/null | grep -q "mInputShown=true"
}
dismiss_keyboard() {
  keyboard_shown || return 0
  adb_ shell input keyevent 4 >/dev/null
  sleep 1
}

# Taps are resolved from the view hierarchy, never from hardcoded pixels: a
# coordinate that is right today silently misses the day a label wraps or the
# layout shifts, and the failure looks exactly like "the app ignored me".
node_center() { # node_center <regex sur text ou resource-id>
  # A page title and its submit button carry the SAME text, and the clickable
  # node is the parent - the label itself reports clickable="false". Taps are
  # resolved by coordinate, not by node, so tapping the label works; the only
  # thing to get right is picking the right one of the two. The button is
  # always the lower of the two, so the match with the greatest top-y wins.
  adb_ shell cat /sdcard/evey-ui.xml 2>/dev/null | tr '>' '\n' \
    | grep -iE "$1" \
    | grep -oE 'bounds="\[[0-9]+,[0-9]+\]\[[0-9]+,[0-9]+\]"' \
    | sed -E 's/bounds="\[([0-9]+),([0-9]+)\]\[([0-9]+),([0-9]+)\]"/\2 \1 \2 \3 \4/' \
    | sort -rn | head -1 \
    | awk '{ if (NF==5) printf "%d %d", ($2+$4)/2, ($3+$5)/2 }'
}

tap_label() { # tap_label <regex>
  adb_ shell uiautomator dump /sdcard/evey-ui.xml >/dev/null 2>&1 || return 1
  local xy; xy=$(node_center "$1")
  [[ -n "$xy" ]] || return 1
  # shellcheck disable=SC2086
  adb_ shell input tap $xy >/dev/null
  sleep 1
}

# Placeholder text is a moving target: it is translated, it changes when the
# field has content, and it disappears behind the keyboard. The input BOXES
# are stable, so fields are addressed by position among the EditText nodes.
nth_field() { # nth_field <n>
  adb_ shell uiautomator dump /sdcard/evey-ui.xml >/dev/null 2>&1 || return 1
  adb_ shell cat /sdcard/evey-ui.xml 2>/dev/null | tr '>' '\n' \
    | grep 'class="android.widget.EditText"' \
    | grep -oE 'bounds="\[[0-9]+,[0-9]+\]\[[0-9]+,[0-9]+\]"' | sed -n "$1p" \
    | sed -E 's/bounds="\[([0-9]+),([0-9]+)\]\[([0-9]+),([0-9]+)\]"/\1 \2 \3 \4/' \
    | awk '{ if (NF==4) printf "%d %d", ($1+$3)/2, ($2+$4)/2 }'
}

fill_field() { # fill_field <n> <valeur>
  dismiss_keyboard
  local xy; xy=$(nth_field "$1")
  [[ -n "$xy" ]] || return 1
  # shellcheck disable=SC2086
  adb_ shell input tap $xy >/dev/null; sleep 1
  adb_ shell input text "$2" >/dev/null; sleep 1
}

# Move focus down to the primary button and press OK. Two downs at most: the
# forms have one or two fields above the button, and a press on an already
# focused button is harmless.
submit() {
  adb_ shell input dpad keyevent 20 >/dev/null 2>&1 || true; sleep 1
  adb_ shell input dpad keyevent 23 >/dev/null 2>&1 || adb_ shell input keyevent 23 >/dev/null
}

type_into() { # type_into <regex du champ> <valeur>
  # The soft keyboard opens on the first field and covers the next one, so a
  # tap aimed at "password" landed back on "username" and both values ended up
  # concatenated in one box. Closing it first is what keeps the fields apart.
  dismiss_keyboard
  tap_label "$1" || return 1
  adb_ shell input text "$2" >/dev/null; sleep 1
}

device_ready || die "aucun device attaché" "make tv-up"
TEXT=$(screen_text || true)

if grep -qi 'your server\|votre serveur' <<<"$TEXT"; then
  step "serveur non configuré → $EVEY_TV_SERVER"
  fill_field 1 "$EVEY_TV_SERVER" \
    || die "champ URL introuvable" "make tv-shot puis regarde l'écran"
  dismiss_keyboard
  submit
  sleep 8
  TEXT=$(screen_text || true)
fi

if grep -qi "nom d'utilisateur\|username" <<<"$TEXT"; then
  if [[ -z "${EVEY_TV_USER:-}" || -z "${EVEY_TV_PASS:-}" ]]; then
    warn "écran de connexion, mais EVEY_TV_USER / EVEY_TV_PASS manquent"
    say  "  ajoute-les à scripts/tv/.env (ignoré par git)"
    exit 0
  fi
  step "connexion (${EVEY_TV_USER})"
  fill_field 1 "$EVEY_TV_USER" || die "champ utilisateur introuvable" "make tv-shot"
  fill_field 2 "$EVEY_TV_PASS" || die "champ mot de passe introuvable" "make tv-shot"
  dismiss_keyboard
  # A TV Pressable answers the SELECT key, not a synthetic touch: tapping the
  # highlighted button did nothing at all. Focus already sits on it after the
  # last field, so OK is both the simplest and the most faithful to what a
  # viewer actually does.
  submit
  sleep 10
  TEXT=$(screen_text || true)
fi

# Deliberately best-effort from here on.
#
# Filling the fields is where the tedium is: typing a URL and a password with a
# D-pad is a minute of work, and this does it in two seconds. Pressing the
# final button turned out to be worth far less than the time spent automating
# it - a TV Pressable ignores synthetic taps, and driving focus reliably across
# two different forms was a rabbit hole. So the script stops at "filled" and
# says so, rather than failing and making the caller think nothing worked.

# A dialog can sit on top of the form, and an earlier version of this script
# declared success while "Connection failed" was on screen - the check must
# look for the failure, not only for the absence of the form.
if grep -qiE 'connection failed|échec de (la )?connexion|network request failed' <<<"$TEXT"; then
  fail "le serveur n'est pas joignable depuis l'app ($EVEY_TV_SERVER)"
  say  "  le tunnel reverse est lié au transport adb : après une reconnexion il faut le refaire"
  say  "  → make tv-up"
  exit 1
fi
# The placeholder disappears once a field is filled, so its absence proves
# nothing: the SUBMIT button is what tells us the form is still up.
if grep -qiE 'text="(Se connecter|Sign in|Connect)"' <<<"$TEXT"; then
  ok "champs remplis - appuie sur OK sur l'émulateur pour valider"
  exit 0
fi
# Being off the login form is not the same as being in the app: the Android TV
# launcher has no "username" label either, and an app that had died was twice
# reported as "signed in".
if [[ "$(current_activity)" != "$TV_PKG/$TV_ACTIVITY" ]]; then
  fail "l'app n'est pas au premier plan ($(current_activity))"
  say  "  → make tv-state, puis make tv-logs --all"
  exit 1
fi
ok "session prête"
