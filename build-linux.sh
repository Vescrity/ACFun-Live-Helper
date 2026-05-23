#!/bin/bash
# Build script for Linux

set -e

echo "🐧 Building ACFun Live Helper for Linux..."

# Check dependencies
if ! command -v wails &> /dev/null; then
    echo "❌ Wails CLI not found. Installing..."
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
fi

if ! command -v npm &> /dev/null; then
    echo "❌ Node.js/npm not found. Please install Node.js 18 or higher."
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "❌ Go not found. Please install Go 1.21 or higher."
    exit 1
fi

# Install dependencies
echo "📦 Installing dependencies..."
npm install
go mod tidy

# Build
echo "🔨 Building..."
wails build -platform linux/amd64

echo "✅ Build complete! Executable: ./build/bin/acfun-live-helper"
