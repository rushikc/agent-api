# sourced from ~/.zshrc via: [[ -f ~/.devhub/shell/devhub.zsh ]] && source ~/.devhub/shell/devhub.zsh
# No-op unless this pane was created by devhub.

if [[ -z "${DEVHUB_PROJECT:-}" ]]; then
  return 0
fi

devhub_prompt_badge() {
  local color="${DEVHUB_COLOR:-#89B4FA}"
  print -n "%K{${color}}%F{black}%B[${DEVHUB_PROJECT}]%b%f%k "
}

# Starship owns PROMPT; a custom module is printed by `devhub shell install`.
if [[ -n "${STARSHIP_SHELL:-}" ]]; then
  return 0
fi

prompt_devhub() {
  if typeset -f p10k >/dev/null 2>&1; then
    p10k segment -b "${DEVHUB_COLOR:-blue}" -f black -t "[${DEVHUB_PROJECT}]"
  fi
}

if typeset -p POWERLEVEL9K_LEFT_PROMPT_ELEMENTS >/dev/null 2>&1; then
  if [[ ${POWERLEVEL9K_LEFT_PROMPT_ELEMENTS[(Ie)devhub]} -eq 0 ]]; then
    POWERLEVEL9K_LEFT_PROMPT_ELEMENTS=(devhub "${POWERLEVEL9K_LEFT_PROMPT_ELEMENTS[@]}")
  fi
  return 0
fi

_devhub_precmd() {
  [[ -n "${DEVHUB_PROJECT:-}" ]] || return
  if [[ ${PROMPT} != *"[${DEVHUB_PROJECT}]"* ]]; then
    PROMPT="$(devhub_prompt_badge)${PROMPT}"
  fi
}

autoload -Uz add-zsh-hook
add-zsh-hook precmd _devhub_precmd
