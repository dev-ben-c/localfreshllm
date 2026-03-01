#!/usr/bin/env bash
set -euo pipefail

# ── ANSI helpers ──────────────────────────────────────────────────────────────

BOLD='\033[1m'
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
RESET='\033[0m'

print_header()  { echo -e "\n${BOLD}${BLUE}══ $1 ══${RESET}\n"; }
print_step()    { echo -e "  ${CYAN}→${RESET} $1"; }
print_ok()      { echo -e "  ${GREEN}✓${RESET} $1"; }
print_warn()    { echo -e "  ${YELLOW}!${RESET} $1"; }
print_err()     { echo -e "  ${RED}✗${RESET} $1"; }
print_info()    { echo -e "  ${BOLD}$1${RESET}"; }

# ── Go installer ──────────────────────────────────────────────────────────────

GO_VERSION="1.23.6"

install_go() {
    local arch
    case "$(uname -m)" in
        x86_64)  arch="amd64" ;;
        aarch64) arch="arm64" ;;
        *)       print_err "Unsupported architecture: $(uname -m)"; return 1 ;;
    esac

    local tarball="go${GO_VERSION}.linux-${arch}.tar.gz"
    local url="https://go.dev/dl/${tarball}"

    print_step "Downloading Go ${GO_VERSION} (${arch})..."
    curl -fSL -o "/tmp/${tarball}" "$url"

    print_step "Installing to /usr/local/go..."
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"

    # Add to PATH for this session
    export PATH="/usr/local/go/bin:$PATH"
    print_ok "Go ${GO_VERSION} installed"

    # Hint for future sessions
    if ! grep -q '/usr/local/go/bin' "${HOME}/.profile" 2>/dev/null; then
        echo 'export PATH="/usr/local/go/bin:$PATH"' >> "${HOME}/.profile"
        print_info "Added /usr/local/go/bin to ~/.profile"
    fi
}

# ── Prerequisites ─────────────────────────────────────────────────────────────

check_prereqs() {
    local ok=true
    local need_go=false

    if ! command -v go &>/dev/null; then
        need_go=true
    else
        local go_ver
        go_ver=$(go version | grep -oP '\d+\.\d+' | head -1)
        if [[ "$(printf '%s\n' "1.23" "$go_ver" | sort -V | head -1)" != "1.23" ]]; then
            print_warn "Go >= 1.23 required (found $go_ver)"
            need_go=true
        else
            print_ok "Go $go_ver"
        fi
    fi

    if [[ "$need_go" == true ]]; then
        read -rp "  Go >= 1.23 not found. Install Go ${GO_VERSION}? [Y/n] " go_choice
        if [[ "${go_choice,,}" != "n" ]]; then
            install_go
        else
            print_err "Go is required to build"
            ok=false
        fi
    fi

    if [[ ! -f go.mod ]]; then
        print_err "Not in repo root (go.mod not found). Run from the localfreshllm directory."
        ok=false
    else
        print_ok "In repo root"
    fi

    if [[ ! -d ../localfreshsearch ]]; then
        print_step "Cloning localfreshsearch (build dependency)..."
        if git clone https://github.com/dev-ben-c/localfreshsearch.git ../localfreshsearch; then
            print_ok "Cloned ../localfreshsearch"
        else
            print_err "Failed to clone localfreshsearch"
            ok=false
        fi
    else
        print_ok "../localfreshsearch exists"
    fi

    if [[ "$ok" != true ]]; then
        echo
        print_err "Prerequisites not met. Aborting."
        exit 1
    fi
}

# ── Build + install ───────────────────────────────────────────────────────────

build_binary() {
    print_step "Building localfreshllm..."
    go build -o localfreshllm .
    print_ok "Built ./localfreshllm"
}

install_binary() {
    print_step "Installing to /usr/local/bin..."
    sudo cp localfreshllm /usr/local/bin/localfreshllm
    sudo chmod 755 /usr/local/bin/localfreshllm
    print_ok "Installed /usr/local/bin/localfreshllm"
}

create_data_dir() {
    mkdir -p ~/.local/share/localfreshllm
    print_ok "Data directory ready (~/.local/share/localfreshllm)"
}

