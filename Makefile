# Razor File Explorer - Build Makefile

APP_NAME := Razor
BINARY_NAME := razor
VERSION := 1.0.0

# Directories
BUILD_DIR := build
ASSETS_DIR := assets

# Go build flags
LDFLAGS := -s -w

.PHONY: all build build-macos build-windows build-linux clean app-bundle icons

all: build

# Standard build for current platform
build:
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/razor

# Cross-platform builds
build-macos:
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/razor
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/razor

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/razor
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe ./cmd/razor

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/razor
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/razor

# macOS Application Bundle
app-bundle: build
	@echo "Creating macOS app bundle..."
	@mkdir -p "$(BUILD_DIR)/$(APP_NAME).app/Contents/MacOS"
	@mkdir -p "$(BUILD_DIR)/$(APP_NAME).app/Contents/Resources"
	@cp $(BUILD_DIR)/$(BINARY_NAME) "$(BUILD_DIR)/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)"
	@if [ -f "$(ASSETS_DIR)/icon.icns" ]; then \
		cp "$(ASSETS_DIR)/icon.icns" "$(BUILD_DIR)/$(APP_NAME).app/Contents/Resources/icon.icns"; \
	else \
		echo "Warning: icon.icns not found. Run 'make icons' first to generate it."; \
	fi
	@echo '<?xml version="1.0" encoding="UTF-8"?>' > "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '<plist version="1.0">' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '<dict>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>CFBundleExecutable</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>$(APP_NAME)</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>CFBundleIconFile</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>icon</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>CFBundleIdentifier</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>com.justyntemme.razor</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>CFBundleName</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>$(APP_NAME)</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>CFBundlePackageType</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>APPL</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>CFBundleShortVersionString</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>$(VERSION)</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>CFBundleVersion</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>$(VERSION)</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>LSMinimumSystemVersion</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <string>10.13</string>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>NSHighResolutionCapable</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <true/>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <key>NSSupportsAutomaticGraphicsSwitching</key>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '  <true/>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '</dict>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo '</plist>' >> "$(BUILD_DIR)/$(APP_NAME).app/Contents/Info.plist"
	@echo "App bundle created: $(BUILD_DIR)/$(APP_NAME).app"

# Generate icon files from SVG (requires rsvg-convert and iconutil on macOS)
icons:
	@echo "Generating icon files..."
	@mkdir -p $(BUILD_DIR)/icon.iconset
	# Generate PNG sizes for macOS iconset (requires rsvg-convert from librsvg)
	@if command -v rsvg-convert >/dev/null 2>&1; then \
		rsvg-convert -w 16 -h 16 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_16x16.png; \
		rsvg-convert -w 32 -h 32 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_16x16@2x.png; \
		rsvg-convert -w 32 -h 32 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_32x32.png; \
		rsvg-convert -w 64 -h 64 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_32x32@2x.png; \
		rsvg-convert -w 128 -h 128 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_128x128.png; \
		rsvg-convert -w 256 -h 256 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_128x128@2x.png; \
		rsvg-convert -w 256 -h 256 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_256x256.png; \
		rsvg-convert -w 512 -h 512 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_256x256@2x.png; \
		rsvg-convert -w 512 -h 512 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_512x512.png; \
		rsvg-convert -w 1024 -h 1024 $(ASSETS_DIR)/icon.svg -o $(BUILD_DIR)/icon.iconset/icon_512x512@2x.png; \
		iconutil -c icns $(BUILD_DIR)/icon.iconset -o $(ASSETS_DIR)/icon.icns; \
		echo "Created $(ASSETS_DIR)/icon.icns"; \
	else \
		echo "rsvg-convert not found. Install librsvg: brew install librsvg"; \
	fi

# Regenerate Windows .syso resource files (requires rsrc)
windows-resources:
	@echo "Generating Windows resource files..."
	@if command -v rsrc >/dev/null 2>&1 || [ -f "$(shell go env GOPATH)/bin/rsrc" ]; then \
		$(shell go env GOPATH)/bin/rsrc -ico $(ASSETS_DIR)/icon.ico -o cmd/razor/rsrc_windows_amd64.syso; \
		$(shell go env GOPATH)/bin/rsrc -ico $(ASSETS_DIR)/icon.ico -o cmd/razor/rsrc_windows_arm64.syso; \
		echo "Windows resources generated"; \
	else \
		echo "rsrc not found. Install: go install github.com/akavel/rsrc@latest"; \
	fi

clean:
	rm -rf $(BUILD_DIR)

# Install to /Applications (macOS)
install: app-bundle
	@echo "Installing to /Applications..."
	@cp -R "$(BUILD_DIR)/$(APP_NAME).app" /Applications/
	@echo "Installed to /Applications/$(APP_NAME).app"

# Run the application
run:
	go run ./cmd/razor

test:
	go test ./...
