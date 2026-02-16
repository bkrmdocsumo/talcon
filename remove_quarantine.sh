#!/usr/bin/env bash

set -e

APP_NAME="Talon.app"

echo "🔍 Looking for $APP_NAME..."

if [ -d "/Applications/$APP_NAME" ]; then
    APP_PATH="/Applications/$APP_NAME"
    echo "✅ Found in /Applications"
elif [ -d "$HOME/Downloads/$APP_NAME" ]; then
    APP_PATH="$HOME/Downloads/$APP_NAME"
    echo "✅ Found in ~/Downloads"
else
    echo "❌ Not found in /Applications or ~/Downloads."
    read -p "👉 Please enter full path to $APP_NAME: " APP_PATH
fi

if [ ! -d "$APP_PATH" ]; then
    echo "🚨 Invalid path: $APP_PATH"
    exit 1
fi

echo "🔧 Removing quarantine attributes from:"
echo "   $APP_PATH"

sudo xattr -cr "$APP_PATH"

echo "🎉 Done! You should now be able to open Talon.app normally."