# ── Shared: server prereq check ──────────────────────────────────────────────

check_server_prereqs() {
    local ok=true
    for cmd in systemctl ufw; do
        if ! command -v "$cmd" &>/dev/null; then
            print_err "$cmd not found"
            ok=false
        fi
    done
    if ! sudo -n true 2>/dev/null; then
        print_warn "sudo may prompt for password"
    fi
    if [[ "$ok" != true ]]; then
        print_err "Missing server prerequisites. Aborting."
        return 1
    fi
}

# ── Shared: write env file ───────────────────────────────────────────────────

write_env_file() {
    local master_key="$1"
    local anthropic_key="${2:-}"
    local env_file="/etc/localfreshllm.env"

    local env_content="LOCALFRESH_MASTER_KEY=${master_key}"
    if [[ -n "$anthropic_key" ]]; then
        env_content="${env_content}
ANTHROPIC_API_KEY=${anthropic_key}"
    fi
    env_content="${env_content}
HOME=/home/$(whoami)"

    echo "$env_content" | sudo tee "$env_file" > /dev/null
    sudo chmod 600 "$env_file"
    sudo chown root:root "$env_file"
}

# ── Shared: write systemd unit ───────────────────────────────────────────────

write_systemd_unit() {
    local service_file="/etc/systemd/system/localfreshllm.service"
    cat <<EOF | sudo tee "$service_file" > /dev/null
[Unit]
Description=LocalFreshLLM API Server
After=network.target ollama.service
Wants=ollama.service

[Service]
Type=simple
User=$(whoami)
Group=$(id -gn)
ExecStart=/usr/local/bin/localfreshllm serve --addr 0.0.0.0:8400
EnvironmentFile=/etc/localfreshllm.env
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    sudo systemctl daemon-reload
}

# ── Shared: UFW rule ─────────────────────────────────────────────────────────

ensure_ufw_rule() {
    if sudo ufw status | grep -q "8400"; then
        print_ok "UFW rule for port 8400 already exists"
    else
        sudo ufw allow from 192.168.0.0/24 to any port 8400 comment 'LocalFreshLLM'
        print_ok "Added UFW rule: 192.168.0.0/24 → port 8400"
    fi
}

# ── Shared: start + verify service ───────────────────────────────────────────

start_service() {
    sudo systemctl enable localfreshllm.service
    sudo systemctl restart localfreshllm.service
    print_ok "Service enabled and started"

    sleep 2
    if systemctl is-active --quiet localfreshllm.service; then
        print_ok "Service is running"
    else
        print_err "Service failed to start — check: journalctl -u localfreshllm.service"
    fi
}

# ── Shared: print registration command ───────────────────────────────────────

print_registration_cmd() {
    local master_key="$1"
    local ip
    ip=$(hostname -I | awk '{print $1}')

    echo
    print_header "Device Registration"
    print_info "To register a device from another machine, run:"
    echo
    echo "  curl -X POST http://${ip}:8400/v1/devices/register -H 'Content-Type: application/json' -d '{\"name\":\"<device-name>\",\"registration_key\":\"${master_key}\"}'"
    echo
    print_info "The response contains the API key for that device."
}

# ── Shared: detect shell profile ─────────────────────────────────────────────

detect_shell_profile() {
    case "${SHELL:-/bin/bash}" in
        */zsh)  echo "$HOME/.zshrc" ;;
        *)      echo "$HOME/.bashrc" ;;
    esac
}

# ── Shared: write client env to shell profile ────────────────────────────────

write_client_profile() {
    local server_url="$1"
    local api_key="$2"
    local profile
    profile=$(detect_shell_profile)

    if [[ ! -f "$profile" ]]; then
        print_err "Shell profile not found: ${profile}"
        return 1
    fi

    local block="# LocalFreshLLM client
export LOCALFRESH_SERVER=\"${server_url}\"
export LOCALFRESH_KEY=\"${api_key}\""

    if grep -q "LOCALFRESH_SERVER" "$profile" 2>/dev/null; then
        print_warn "LOCALFRESH_SERVER already in ${profile} — replacing"
        sed -i '/# LocalFreshLLM client/d' "$profile"
        sed -i '/export LOCALFRESH_SERVER=/d' "$profile"
        sed -i '/export LOCALFRESH_KEY=/d' "$profile"
    fi

    echo "" >> "$profile"
    echo "$block" >> "$profile"
    print_ok "Wrote client config to ${profile}"
    print_info "Run: source ${profile}"
}

