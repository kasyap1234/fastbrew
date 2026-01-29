#!/bin/bash
set -e

# Benchmark script for FastBrew vs Homebrew

PKG="cowsay"

echo "🧹 Cleaning up..."
brew uninstall $PKG > /dev/null 2>&1 || true
rm -rf ~/.fastbrew/cache/*

echo -e "\n🐢 Benchmarking standard 'brew install $PKG'..."
start=$(date +%s%N)
brew install $PKG > /dev/null
end=$(date +%s%N)
brew_time=$((($end - $start)/1000000))
echo "👉 Brew took: ${brew_time}ms"

# Cleanup again
brew uninstall $PKG > /dev/null 2>&1 || true

echo -e "\n🐇 Benchmarking 'fastbrew install $PKG' (Native)..."
start=$(date +%s%N)
./fastbrew install $PKG
end=$(date +%s%N)
fastbrew_time=$((($end - $start)/1000000))
echo "👉 FastBrew took: ${fastbrew_time}ms"

echo -e "\n📊 Results:"
echo "Brew: $brew_time ms"
echo "FastBrew: $fastbrew_time ms"

if [ $brew_time -gt 0 ]; then
    ratio=$(echo "scale=2; $brew_time / $fastbrew_time" | bc)
    echo "🚀 Speedup: ${ratio}x"
fi

# Verify installation
echo -e "\n✅ Verifying installation..."
which cowsay
cowsay "FastBrew Rocks!"
