#!/bin/sh
# Container entrypoint for the CodeBridge client.
#
# omp (oh-my-pi) keeps its provider table, settings and credentials inside its
# agent directory. The host copy is mounted read-only at /omp-seed; seed a
# writable agent dir from it once, then leave whatever is in the state volume
# alone so in-container edits survive restarts.
set -e

seed=/omp-seed
agent_dir="${PI_CODING_AGENT_DIR:-/state/omp}"

if [ -d "$seed" ]; then
	mkdir -p "$agent_dir"
	if [ -f "$seed/models.yml" ] && [ ! -f "$agent_dir/models.yml" ]; then
		# omp's provider base URLs point at the host loopback (the local model
		# proxies). Inside the container the host is host.docker.internal.
		sed -e 's#http://localhost:#http://host.docker.internal:#g' \
			-e 's#http://127\.0\.0\.1:#http://host.docker.internal:#g' \
			"$seed/models.yml" >"$agent_dir/models.yml"
	fi
	for f in config.yml agent.db; do
		if [ -f "$seed/$f" ] && [ ! -f "$agent_dir/$f" ]; then
			cp "$seed/$f" "$agent_dir/$f"
		fi
	done
fi

# OpenCode stores its durable SQLite database under XDG_DATA_HOME/opencode.
# Seed only auth.json; do not import the host's very large database or plugins.
opencode_seed=/opencode-seed/auth.json
opencode_data_home="${CODEBRIDGE_OPENCODE_DATA_HOME:-/state/opencode-data}"
opencode_dir="$opencode_data_home/opencode"
mkdir -p "$opencode_dir"
if [ -f "$opencode_seed" ] && [ ! -f "$opencode_dir/auth.json" ]; then
	cp "$opencode_seed" "$opencode_dir/auth.json"
	chmod 600 "$opencode_dir/auth.json"
fi

# Codex uses its own persistent home. Seed only auth.json from the host so the
# container can authenticate without inheriting host-only plugins, MCP servers,
# notify hooks, or Computer Use configuration.
codex_seed=/codex-seed/auth.json
codex_dir="${CODEX_HOME:-/state/codex}"
mkdir -p "$codex_dir"
if [ -f "$codex_seed" ] && [ ! -f "$codex_dir/auth.json" ]; then
	cp "$codex_seed" "$codex_dir/auth.json"
	chmod 600 "$codex_dir/auth.json"
fi

exec codebridge-client "$@"
