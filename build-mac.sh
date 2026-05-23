#!/bin/bash
# Build script for macOS

set -e

echo "🍎 Building ACFun Live Helper for macOS..."

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

# Build for both Intel and Apple Silicon
echo "🔨 Building for macOS (universal binary)..."
wails build -platform darwin/universal

echo "✅ Build complete! Application: ./build/bin/ACFun Live Helper.app"
