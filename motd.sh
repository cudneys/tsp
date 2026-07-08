#!/bin/sh
# tsp-motd — login banner for the network troubleshooting pod.
# Sourced by interactive shells (see Dockerfile). POSIX sh; no external deps.

# Only show once per shell session, and only when there is a terminal.
if [ -z "${TSP_MOTD_SHOWN:-}" ]; then
  TSP_MOTD_SHOWN=1
  export TSP_MOTD_SHOWN

  # Colors only when stdout is a TTY.
  if [ -t 1 ]; then
    _b=$(printf '\033[1m'); _c=$(printf '\033[36m'); _y=$(printf '\033[33m')
    _d=$(printf '\033[2m'); _r=$(printf '\033[0m')
  else
    _b=; _c=; _y=; _d=; _r=
  fi

  # Column widths: command=13, description=52.
  _rule() { # $1 left  $2 join  $3 right
    printf '%s' "$1"
    i=0; while [ "$i" -lt 15 ]; do printf '─'; i=$((i + 1)); done
    printf '%s' "$2"
    i=0; while [ "$i" -lt 54 ]; do printf '─'; i=$((i + 1)); done
    printf '%s\n' "$3"
  }
  _row() { printf "│ %b%-13s%b │ %-52s │\n" "$_c" "$1" "$_r" "$2"; }
  _cat() { printf "│ %b%-68s%b │\n" "$_y$_b" "$1" "$_r"; }

  printf '\n'
  printf '  %b🛠  TSP — Network Troubleshooting Pod%b\n\n' "$_b" "$_r"

  _rule '┌' '┬' '┐'
  printf "│ %b%-13s%b │ %b%-52s%b │\n" "$_b" "COMMAND" "$_r" "$_b" "WHAT IT DOES" "$_r"
  _rule '├' '┼' '┤'

  _cat 'CONNECTIVITY & ROUTING'
  _row 'ip'         'addresses, routes, links (iproute2)'
  _row 'ping'       'ICMP reachability'
  _row 'traceroute' 'hop-by-hop path to a host'
  _row 'tracepath'  'path + MTU discovery (no root needed)'
  _row 'mtr'        'continuous traceroute + loss/latency'
  _row 'arping'     'ARP-level reachability on the L2 segment'
  _row 'ethtool'    'NIC driver, link speed, offloads'
  _row 'ss'         'socket / connection statistics'
  _row 'netstat'    'legacy sockets & routes (net-tools)'

  _rule '├' '┼' '┤'
  _cat 'DNS'
  _row 'dig'        'DNS queries, +trace, +short'
  _row 'nslookup'   'quick name lookups'
  _row 'drill'      'DNS queries incl. DNSSEC (ldns)'

  _rule '├' '┼' '┤'
  _cat 'HTTP / TLS'
  _row 'curl'       'HTTP(S) client, -v for headers/TLS'
  _row 'wget'       'fetch files / test endpoints'
  _row 'openssl'    's_client -connect to inspect certs/TLS'

  _rule '├' '┼' '┤'
  _cat 'CAPTURE & ANALYSIS'
  _row 'tcpdump'    'packet capture & filtering'
  _row 'tshark'     'CLI Wireshark, protocol dissection'
  _row 'ngrep'      'grep across live packet payloads'

  _rule '├' '┼' '┤'
  _cat 'PORTS / SCAN / THROUGHPUT'
  _row 'nc'         'netcat: connect/listen on TCP/UDP'
  _row 'socat'      'bidirectional socket relay/proxy'
  _row 'nmap'       'port scan & service detection'
  _row 'iperf3'     'TCP/UDP throughput testing'
  _row 'nft'        'inspect/modify nftables firewall'

  _rule '├' '┼' '┤'
  _cat 'UTILITIES'
  _row 'jq'         'filter & format JSON'
  _row 'yq'         'filter & format YAML'
  _row 'lsof'       'list open files / sockets'
  _row 'strace'     'trace syscalls of a process'
  _row 'htop'       'interactive process viewer'

  _rule '└' '┴' '┘'

  # Downward API values (fall back to hostname if unset).
  printf '\n  %bPod:%b %s   %bNamespace:%b %s   %bNode:%b %s\n\n' \
    "$_b" "$_r" "${POD_NAME:-$(hostname)}" \
    "$_b" "$_r" "${POD_NAMESPACE:-?}" \
    "$_b" "$_r" "${NODE_NAME:-?}"

  unset _b _c _y _d _r i
fi