# ═══════════════════════════════════════════════════════════════════════════════
# 1) Simple Client
# ═══════════════════════════════════════════════════════════════════════════════

simple_client() {
    print_header "Client Setup"

    build_binary
    install_binary
    create_data_dir

    # Server URL (retry loop)
    local server_url=""
    while true; do
        echo
        read -rp "  Server URL (e.g. http://192.168.0.69:8400): " server_url
        server_url="${server_url%/}"
        if [[ -z "$server_url" ]]; then
            print_err "Server URL cannot be empty"
            continue
        fi

        print_step "Checking server health..."
        if curl -sf "${server_url}/health" > /dev/null 2>&1; then
            print_ok "Server is reachable"
            break
        else
            print_warn "Could not reach ${server_url}/health"
            read -rp "  Retry? [Y/n] " retry
            if [[ "${retry,,}" == "n" ]]; then
                print_warn "Continuing without server validation"
                break
            fi
        fi
    done

    # Registration
    echo
    print_header "Device Registration"
    read -rsp "  Enter the server's master key: " master_key
    echo
    if [[ -z "$master_key" ]]; then
        print_err "Master key cannot be empty"
        return 1
    fi

    print_step "Registering device '$(hostname)'..."
    local reg_response
    reg_response=$(curl -sf -X POST "${server_url}/v1/devices/register" \
        -H 'Content-Type: application/json' \
        -d "{\"name\":\"$(hostname)\",\"registration_key\":\"${master_key}\"}" 2>&1) || true

    if [[ -z "$reg_response" ]]; then
        print_err "Registration failed — no response from server"
        return 1
    fi

    # Try to extract token from JSON response
    local api_key
    api_key=$(echo "$reg_response" | grep -oP '"token"\s*:\s*"\K[^"]+' || true)

    if [[ -z "$api_key" ]]; then
        print_err "Registration failed: ${reg_response}"
        return 1
    fi

    print_ok "Device registered"

    write_client_profile "$server_url" "$api_key"

    echo
    print_header "Done!"
    print_warn "Run this before using localfreshllm (or open a new terminal):"
    echo
    echo "  source $(detect_shell_profile)"
    echo
    print_info "Then: localfreshllm \"hello\""
}

# ═══════════════════════════════════════════════════════════════════════════════
# 2) Simple Server
# ═══════════════════════════════════════════════════════════════════════════════

simple_server() {
    print_header "Server Setup"
    check_server_prereqs

    build_binary
    install_binary
    create_data_dir

    # Auto-generate master key
    local master_key
    master_key=$(openssl rand -hex 32)
    print_ok "Generated master key"
    echo
    print_info "Master key: ${master_key}"
    print_info "Save this in Vaultwarden!"
    echo

    # Pick up ANTHROPIC_API_KEY from env if available
    local anthropic_key="${ANTHROPIC_API_KEY:-}"
    if [[ -n "$anthropic_key" ]]; then
        print_ok "Found ANTHROPIC_API_KEY in environment"
    else
        print_warn "ANTHROPIC_API_KEY not set — Anthropic models won't be available"
    fi

    # Write configs (overwrite if re-running)
    write_env_file "$master_key" "$anthropic_key"
    print_ok "Created /etc/localfreshllm.env (0600, root-owned)"

    write_systemd_unit
    print_ok "Created systemd service"

    ensure_ufw_rule

    print_header "Starting Service"
    start_service

    print_registration_cmd "$master_key"
}

# ═══════════════════════════════════════════════════════════════════════════════
# 3) Advanced
# ═══════════════════════════════════════════════════════════════════════════════

advanced_menu() {
    print_header "Advanced Setup"
    echo "  1) Standalone install — build + install binary only"
    echo "  2) Server with options — Playwright, manual key, overwrite controls"
    echo "  3) Back"
    echo
    read -rp "  Choose [1-3]: " choice
    case "$choice" in
        1) advanced_standalone ;;
        2) advanced_server ;;
        3) return ;;
        *) print_err "Invalid choice"; advanced_menu ;;
    esac
}

