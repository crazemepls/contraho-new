#!/usr/bin/env bash
# Runs contraho, saves a timestamped CSV and emails it.
# Defaults (server, token, SMTP, recipients...) live in contraho.conf next to this
# script. The options below override the config for a single run.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<'USAGE'
Usage: contraho-report.sh [options]

Options (each overrides the value in contraho.conf for this run only):
  --mail-to a@x.com,b@x.com   recipients
  --mail-subject "text"       email subject
  --bureau "tsc:"             only projects whose key starts with this ("" = all projects)
  --conf /path/to/file        use another config file
                              (default: contraho.conf next to this script)
  -h, --help                  show this help

Examples:
  ./contraho-report.sh
  ./contraho-report.sh --mail-to bob@example.com --mail-subject "TSC check"
  ./contraho-report.sh --bureau "tsc:" --conf ~/other.conf
USAGE
}

need_value() { # $1 = option name, $2 = number of arguments left (including the option)
  if [ "$2" -lt 2 ]; then echo "$1 needs a value" >&2; exit 2; fi
}

CONF="$SCRIPT_DIR/contraho.conf"
MAIL_TO_ARG="" MAIL_SUBJECT_ARG="" BUREAU_ARG="" MAIL_TO_SET=0 MAIL_SUBJECT_SET=0 BUREAU_SET=0
while [ $# -gt 0 ]; do
  case "$1" in
    --mail-to)        need_value "$1" $#; MAIL_TO_ARG="$2"; MAIL_TO_SET=1; shift ;;
    --mail-to=*)      MAIL_TO_ARG="${1#*=}"; MAIL_TO_SET=1 ;;
    --mail-subject)   need_value "$1" $#; MAIL_SUBJECT_ARG="$2"; MAIL_SUBJECT_SET=1; shift ;;
    --mail-subject=*) MAIL_SUBJECT_ARG="${1#*=}"; MAIL_SUBJECT_SET=1 ;;
    --bureau)         need_value "$1" $#; BUREAU_ARG="$2"; BUREAU_SET=1; shift ;;
    --bureau=*)       BUREAU_ARG="${1#*=}"; BUREAU_SET=1 ;;
    --conf)           need_value "$1" $#; CONF="$2"; shift ;;
    --conf=*)         CONF="${1#*=}" ;;
    -h|--help)        usage; exit 0 ;;
    *)                echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

if [ ! -r "$CONF" ]; then
  echo "cannot read $CONF (copy contraho.conf.example to contraho.conf and fill it in)" >&2
  exit 2
fi
if [ "$(stat -c %a "$CONF")" != "600" ]; then
  echo "warning: $CONF holds secrets, run: chmod 600 $CONF" >&2
fi

# Read NAME=value lines from the config (it is parsed, never executed as shell code,
# so no quoting is needed). Values are exported so contraho sees SONAR_TOKEN and
# SMTP_PASSWORD. Comments go on their own line, starting with #.
load_conf() {
  local line key value n=0
  while IFS= read -r line || [ -n "$line" ]; do
    n=$((n + 1))
    line="${line%$'\r'}"                                   # tolerate Windows line endings
    if [[ "$line" =~ ^[[:space:]]*(#.*)?$ ]]; then continue; fi
    if [[ ! "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=(.*)$ ]]; then
      echo "$CONF line $n: expected NAME=value" >&2; exit 2
    fi
    key="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    value="${value#"${value%%[![:space:]]*}"}"              # trim leading spaces
    value="${value%"${value##*[![:space:]]}"}"              # trim trailing spaces
    if [[ "$value" =~ ^\'(.*)\'$ ]] || [[ "$value" =~ ^\"(.*)\"$ ]]; then
      value="${BASH_REMATCH[1]}"                            # one pair of quotes is optional
    fi
    case "$key" in
      SONAR_HOST|SONAR_TOKEN|BUREAU|MAIL_TO|MAIL_FROM|MAIL_SUBJECT|SMTP_HOST|SMTP_USER|SMTP_PASSWORD|SMTP_TLS|KEEP_DAYS|OUT_DIR|BIN)
        export "$key=$value" ;;
      *) echo "$CONF line $n: unknown setting $key (typo?)" >&2; exit 2 ;;
    esac
  done < "$CONF"
}
load_conf

# Command-line options win over the config file.
if [ "$MAIL_TO_SET" = 1 ];      then MAIL_TO="$MAIL_TO_ARG"; fi
if [ "$MAIL_SUBJECT_SET" = 1 ]; then MAIL_SUBJECT="$MAIL_SUBJECT_ARG"; fi
if [ "$BUREAU_SET" = 1 ];       then BUREAU="$BUREAU_ARG"; fi

: "${SONAR_HOST:?missing in $CONF}"
: "${SONAR_TOKEN:?missing in $CONF}"
: "${MAIL_FROM:?missing in $CONF}"
: "${SMTP_HOST:?missing in $CONF}"
: "${MAIL_TO:?set MAIL_TO in $CONF or pass --mail-to}"

BIN="${BIN:-$SCRIPT_DIR/contraho}"
OUT_DIR="${OUT_DIR:-$SCRIPT_DIR/reports}"
KEEP_DAYS="${KEEP_DAYS:-30}"      # 0 = never delete old reports
SMTP_TLS="${SMTP_TLS:-auto}"

mkdir -p "$OUT_DIR"
CSV="$OUT_DIR/report-$(date +%F_%H%M%S).csv"

args=(project --host="$SONAR_HOST" --filename="$CSV"
      --mail-to="$MAIL_TO" --mail-from="$MAIL_FROM"
      --smtp-host="$SMTP_HOST" --smtp-tls="$SMTP_TLS")
if [ -n "${BUREAU:-}" ];       then args+=(--bureau="$BUREAU"); fi
if [ -n "${SMTP_USER:-}" ];    then args+=(--smtp-user="$SMTP_USER"); fi
if [ -n "${MAIL_SUBJECT:-}" ]; then args+=(--mail-subject="$MAIL_SUBJECT"); fi

echo "Sending to: $MAIL_TO${BUREAU:+  (projects starting with \"$BUREAU\")}"
"$BIN" "${args[@]}"      # a failure stops the script with contraho's exit code
echo "Saved: $CSV"

# Remove old reports (only report-*.csv files in OUT_DIR).
if [ "$KEEP_DAYS" -gt 0 ]; then
  find "$OUT_DIR" -maxdepth 1 -name 'report-*.csv' -mtime +"$KEEP_DAYS" -delete
fi
