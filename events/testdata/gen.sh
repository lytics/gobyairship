#!/bin/bash
set -o errexit
set -o pipefail

url="localhost:5555/api/events/"
auth="a:a"

if [[ "$EVENTS_URL" != "" ]]; then
    url="$EVENTS_URL"
fi

if [[ "$EVENTS_AUTH" != "" ]]; then
    auth="$EVENTS_AUTH"
fi

cmd="http --check-status --auth=$auth --stream POST $url"

echo All
echo '{}' | $cmd | head -n 100 > all.json
echo Open
echo '{"filters":[{"types":["OPEN"]}]}' | $cmd | head -n 50 > open.json
echo Push
echo '{"filters":[{"types":["PUSH_BODY"]}]}' | $cmd | head -n 50 > push_body.json
echo Send
echo '{"filters":[{"types":["SEND"]}]}' | $cmd | head -n 50 > send.json
echo Close
echo '{"filters":[{"types":["CLOSE"]}]}' | $cmd | head -n 50 > close.json
echo Tag Change
echo '{"filters":[{"types":["TAG_CHANGE"]}]}' | $cmd | head -n 50 > tag_change.json
echo Uninstall
echo '{"filters":[{"types":["UNINSTALL"]}]}' | $cmd | head -n 50 > uninstall.json
echo First Open
echo '{"filters":[{"types":["FIRST_OPEN"]}]}' | $cmd | head -n 50 > first_open.json
echo Location
echo '{"filters":[{"types":["LOCATION"]}]}' | $cmd | head -n 50 > location.json
echo Skipping Custom events
#echo '{"filters":[{"types":["CUSTOM"]}]}' | $cmd | head -n 50 > custom.json