advanced_standalone() {
    print_header "Standalone Install"
    build_binary
    install_binary
    create_data_dir

    echo
    print_header "Done!"
    print_info "Usage:"
    echo "    localfreshllm \"what is the weather?\"   # one-shot"
    echo "    localfreshllm                           # interactive REPL"
    echo "    echo \"explain this\" | localfreshllm     # pipe mode"
    echo "    localfreshllm --list                    # list models"
}

advanced_server() {
    print_header "Advanced Server Setup"
    check_server_prereqs

    build_binary
    install_binary
    create_data_dir

    # Playwright (optional)
    echo
    read -rp "  Install Playwright browser deps for web scraping tools? [y/N] " pw_choice
    if [[ "${pw_choice,,}" == "y" ]]; then
        print_step "Installing Playwright chromium + deps..."
        go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium
        print_ok "Playwright installed"
    else
        print_info "Skipping Playwright"
    fi

    # Master key
    echo
    print_header "Master Key"
    read -rp "  Generate a new master key? [Y/n] " key_choice
    local master_key
    if [[ "${key_choice,,}" != "n" ]]; then
        master_key=$(openssl rand -hex 32)
        print_ok "Generated master key"
    else
        read -rsp "  Enter master key: " master_key
        echo
        if [[ -z "$master_key" ]]; then
            print_err "Master key cannot be empty"
            return 1
        fi
        print_ok "Using provided master key"
    fi

    echo
    print_info "Master key: ${master_key}"
    print_info "Save this in Vaultwarden!"
    echo

    # Anthropic key
    print_header "Anthropic API Key"
    local anthropic_key=""
    if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
        anthropic_key="${ANTHROPIC_API_KEY}"
        print_ok "Found ANTHROPIC_API_KEY in environment"
    else
        print_warn "ANTHROPIC_API_KEY not set — Anthropic models won't be available"
        read -rp "  Enter ANTHROPIC_API_KEY (or press Enter to skip): " anthropic_key
    fi

    # Env file (with overwrite check)
    print_header "Secrets File"
    local env_file="/etc/localfreshllm.env"
    if [[ -f "$env_file" ]]; then
        read -rp "  ${env_file} already exists. Overwrite? [y/N] " ow
        if [[ "${ow,,}" != "y" ]]; then
            print_info "Keeping existing secrets file"
        else
            write_env_file "$master_key" "$anthropic_key"
            print_ok "Updated ${env_file}"
        fi
    else
        write_env_file "$master_key" "$anthropic_key"
        print_ok "Created ${env_file} (0600, root-owned)"
    fi

    # Systemd (with overwrite check)
    print_header "Systemd Service"
    local service_file="/etc/systemd/system/localfreshllm.service"
    if [[ -f "$service_file" ]]; then
        read -rp "  ${service_file} already exists. Overwrite? [y/N] " ow
        if [[ "${ow,,}" != "y" ]]; then
            print_info "Keeping existing service file"
            sudo systemctl daemon-reload
        else
            write_systemd_unit
            print_ok "Updated ${service_file}"
        fi
    else
        write_systemd_unit
        print_ok "Created ${service_file}"
    fi

    print_header "Firewall"
    ensure_ufw_rule

    print_header "Starting Service"
    start_service

    print_registration_cmd "$master_key"
}

# ── Main menu ─────────────────────────────────────────────────────────────────

main_menu() {
    print_header "LocalFreshLLM Deploy"
    echo "  1) Client   — build + connect to a server"
    echo "  2) Server   — full server deploy (systemd, UFW, key gen)"
    echo "  3) Advanced — standalone install, Playwright, manual key, etc."
    echo "  4) Exit"
    echo
    read -rp "  Choose [1-4]: " choice
    case "$choice" in
        1) simple_client ;;
        2) simple_server ;;
        3) advanced_menu ;;
        4) exit 0 ;;
        *) print_err "Invalid choice"; main_menu ;;
    esac
}

# ── Entry point ───────────────────────────────────────────────────────────────

print_header "Checking prerequisites"
check_prereqs
echo
main_menu
