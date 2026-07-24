# Canonical command layer. PowerShell and Go remain implementation primitives.

# Shared generated inputs. Build profiles keep their own flags and packaging.
generate:
    go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate
    go run ./scripts/bake-release-notes
    go run ./scripts/bake-activity
    npm run build:css

# Windows/Wails debug build plus sibling binaries required by local smoke/audit flows.
debug: generate
    pwsh -NoLogo -NoProfile -File scripts/probe-clean.ps1
    pwsh -NoLogo -NoProfile -File scripts/build-debug.ps1
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/dixiedata-web.exe ./cmd/dixiedata-web
    pwsh -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "& './scripts/bundle-web-assets.ps1' -Root (Get-Location).Path"
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/seed-data.exe ./cmd/seed-data
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force build/bin | Out-Null"
    go build -tags debug -o build/bin/gold-master.exe ./cmd/gold-master
    pwsh -NoLogo -NoProfile -Command "New-Item -ItemType Directory -Force tools/tune/bin | Out-Null"
    pwsh -NoLogo -NoProfile -Command "Set-Location tools/tune; go build -tags debug -o bin/dixiedata-tune.exe ."

# Interactive Wails development stays uncaptured by design.
dev:
    wails dev

# Read-only contract check for local/CI bootstrap.
contract-test:
    node --test audit/just_contract.test.mjs
