# shellcheck shell=bash
# Shared helpers for the AmneziaWG container scripts. Sourced, not executed.

# Prints a uniformly distributed 48-bit random integer.
awg_random_u48() {
  local hi lo
  read -r hi lo < <(od -An -N8 -tu4 /dev/urandom)
  printf '%s\n' "$(((hi & 0xFFFF) << 32 | lo))"
}

# awg_random_between MIN MAX: prints a random integer in [MIN, MAX].
awg_random_between() {
  local minimum=$1 maximum=$2 value
  ((maximum >= minimum)) || return 1
  value=$(awg_random_u48)
  printf '%s\n' "$((minimum + value % (maximum - minimum + 1)))"
}

# Prints four non-overlapping H1..H4 ranges ("a-b") on one line. The uint32
# space above the standard WireGuard message types (1..4) is split into four
# segments; each message type gets a random segment and a random sub-range of
# 2^16..2^24 values inside it.
awg_header_ranges() {
  local first=5 last=4294967295 segment order=(0 1 2 3) i j tmp k base width start
  local -a ranges=()
  segment=$(((last - first + 1) / 4))
  for ((i = 3; i > 0; i--)); do
    j=$(awg_random_between 0 "$i")
    tmp=${order[i]}
    order[i]=${order[j]}
    order[j]=$tmp
  done
  for k in "${order[@]}"; do
    base=$((first + k * segment))
    width=$(awg_random_between 65536 16777216)
    start=$(awg_random_between "$base" "$((base + segment - 1 - width))")
    ranges+=("$start-$((start + width))")
  done
  printf '%s %s %s %s\n' "${ranges[@]}"
}

# awg_format_endpoint HOST PORT: prints HOST:PORT, bracketing IPv6 literals.
awg_format_endpoint() {
  local host=$1 port=$2
  if [[ $host == \[*\] ]]; then
    host=${host:1:${#host}-2}
  fi
  [[ -n $host && $host != CHANGE_ME && $host =~ ^[a-zA-Z0-9._:-]+$ ]] || return 1
  if [[ $host == *:* ]]; then
    printf '[%s]:%s\n' "$host" "$port"
  else
    printf '%s:%s\n' "$host" "$port"
  fi
}

# awg_validate_dns VALUE: an empty value or a list of IP literals.
awg_validate_dns() {
  local value=$1
  [[ $value != *$'\n'* && $value != *$'\r'* && $value != *$'\t'* ]] || return 1
  [[ -z $value || $value =~ ^[0-9a-fA-F:.,[:blank:]]+$ ]]
}

# Optional client-side signature packets (I1..I5) from AWG_CLIENT_I1..I5.
# Only the tags documented by amneziawg-go are accepted, so a typo cannot
# silently produce an empty packet.
awg_validate_signature_packets() {
  local n value tag='<(b 0x([0-9a-fA-F]{2})+|r [0-9]{1,4}|rd [0-9]{1,4}|rc [0-9]{1,4}|t)>'
  for n in 1 2 3 4 5; do
    value=$(awg_signature_packet "$n")
    [[ -z $value ]] && continue
    ((${#value} <= 4096)) || return 1
    [[ $value =~ ^($tag)+$ ]] || return 1
  done
}

awg_signature_packet() {
  local name="AWG_CLIENT_I$1"
  printf '%s' "${!name:-}"
}

awg_print_signature_packets() {
  local n value
  for n in 1 2 3 4 5; do
    value=$(awg_signature_packet "$n")
    [[ -z $value ]] || printf 'I%s = %s\n' "$n" "$value"
  done
}
