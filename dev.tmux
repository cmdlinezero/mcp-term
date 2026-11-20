#!/usr/bin/env bash

# DEFAULT_PORT
DEFAULT_PORT=8080
# Detect if we are on a Mac
if [[ "$OSTYPE" == "darwin"* ]]; then
  SHELL_CMD="zsh"
else
  SHELL_CMD="bash"
fi

# Generate random Docker-like name
generate_session_name() {
  ADJECTIVES=(happy brave calm eager fancy gentle jolly kind lucky nice proud quick shiny witty bold clever cosmic mystic swift agile bright cosmic stellar lunar solar turbo hyper mega micro)
  NOUNS=(tiger panda eagle otter fox whale lion falcon wolf bear koala shark hawk raven badger lynx panther viper cobra dragon phoenix)

  local adj=${ADJECTIVES[$RANDOM % ${#ADJECTIVES[@]}]}
  local noun=${NOUNS[$RANDOM % ${#NOUNS[@]}]}
  local suffix=$((RANDOM % 9000 + 1000)) # Generates a 4-digit number (1000-9999)

  echo "${adj}-${noun}-${suffix}"
}

SESSION=${2:-$(generate_session_name)}
echo "Using SESSION=$SESSION"

# Check if the session already exists
tmux has-session -t "$SESSION" 2>/dev/null

# Create new sessions [Optional: ZSH default shell]
if [ $? != 0 ]; then
  tmux new-session -d -s "$SESSION" "$SHELL_CMD"
  tmux split-window -h -t "$SESSION" "$SHELL_CMD"
  tmux split-window -v -t "$SESSION" "$SHELL_CMD"

  # Optional: Send terminal command to a pane if needed
  # tmux send-keys -t "$SESSION":0.0 "ls" C-m

  tmux send-keys -t "$SESSION":0
fi

tmux attach-session -t "$SESSION"
