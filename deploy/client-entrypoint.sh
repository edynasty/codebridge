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

exec codebridge-client "$@"
